package os

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	ssv1alpha1 "github.com/bitnami-labs/sealed-secrets/pkg/apis/sealed-secrets/v1alpha1"
	"github.com/bitnami-labs/sealed-secrets/pkg/crypto"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	existinginfrav1 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeios "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/cluster/machine"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	capeiresource "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/scheme"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/specs"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/object"
	"github.com/weaveworks/libgitops/pkg/serializer"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/keyutil"
	"sigs.k8s.io/yaml"
)

const (
	sealedSecretVersion       = "v0.11.0"
	sealedSecretKeySecretName = "sealed-secrets-key"
	fluxSecretTemplate        = `apiVersion: v1
{{ if .SecretValue }}
data:
  identity: {{.SecretValue}}
{{ end }}
kind: Secret
metadata:
  name: flux-git-deploy
  namespace: {{.Namespace}}
type: Opaque`
)

var (
	pemKeys = []string{"certificate-authority", "client-certificate", "client-key"}
)

// SetupSeedNode installs Kubernetes on this machine, and store the provided
// manifests in the API server, so that the rest of the cluster can then be
// set up by the WKS controller.
func SetupSeedNode(ctx context.Context, o *capeios.OS, params SeedNodeParams) error {
	p, err := CreateSeedNodeSetupPlan(ctx, o, params)
	if err != nil {
		return err
	}
	updatedParams, err = createMachinePoolInfo(updatedParams)
	if err != nil {
		return nil, err
	}
	kubernetesVersion, kubernetesNamespace, err := machine.GetKubernetesVersionFromManifest(params.MachinesManifestPath)
	if err != nil {
		return nil, err
	}
	cluster, err := parseCluster(params.ClusterManifestPath)
	if err != nil {
		return nil, err
	}
	// Get configuration file resources from config map manifests referenced by the cluster spec
	configMapManifests, configMaps, configFileResources, err := createConfigFileResourcesFromFiles(&cluster.Spec, params.ConfigDirectory, params.Namespace)
	if err != nil {
		return nil, err
	}

	b := plan.NewBuilder()

	baseRes := capeirecipe.BuildBasePlan(o.PkgType)
	b.AddResource("install:base", baseRes)

	configRes := capeirecipe.BuildConfigPlan(configFileResources)
	b.AddResource("install:config", configRes, plan.DependOn("install:base"))

	pemSecretResources, authConfigMap, authConfigManifest, err := processPemFilesIfAny(b, &cluster.Spec, params.ConfigDirectory, params.Namespace, params.SealedSecretKeyPath, params.SealedSecretCertPath)
	if err != nil {
		return nil, err
	}

	criRes := capeirecipe.BuildCRIPlan(ctx, &cluster.Spec.CRI, cfg, o.PkgType)
	b.AddResource("install:cri", criRes, plan.DependOn("install:config"))

	k8sRes := capeirecipe.BuildK8SPlan(kubernetesVersion, params.KubeletConfig.NodeIP, cfg.SELinuxInstalled, cfg.SetSELinuxPermissive, cfg.DisableSwap, cfg.LockYUMPkgs, o.PkgType, params.KubeletConfig.CloudProvider, params.KubeletConfig.ExtraArguments)
	b.AddResource("install:k8s", k8sRes, plan.DependOn("install:cri"))

	apiServerArgs := getAPIServerArgs(&cluster.Spec, pemSecretResources)

	// Backwards-compatibility: fall back if not specified
	controlPlaneEndpoint := params.ControlPlaneEndpoint
	if controlPlaneEndpoint == "" {
		// TODO: dynamically inject the API server's port.
		controlPlaneEndpoint = params.PrivateIP + ":6443"
	}

	kubeadmInitResource :=
		&capeiresource.KubeadmInit{
			PublicIP:              params.PublicIP,
			PrivateIP:             params.PrivateIP,
			KubeletConfig:         &params.KubeletConfig,
			ConntrackMax:          cfg.ConntrackMax,
			UseIPTables:           cfg.UseIPTables,
			SSHKeyPath:            params.SSHKeyPath,
			BootstrapToken:        params.BootstrapToken,
			ControlPlaneEndpoint:  controlPlaneEndpoint,
			IgnorePreflightErrors: cfg.IgnorePreflightErrors,
			KubernetesVersion:     kubernetesVersion,
			CloudProvider:         params.KubeletConfig.CloudProvider,
			ImageRepository:       params.ImageRepository,
			AdditionalSANs:        params.AdditionalSANs,
			Namespace:             object.String(params.Namespace),
			NodeName:              cfg.HostnameOverride,
			ExtraAPIServerArgs:    apiServerArgs,
			// kubeadm currently accepts a single subnet for services and pods
			// ref: https://godoc.org/k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1#Networking
			// this should be ensured in the validation step in pkg.specs.validation.validateCIDRBlocks()
			ServiceCIDRBlock: params.ServicesCIDRBlocks[0],
			PodCIDRBlock:     params.PodsCIDRBlocks[0],
		}
	b.AddResource("kubeadm:init", kubeadmInitResource, plan.DependOn("install:k8s"))

	// TODO(damien): Add a CNI section in cluster.yaml once we support more than one CNI plugin.
	const cni = "weave-net"

	cniAdddon := capeiv1alpha3.Addon{Name: cni}

	// we use the namespace defined in addon-namespace map to make weave-net run in kube-system
	// as weave-net requires to run in the kube-system namespace *only*.
	manifests, err := buildAddon(cniAdddon, params.ImageRepository, params.ClusterManifestPath, params.GetAddonNamespace(cni))
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate manifests for CNI plugin")
	}

	if len(params.PodsCIDRBlocks) > 0 && params.PodsCIDRBlocks[0] != "" {
		// setting the pod CIDR block is currently only supported for the weave-net CNI
		if cni == "weave-net" {
			manifests, err = SetWeaveNetPodCIDRBlock(manifests, params.PodsCIDRBlocks[0])
			if err != nil {
				return nil, errors.Wrap(err, "failed to inject ipalloc_range")
			}
		}
	}

	cniRsc := capeirecipe.BuildCNIPlan(cni, manifests)
	b.AddResource("install:cni", cniRsc, plan.DependOn("kubeadm:init"))

	// Add resources to apply the cluster API's CRDs so that Kubernetes
	// understands objects like Cluster, Machine, etc.

	crdIDs, err := capeios.AddClusterAPICRDs(b, crds.CRDs)
	if err != nil {
		return nil, err
	}

	kubectlApplyDeps := append([]string{"kubeadm:init"}, crdIDs...)

	// If we're pulling data out of GitHub, we install sealed secrets and any auth secrets stored in sealed secrets
	configDeps, err := addSealedSecretResourcesIfNecessary(b, kubectlApplyDeps, pemSecretResources, sealedSecretVersion, params.SealedSecretKeyPath, params.SealedSecretCertPath, params.Namespace)
	if err != nil {
		return nil, err
	}

	// Set plan as an annotation on node, just like controller does
	seedNodePlan, err := seedNodeSetupPlan(ctx, o, params, &cluster.Spec, configMaps, authConfigMap, pemSecretResources, kubernetesVersion, kubernetesNamespace)
	if err != nil {
		return nil, err
	}
	b.AddResource("node:plan", &capeiresource.KubectlAnnotateSingleNode{Key: capeirecipe.PlanKey, Value: seedNodePlan.ToJSON()}, plan.DependOn("kubeadm:init"))

	addAuthConfigMapIfNecessary(configMapManifests, authConfigManifest)

	// Add config maps to system so controller can use them
	configMapPlan := recipe.BuildConfigMapPlan(configMapManifests, params.Namespace)

	b.AddResource("install:configmaps", configMapPlan, plan.DependOn(configDeps[0], configDeps[1:]...))

	applyClstrRsc := &capeiresource.KubectlApply{ManifestPath: object.String(params.ClusterManifestPath), Namespace: object.String(params.Namespace)}

	b.AddResource("kubectl:apply:cluster", applyClstrRsc, plan.DependOn("install:configmaps"))

	machinesManifest, err := machine.GetMachinesManifest(params.MachinesManifestPath)
	if err != nil {
		return nil, err
	}
	mManRsc := &capeiresource.KubectlApply{Manifest: []byte(machinesManifest), Filename: object.String("machinesmanifest"), Namespace: object.String(params.Namespace)}
	b.AddResource("kubectl:apply:machines", mManRsc, plan.DependOn(kubectlApplyDeps[0], kubectlApplyDeps[1:]...))

	dep := addSealedSecretWaitIfNecessary(b, params.SealedSecretKeyPath, params.SealedSecretCertPath)

	{
		capiCtlrManifest, err := capiControllerManifest(params.Controller, params.Namespace, params.ConfigDirectory)
		if err != nil {
			return nil, err
		}
		ctlrRsc := &capeiresource.KubectlApply{Manifest: capiCtlrManifest, Filename: object.String("capi_controller.yaml")}
		b.AddResource("install:capi", ctlrRsc, plan.DependOn("kubectl:apply:cluster", dep))
	}

	wksCtlrManifest, err := wksControllerManifest(params.Controller, params.Namespace, params.ConfigDirectory)
	if err != nil {
		return nil, err
	}

	ctlrRsc := &capeiresource.KubectlApply{Manifest: wksCtlrManifest, Filename: object.String("wks_controller.yaml")}
	b.AddResource("install:wks", ctlrRsc, plan.DependOn("kubectl:apply:cluster", dep))

	if err := configureFlux(b, params); err != nil {
		return nil, errors.Wrap(err, "Failed to configure flux")
	}

	// TODO move so this can also be performed when the user updates the cluster.  See issue https://github.com/weaveworks/wksctl/issues/440
	addons, err := parseAddons(params.ClusterManifestPath, params.Namespace, params.AddonNamespaces)
	if err != nil {
		return nil, err
	}

	addonRsc := recipe.BuildAddonPlan(params.ClusterManifestPath, addons)
	b.AddResource("install:addons", addonRsc, plan.DependOn("kubectl:apply:cluster", "kubectl:apply:machines"))

	return capeios.CreatePlan(b)
}

// Sets the pod CIDR block in the weave-net manifest
func SetWeaveNetPodCIDRBlock(manifests [][]byte, podsCIDRBlock string) ([][]byte, error) {
	// Weave-Net has a container named weave in its daemonset
	containerName := "weave"
	// The pod CIDR block is set via the IPALLOC_RANGE env var
	podCIDRBlock := &v1.EnvVar{
		Name:  "IPALLOC_RANGE",
		Value: podsCIDRBlock,
	}

	manifestList := &v1.List{}
	err := yaml.Unmarshal(manifests[0], manifestList)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal weave-net manifest")
	}

	// Find and parse the DaemonSet included in the manifest list into an object
	idx, daemonSet, err := findDaemonSet(manifestList)
	if err != nil {
		return nil, errors.New("failed to find daemonset in weave-net manifest")
	}

	err = injectEnvVarToContainer(daemonSet.Spec.Template.Spec.Containers, containerName, *podCIDRBlock)
	if err != nil {
		return nil, errors.Wrap(err, "failed to inject env var to weave container")
	}

	manifestList.Items[idx] = runtime.RawExtension{Object: daemonSet}

	manifests[0], err = yaml.Marshal(manifestList)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal weave-net manifest list")
	}

	return manifests, nil
}

// Finds container in the list by name, adds an env var, fails if env var exists with different value
func injectEnvVarToContainer(
	containers []v1.Container, name string, newEnvVar v1.EnvVar) error {
	var targetContainer v1.Container
	containerFound := false
	var idx int
	var container v1.Container

	for idx, container = range containers {
		if container.Name == name {
			targetContainer = container
			containerFound = true
			break
		}
	}
	if !containerFound {
		return errors.New(fmt.Sprintf("did not find container %s in manifest", name))
	}

	envVars := targetContainer.Env
	for _, envVar := range envVars {
		if envVar.Name == newEnvVar.Name {
			if envVar.Value != newEnvVar.Value {
				return errors.New(
					fmt.Sprintf("manifest already contains env var %s, and cannot overwrite", newEnvVar.Name))
			}
			return nil
		}
	}
	targetContainer.Env = append(envVars, newEnvVar)
	containers[idx] = targetContainer

	return nil
}

// Returns a daemonset manifest from a list
func findDaemonSet(manifest *v1.List) (int, *appsv1.DaemonSet, error) {
	if manifest == nil {
		return -1, nil, errors.New("manifest is nil")
	}
	daemonSet := &appsv1.DaemonSet{}
	var err error
	var idx int
	var item runtime.RawExtension
	for idx, item = range manifest.Items {
		err := yaml.Unmarshal(item.Raw, daemonSet)
		if err == nil && daemonSet.Kind == "DaemonSet" {
			break
		}
	}
	if err != nil {
		return err
	}
	if sp != nil {
		b := plan.NewBuilder()
		b.AddResource("install:secret-support", sp)
		b.AddResource("install:seed-node", p)
		plan, err := b.Plan()
		if err != nil {
			return err
		}
		p = &plan
	}
	return capeios.ApplyPlan(o, p)
}

func getAPIServerArgs(providerSpec *capeiv1alpha3.ClusterSpec, pemSecretResources map[string]*secretResourceSpec) map[string]string {
	result := map[string]string{}
	authnResourceSpec := pemSecretResources["authentication"]
	if authnResourceSpec != nil {
		storeIfNotEmpty(result, "authentication-token-webhook-config-file", filepath.Join(capeios.ConfigDestDir, authnResourceSpec.secretName+".yaml"))
		storeIfNotEmpty(result, "authentication-token-webhook-cache-ttl", providerSpec.Authentication.CacheTTL)
	}
	authzResourceSpec := pemSecretResources["authorization"]
	if authzResourceSpec != nil {
		result["authorization-mode"] = "Webhook"
		storeIfNotEmpty(result, "authorization-webhook-config-file", filepath.Join(capeios.ConfigDestDir, authzResourceSpec.secretName+".yaml"))
		storeIfNotEmpty(result, "authorization-webhook-cache-unauthorized-ttl", providerSpec.Authorization.CacheUnauthorizedTTL)
		storeIfNotEmpty(result, "authorization-webhook-cache-authorized-ttl", providerSpec.Authorization.CacheAuthorizedTTL)
	}

	// Also add any explicit api server arguments from the generic section
	for _, arg := range providerSpec.APIServer.ExtraArguments {
		result[arg.Name] = arg.Value
	}
	return result
}

func addClusterAPICRDs(b *plan.Builder) ([]string, error) {
	crds, err := getCRDs()

	if err != nil {
		return nil, errors.Wrap(err, "failed to list cluster API CRDs")
	}
	crdIDs := make([]string, 0)
	for _, crdFile := range crds {
		id := fmt.Sprintf("kubectl:apply:%s", crdFile.fname)
		crdIDs = append(crdIDs, id)
		rsrc := &resource.KubectlApply{Filename: object.String(crdFile.fname), Manifest: crdFile.data, WaitCondition: "condition=Established"}
		b.AddResource(id, rsrc, plan.DependOn("kubeadm:init"))
	}
	return crdIDs, nil
}

func augmentParamsWithPool(user, key string, eim []*existinginfrav1.ExistingInfraMachine, params capeios.SeedNodeParams) capeios.SeedNodeParams {
	info := []capeios.MachineInfo{}
	for _, m := range eim {
		info = append(info, capeios.MachineInfo{
			SSHUser:     user,
			SSHKey:      key,
			PublicIP:    m.Spec.Public.Address,
			PublicPort:  fmt.Sprintf("%d", m.Spec.Public.Port),
			PrivateIP:   m.Spec.Private.Address,
			PrivatePort: fmt.Sprintf("%d", m.Spec.Private.Port)})
	}
	params.ConnectionInfo = info
	return params
}

//updateSecretParams creates secret resources for auth(n/z) which get added to the seed node plan
func createSecretPlan(o *capeios.OS, params capeios.SeedNodeParams) (plan.Resource, capeios.SeedNodeParams, error) {
	pemPlan, pemSecretResources, authConfigMap, authConfigManifest, err := processPemFilesIfAny(&params.ExistingInfraCluster.Spec, params.ConfigDirectory, params.Namespace, params.SealedSecretKey, params.SealedSecretCert)
	if err != nil {
		return nil, nil, err
	}
	info := &capeios.AuthParams{PEMSecretResources: pemSecretResources, AuthConfigMap: authConfigMap, AuthConfigManifest: authConfigManifest}
	newParams := params
	newParams.AuthInfo = info
	return pemPlan, newParams, nil
}

// processPemFilesIfAny reads the SealedSecret from the config
// directory, decrypts it using the GitHub deploy key, creates file
// resources for .pem files stored in the secret, and creates a SealedSecret resource
// for them that can be used by the machine actuator
func processPemFilesIfAny(providerSpec *existinginfrav1.ClusterSpec, configDir string, ns, privateKey, cert string) (plan.Resource, map[string]*capeios.SecretResourceSpec, *v1.ConfigMap, []byte, error) {
	if err := checkPemValues(providerSpec, privateKey, cert); err != nil {
		return nil, nil, nil, nil, err
	}
	if providerSpec.Authentication == nil && providerSpec.Authorization == nil {
		// no auth specified
		return nil, nil, nil, nil, nil
	}
	b := plan.NewBuilder()
	b.AddResource("create:pem-dir", &capeiresource.Dir{Path: object.String(capeios.PemDestDir)})
	b.AddResource("set-perms:pem-dir", &capeiresource.Run{Script: object.String(fmt.Sprintf("chmod 600 %s", capeios.PemDestDir))}, plan.DependOn("create:pem-dir"))
	rsaPrivateKey, err := getPrivateKey(privateKey)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var authenticationSecretFileName, authorizationSecretFileName, authenticationSecretName, authorizationSecretName string
	var authenticationSecretManifest, authorizationSecretManifest, authenticationConfig, authorizationConfig []byte
	var decrypted map[string][]byte
	secretResources := map[string]*capeios.SecretResourceSpec{}
	if providerSpec.Authentication != nil {
		authenticationSecretFileName = providerSpec.Authentication.SecretFile
		authenticationSecretManifest, decrypted, authenticationSecretName, authenticationConfig, err = processSecret(
			b, rsaPrivateKey, configDir, authenticationSecretFileName, providerSpec.Authentication.URL)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		secretResources["authentication"] = &capeios.SecretResourceSpec{
			SecretName: authenticationSecretName,
			Decrypted:  decrypted,
			Resource:   &capeiresource.KubectlApply{Namespace: object.String(ns), Manifest: authenticationSecretManifest, Filename: object.String(authenticationSecretName)}}
	}
	if providerSpec.Authorization != nil {
		authorizationSecretFileName = providerSpec.Authorization.SecretFile
		authorizationSecretManifest, decrypted, authorizationSecretName, authorizationConfig, err = processSecret(
			b, rsaPrivateKey, configDir, authorizationSecretFileName, providerSpec.Authorization.URL)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		secretResources["authorization"] = &capeios.SecretResourceSpec{
			SecretName: authorizationSecretName,
			Decrypted:  decrypted,
			Resource:   &capeiresource.KubectlApply{Namespace: object.String(ns), Manifest: authorizationSecretManifest, Filename: object.String(authorizationSecretName)}}
	}
	filePlan, err := b.Plan()
	if err != nil {
		log.Infof("Plan creation failed:\n%s\n", err)
		return nil, nil, nil, nil, err
	}
	authConfigMap, authConfigMapManifest, err := createAuthConfigMapManifest(authenticationSecretName, authorizationSecretName,
		authenticationConfig, authorizationConfig)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return &filePlan, secretResources, authConfigMap, authConfigMapManifest, nil
}

func getPrivateKey(privateKey string) (*rsa.PrivateKey, error) {
	privateKeyBytes := []byte(privateKey)
	privateKeyData, err := keyutil.ParsePrivateKeyPEM(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	rsaPrivateKey, ok := privateKeyData.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("Invalid private key")
	}
	return rsaPrivateKey, nil
}

func checkPemValues(providerSpec *existinginfrav1.ClusterSpec, privateKey, cert string) error {
	if privateKey == "" || cert == "" {
		if providerSpec.Authentication != nil || providerSpec.Authorization != nil {
			return errors.New("Encryption keys not specified; cannot process authentication and authorization specifications.")
		}
	}
	if (providerSpec.Authentication != nil && providerSpec.Authentication.SecretFile == "") ||
		(providerSpec.Authorization != nil && providerSpec.Authorization.SecretFile == "") {
		return errors.New("A secret must be specified to configure an authentication or authorization specification.")
	}
	return nil
}

func createAuthConfigMapManifest(authnSecretName, authzSecretName string, authnConfig, authzConfig []byte) (*v1.ConfigMap, []byte, error) {
	data := map[string]string{}
	capeios.StoreIfNotEmpty(data, "authentication-secret-name", authnSecretName)
	capeios.StoreIfNotEmpty(data, "authorization-secret-name", authzSecretName)
	capeios.StoreIfNotEmpty(data, "authentication-config", string(authnConfig))
	capeios.StoreIfNotEmpty(data, "authorization-config", string(authzConfig))
	cm := v1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "auth-config"},
		Data:       data,
	}
	manifest, err := yaml.Marshal(cm)
	if err != nil {
		return nil, nil, err
	}
	return &cm, manifest, nil
}

// Decrypts secret, adds plan resources to install files found inside, plus a kubeconfig file pointing to them.
// returns the sealed file contents, decrypted contents, secret name, kubeconfig, error if any
func processSecret(b *plan.Builder, key *rsa.PrivateKey, configDir, secretFileName, URL string) ([]byte, map[string][]byte, string, []byte, error) {
	// Read the file contents at configDir/secretFileName
	contents, err := getConfigFileContents(configDir, secretFileName)
	if err != nil {
		return nil, nil, "", nil, err
	}

	// Create a new YAML FrameReader from the given bytes
	fr := serializer.NewYAMLFrameReader(serializer.FromBytes(contents))
	// Create the secret to decode into
	ss := &ssv1alpha1.SealedSecret{}
	// Decode the Sealed Secret into the object
	// In the future, if we wish to support other kinds of secrets than SealedSecrets, we
	// can just change this to do .Decode(fr), and switch on the type
	if err := scheme.Serializer.Decoder().DecodeInto(fr, ss); err != nil {
		return nil, nil, "", nil, errors.Wrapf(err, "couldn't decode the file %q into a sealed secret", secretFileName)
	}

	fingerprint, err := crypto.PublicKeyFingerprint(&key.PublicKey)
	if err != nil {
		return nil, nil, "", nil, err
	}
	keys := map[string]*rsa.PrivateKey{fingerprint: key}

	codecs := scheme.Serializer.Codecs()
	if codecs == nil {
		return nil, nil, "", nil, fmt.Errorf("codecs must not be nil")
	}
	secret, err := ss.Unseal(*codecs, keys)
	if err != nil {
		return nil, nil, "", nil, errors.Wrap(err, "Could not unseal auth secret")
	}
	decrypted := map[string][]byte{}
	secretName := secret.Name
	for _, key := range pemKeys {
		fileContents, ok := secret.Data[key]
		if !ok {
			return nil, nil, "", nil, fmt.Errorf("Missing auth config value for: %q in secret %q", key, secretName)
		}
		resName := secretName + "-" + key
		fileName := filepath.Join(capeios.PemDestDir, secretName, key+".pem")
		b.AddResource("install:"+resName, &capeiresource.File{Content: string(fileContents), Destination: fileName}, plan.DependOn("set-perms:pem-dir"))
		decrypted[key] = fileContents
	}
	contextName := secretName + "-webhook"
	userName := secretName + "-api-server"
	config := &clientcmdapi.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: map[string]*clientcmdapi.Cluster{
			secretName: {
				CertificateAuthority: filepath.Join(capeios.PemDestDir, secretName, "certificate-authority.pem"),
				Server:               URL,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			userName: {
				ClientCertificate: filepath.Join(capeios.PemDestDir, secretName, "client-certificate.pem"),
				ClientKey:         filepath.Join(capeios.PemDestDir, secretName, "client-key.pem"),
			},
		},
		CurrentContext: contextName,
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  secretName,
				AuthInfo: userName,
			},
		},
	}
	authConfig, err := clientcmd.Write(*config)
	if err != nil {
		return nil, nil, "", nil, err
	}
	configResource := &capeiresource.File{Content: string(authConfig), Destination: filepath.Join(capeios.ConfigDestDir, secretName+".yaml")}
	b.AddResource("install:"+secretName, configResource, plan.DependOn("set-perms:pem-dir"))

	return contents, decrypted, secretName, authConfig, nil
}

func createSealedSecretKeySecretManifest(privateKey, cert, ns string) ([]byte, error) {
	secret := &v1.Secret{
		TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: sealedSecretKeySecretName, Namespace: "kube-system"},
		Type:       v1.SecretTypeOpaque,
	}
	secret.Data = map[string][]byte{}
	secret.StringData = map[string]string{}
	secret.StringData[v1.TLSPrivateKeyKey] = privateKey
	secret.StringData[v1.TLSCertKey] = cert
	return yaml.Marshal(secret)
}

// processDeployKey adds the encoded deploy key to the set of parameters used to configure flux
func processDeployKey(params map[string]string, gitDeployKeyPath string) error {
	if gitDeployKeyPath == "" {
		return nil
	}
	b64Key, err := readAndBase64EncodeKey(gitDeployKeyPath)
	if err != nil {
		return err
	}
	params["gitDeployKey"] = b64Key
	return nil
}

func readAndBase64EncodeKey(keypath string) (string, error) {
	content, err := ioutil.ReadFile(keypath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(content), nil
}

// getConfigFileContents reads a config manifest from a file in the config directory.
func getConfigFileContents(fileNameComponent ...string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(fileNameComponent...))
}
