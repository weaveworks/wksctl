package os

import (
	"bytes"
	"crypto/rsa"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	ssv1alpha1 "github.com/bitnami-labs/sealed-secrets/pkg/apis/sealed-secrets/v1alpha1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/addons"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/controller/manifests"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/crds"
	baremetalspecv1 "github.com/weaveworks/wksctl/pkg/baremetalproviderspec/v1alpha1"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/recipe"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/plan/runners/sudo"
	"github.com/weaveworks/wksctl/pkg/utilities/envcfg"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/utilities/object"
	v1beta2 "k8s.io/api/apps/v1beta2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certUtil "k8s.io/client-go/util/cert"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/yaml"
)

const (
	SeedNodePlanName          = "seed-node-standard-plan"
	ConfigDestDir             = "/etc/pki/weaveworks/wksctl"
	PemDestDir                = "/etc/pki/weaveworks/wksctl/pem"
	authCmapName              = "authn-authz"
	masterConfigKey           = "master-config"
	sealedSecretVersion       = "v0.7.0"
	sealedSecretKeySecretName = "sealed-secrets-key"
)

var (
	pemKeys            = []string{"certificate-authority", "client-certificate", "client-key"}
	fluxSecretTemplate = `apiVersion: v1
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

// OS represents an operating system and exposes the operations required to
// install Kubernetes on a machine setup with that OS.
type OS struct {
	Name    string
	runner  plan.Runner
	PkgType resource.PkgType
}

// Identifiers groups the various pieces of data usable to uniquely identify a
// machine in a cluster.
type Identifiers struct {
	MachineID  string
	SystemUUID string
}

// IDs returns this machine's ID (see also OS#GetMachineID) and system UUID (see
// also: OS#GetSystemUUID).
func (o OS) IDs() (*Identifiers, error) {
	osres, err := resource.NewOS(o.runner)
	if err != nil {
		return nil, err
	}
	machineID, err := osres.GetMachineID(o.runner)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read machine's ID")
	}
	systemUUID, err := osres.GetSystemUUID(o.runner)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read machine's system UUID")
	}
	return &Identifiers{MachineID: machineID, SystemUUID: systemUUID}, nil
}

type crdFile struct {
	fname string
	data  []byte
}

// Retrieve all CRD definitions needed for cluster API
func getCRDs() ([]crdFile, error) {
	crddir, err := crds.CRDs.Open(".")
	if err != nil {
		return nil, errors.Wrap(err, "failed to list cluster API CRDs")
	}
	crdFiles := make([]crdFile, 0)
	for {
		entry, err := crddir.Readdir(1)
		if err != nil && err != io.EOF {
			return nil, errors.Wrap(err, "failed to open cluster API CRD directory")
		}
		if entry == nil {
			break
		}
		fname := entry[0].Name()
		crd, err := crds.CRDs.Open(fname)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open cluster API CRD")
		}
		data, err := ioutil.ReadAll(crd)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read cluster API CRD")
		}
		crdFiles = append(crdFiles, crdFile{fname, data})
	}
	return crdFiles, nil
}

// GitParams are all SeedNodeParams related to the user's Git(Hub) repo
type GitParams struct {
	GitURL           string
	GitBranch        string
	GitPath          string
	GitDeployKeyPath string
}

// ControllerParams are all SeedNodeParams related to the WKS controller
type ControllerParams struct {
	// ImageOverride will override the WKS controller image if set. It will do so
	// whether the controller manifest comes from a git repository or is the
	// built-in one.
	ImageOverride string
	// ImageBuiltin is the WKS controller image to use when generating the WKS
	// controller manifest from in-memory data.
	ImageBuiltin string
}

// SeedNodeParams groups required inputs to configure a "seed" Kubernetes node.
type SeedNodeParams struct {
	PublicIP             string
	PrivateIP            string
	ClusterManifestPath  string
	MachinesManifestPath string
	SSHKeyPath           string
	// BootstrapToken is the token used by kubeadm init and kubeadm join
	// to safely form new clusters.
	BootstrapToken       *kubeadmapi.BootstrapTokenString
	KubeletConfig        config.KubeletConfig
	Controller           ControllerParams
	GitData              GitParams
	SealedSecretKeyPath  string
	SealedSecretCertPath string
	ConfigDirectory      string
	Namespace            string
	ImageRepository      string
	ExternalLoadBalancer string
	AdditionalSANs       []string
}

// Validate generally validates this SeedNodeParams struct, e.g. ensures it
// contains mandatory values, that these are well-formed, etc.
func (params SeedNodeParams) Validate() error {
	if len(params.KubeletConfig.NodeIP) == 0 {
		return errors.New("empty kubelet node IP")
	}
	if len(params.PublicIP) == 0 {
		return errors.New("empty API server public IP")
	}
	if len(params.PrivateIP) == 0 {
		return errors.New("empty API server private IP")
	}
	return nil
}

// SetupSeedNode installs Kubernetes on this machine, and store the provided
// manifests in the API server, so that the rest of the cluster can then be
// set up by the WKS controller.
func (o OS) SetupSeedNode(params SeedNodeParams) error {
	p, err := o.CreateSeedNodeSetupPlan(params)
	if err != nil {
		return err
	}
	return o.applySeedNodePlan(p)
}

// CreateSeedNodeSetupPlan constructs the seed node plan used to setup the initial node
// prior to turning control over to wks-controller
func (o OS) CreateSeedNodeSetupPlan(params SeedNodeParams) (*plan.Plan, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	cfg, err := envcfg.GetEnvSpecificConfig(o.PkgType, params.Namespace, params.KubeletConfig.CloudProvider, o.runner)
	if err != nil {
		return nil, err
	}
	kubernetesVersion, err := machine.GetKubernetesVersionFromManifest(params.MachinesManifestPath)
	if err != nil {
		return nil, err
	}
	providerSpec, err := getClusterProviderSpec(params.ClusterManifestPath)
	if err != nil {
		return nil, err
	}
	// Get configuration file resources from config map manifests referenced by the cluster spec
	configMapManifests, configMaps, configFileResources, err := createConfigFileResourcesFromFiles(providerSpec, params.ConfigDirectory, params.Namespace)
	if err != nil {
		return nil, err
	}

	b := plan.NewBuilder()

	baseRes := recipe.BuildBasePlan(o.PkgType)
	b.AddResource("install:base", baseRes)

	configRes := recipe.BuildConfigPlan(configFileResources)
	b.AddResource("install:config", configRes, plan.DependOn("install:base"))

	pemSecretResources, authConfigMap, authConfigManifest, err := processPemFilesIfAny(b, providerSpec, params.ConfigDirectory, params.Namespace, params.SealedSecretKeyPath, params.SealedSecretCertPath)
	if err != nil {
		return nil, err
	}

	criRes := recipe.BuildCRIPlan(&providerSpec.CRI, cfg, o.PkgType)
	b.AddResource("install:cri", criRes, plan.DependOn("install:config"))

	k8sRes := recipe.BuildK8SPlan(kubernetesVersion, params.KubeletConfig.NodeIP, cfg.SetSELinuxPermissive, cfg.DisableSwap, cfg.LockYUMPkgs, o.PkgType, params.KubeletConfig.CloudProvider)
	b.AddResource("install:k8s", k8sRes, plan.DependOn("install:cri"))

	apiServerArgs := getAPIServerArgs(providerSpec, pemSecretResources)

	controlPlaneEndpointIP := params.ExternalLoadBalancer
	if controlPlaneEndpointIP == "" {
		controlPlaneEndpointIP = params.PrivateIP
	}
	kubeadmInitResource :=
		&resource.KubeadmInit{
			PublicIP:       params.PublicIP,
			PrivateIP:      params.PrivateIP,
			KubeletConfig:  &params.KubeletConfig,
			ConntrackMax:   cfg.ConntrackMax,
			UseIPTables:    cfg.UseIPTables,
			SSHKeyPath:     params.SSHKeyPath,
			BootstrapToken: params.BootstrapToken,
			// TODO: dynamically inject the API server's port.
			ControlPlaneEndpoint:  fmt.Sprintf("%s:6443", controlPlaneEndpointIP),
			IgnorePreflightErrors: cfg.IgnorePreflightErrors,
			KubernetesVersion:     kubernetesVersion,
			CloudProvider:         params.KubeletConfig.CloudProvider,
			ImageRepository:       params.ImageRepository,
			ExternalLoadBalancer:  params.ExternalLoadBalancer,
			AdditionalSANs:        params.AdditionalSANs,
			Namespace:             object.String(params.Namespace),
			NodeName:              cfg.HostnameOverride,
			ExtraAPIServerArgs:    apiServerArgs,
		}
	b.AddResource("kubeadm:init", kubeadmInitResource, plan.DependOn("install:k8s"))

	// TODO(damien): Add a CNI section in cluster.yaml once we support more than one CNI plugin.
	const cni = "weave-net"

	cniAdddon := baremetalspecv1.Addon{Name: cni}
	manifests, err := buildAddon(cniAdddon, params.ImageRepository, params.ClusterManifestPath, params.Namespace)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate manifests for CNI plugin")
	}

	cniRsc := recipe.BuildCNIPlan(cni, manifests)
	b.AddResource("install:cni", cniRsc, plan.DependOn("kubeadm:init"))

	// Add resources to apply the cluster API's CRDs so that Kubernetes
	// understands objects like Cluster, Machine, etc.

	crdIDs, err := addClusterAPICRDs(b)
	if err != nil {
		return nil, err
	}

	kubectlApplyDeps := append([]string{"kubeadm:init"}, crdIDs...)

	// If we're pulling data out of GitHub, we install sealed secrets and any auth secrets stored in sealed secrets
	configDeps, err := addSealedSecretResourcesIfNecessary(b, kubectlApplyDeps, pemSecretResources, sealedSecretVersion, params.SealedSecretKeyPath, params.SealedSecretCertPath, params.Namespace)
	if err != nil {
		return nil, err
	}

	// Create a config map containing a standard node plan for the seed node so the controller can add it
	// to the seed node as an annotation. Otherwise, we won't be able to determine if the seed node is changed
	// in a later update (this removes a dependency on flux)
	seedNodePlanConfigMapManifest, err := o.createSeedNodePlanConfigMapManifest(params, providerSpec, configMaps, authConfigMap, kubernetesVersion)
	if err != nil {
		return nil, err
	}

	configMapManifests["seed-node-plan"] = seedNodePlanConfigMapManifest
	addAuthConfigMapIfNecessary(configMapManifests, authConfigManifest)

	// Add config maps to system so controller can use them
	configMapPlan := recipe.BuildConfigMapPlan(configMapManifests, params.Namespace)

	b.AddResource("install:configmaps", configMapPlan, plan.DependOn(configDeps[0], configDeps[1:]...))

	applyClstrRsc := &resource.KubectlApply{ManifestPath: object.String(params.ClusterManifestPath), Namespace: object.String(params.Namespace)}

	b.AddResource("kubectl:apply:cluster", applyClstrRsc, plan.DependOn("install:configmaps"))

	machinesManifest, err := machine.GetMachinesManifest(params.MachinesManifestPath)
	if err != nil {
		return nil, err
	}
	mManRsc := &resource.KubectlApply{Manifest: []byte(machinesManifest), Filename: object.String("machinesmanifest"), Namespace: object.String(params.Namespace)}
	b.AddResource("kubectl:apply:machines", mManRsc, plan.DependOn(kubectlApplyDeps[0], kubectlApplyDeps[1:]...))

	wksCtlrManifest, err := wksControllerManifest(params.Controller, params.Namespace, params.ConfigDirectory)
	if err != nil {
		return nil, err
	}

	dep := addSealedSecretWaitIfNecessary(b, params.SealedSecretKeyPath, params.SealedSecretCertPath)
	ctlrRsc := &resource.KubectlApply{Manifest: wksCtlrManifest, Filename: object.String("wks_controller.yaml")}
	b.AddResource("install:wks", ctlrRsc, plan.DependOn("kubectl:apply:cluster", dep))

	if err := o.configureFlux(b, params); err != nil {
		return nil, errors.Wrap(err, "Failed to configure flux")
	}

	// TODO move so this can also be performed when the user updates the cluster.  See issue https://github.com/weaveworks/wksctl/issues/440
	addons, err := parseAddons(params.ClusterManifestPath, params.Namespace)
	if err != nil {
		return nil, err
	}

	addonRsc := recipe.BuildAddonPlan(params.ClusterManifestPath, addons)
	b.AddResource("install:addons", addonRsc, plan.DependOn("kubectl:apply:cluster", "kubectl:apply:machines"))

	return createPlan(b)
}

func addAuthConfigMapIfNecessary(configMapManifests map[string][]byte, authConfigManifest []byte) {
	if authConfigManifest != nil {
		configMapManifests["auth-config"] = authConfigManifest
	}
}

func addSealedSecretWaitIfNecessary(b *plan.Builder, keyPath, certPath string) string {
	if keyPath != "" && certPath != "" {
		b.AddResource("wait:sealed-secrets-controller",
			&resource.KubectlWait{WaitNamespace: "kube-system", WaitType: "pods", WaitSelector: "name=sealed-secrets-controller",
				WaitCondition: "condition=Ready", WaitTimeout: "300s"},
			plan.DependOn("kubectl:apply:machines"))
		return "wait:sealed-secrets-controller"
	}
	return "kubectl:apply:machines"
}

func addSealedSecretResourcesIfNecessary(b *plan.Builder, kubectlApplyDeps []string, pemSecretResources map[string]*secretResourceSpec, sealedSecretVersion, keyPath, certPath, ns string) ([]string, error) {
	if keyPath != "" && certPath != "" {
		privateKeyBytes, err := getConfigFileContents(keyPath)
		if err != nil {
			return nil, errors.Wrap(err, "Could not read private key")
		}
		certBytes, err := getConfigFileContents(certPath)
		if err != nil {
			return nil, errors.Wrap(err, "Could not read cert")
		}
		manifest, err := createSealedSecretKeySecretManifest(string(privateKeyBytes), string(certBytes), ns)
		if err != nil {
			return nil, err
		}
		sealedSecretRsc := recipe.BuildSealedSecretPlan(sealedSecretVersion, ns, manifest)
		b.AddResource("install:sealed-secrets", sealedSecretRsc, plan.DependOn(kubectlApplyDeps[0], kubectlApplyDeps[1:]...))

		// Now that the cluster is up, if auth is configured, create a secret containing the data for use by the machine actuator
		if pemSecretResources != nil {
			for _, resourceSpec := range pemSecretResources {
				b.AddResource(fmt.Sprintf("install:pem-secret-%s", resourceSpec.secretName), resourceSpec.resource, plan.DependOn("install:sealed-secrets"))
			}
		}
		return []string{"install:sealed-secrets"}, nil
	}
	return kubectlApplyDeps, nil
}

func storeIfNotEmpty(vals map[string]string, key, value string) {
	if value != "" {
		vals[key] = value
	}
}

func getAPIServerArgs(providerSpec *baremetalspecv1.BareMetalClusterProviderSpec, pemSecretResources map[string]*secretResourceSpec) map[string]string {
	result := map[string]string{}
	authnResourceSpec := pemSecretResources["authentication"]
	if authnResourceSpec != nil {
		storeIfNotEmpty(result, "authentication-token-webhook-config-file", filepath.Join(ConfigDestDir, authnResourceSpec.secretName+".yaml"))
		storeIfNotEmpty(result, "authentication-token-webhook-cache-ttl", providerSpec.Authentication.CacheTTL)
	}
	authzResourceSpec := pemSecretResources["authorization"]
	if authzResourceSpec != nil {
		result["authorization-mode"] = "Webhook"
		storeIfNotEmpty(result, "authorization-webhook-config-file", filepath.Join(ConfigDestDir, authzResourceSpec.secretName+".yaml"))
		storeIfNotEmpty(result, "authorization-webhook-cache-unauthorized-ttl", providerSpec.Authorization.CacheUnauthorizedTTL)
		storeIfNotEmpty(result, "authorization-webhook-cache-authorized-ttl", providerSpec.Authorization.CacheAuthorizedTTL)
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

func (o OS) createSeedNodePlanConfigMapManifest(params SeedNodeParams, providerSpec *baremetalspecv1.BareMetalClusterProviderSpec, providerConfigMaps map[string]*v1.ConfigMap, authConfigMap *v1.ConfigMap, kubernetesVersion string) ([]byte, error) {
	nodeParams := NodeParams{
		IsMaster:             true,
		MasterIP:             params.PrivateIP,
		MasterPort:           6443, // See TODO in machine_actuator.go
		KubeletConfig:        params.KubeletConfig,
		KubernetesVersion:    kubernetesVersion,
		CRI:                  providerSpec.CRI,
		ConfigFileSpecs:      providerSpec.OS.Files,
		ProviderConfigMaps:   providerConfigMaps,
		AuthConfigMap:        authConfigMap,
		Namespace:            params.Namespace,
		ExternalLoadBalancer: providerSpec.APIServer.ExternalLoadBalancer,
	}
	var paramBuffer bytes.Buffer
	err := gob.NewEncoder(&paramBuffer).Encode(nodeParams)
	if err != nil {
		return nil, err
	}
	if params.Namespace == "" {
		params.Namespace, err = machine.GetKubernetesNamespaceFromMachines()
		if err != nil {
			return nil, err
		}
	}

	seedNodePlanConfigMapManifest, err := planParametersToConfigMapManifest(paramBuffer.Bytes(), params.Namespace)
	if err != nil {
		return nil, err
	}
	return seedNodePlanConfigMapManifest, nil
}

func (o OS) applySeedNodePlan(p *plan.Plan) error {
	err := p.Undo(o.runner, plan.EmptyState)
	if err != nil {
		log.Infof("Pre-plan cleanup failed:\n%s\n", err)
		return err
	}

	_, err = p.Apply(o.runner, plan.EmptyDiff())
	if err != nil {
		log.Errorf("Apply of Plan failed:\n%s\n", err)
	}
	return nil
}

func getClusterProviderSpec(manifestPath string) (*baremetalspecv1.BareMetalClusterProviderSpec, error) {
	cluster, err := parseCluster(manifestPath)
	if err != nil {
		return nil, err
	}
	codec, err := baremetalspecv1.NewCodec()
	if err != nil {
		return nil, err
	}
	providerSpec, err := codec.ClusterProviderFromProviderSpec(cluster.Spec.ProviderSpec)
	if err != nil {
		return nil, err
	}
	return providerSpec, nil
}

func planParametersToConfigMapManifest(plan []byte, ns string) ([]byte, error) {
	cm := v1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: SeedNodePlanName, Namespace: ns},
		BinaryData: map[string][]byte{"plan": plan},
	}
	return yaml.Marshal(cm)
}

func createConfigFileResourcesFromFiles(providerSpec *baremetalspecv1.BareMetalClusterProviderSpec, configDir, namespace string) (map[string][]byte, map[string]*v1.ConfigMap, []*resource.File, error) {
	fileSpecs := providerSpec.OS.Files
	configMapManifests, err := getConfigMapManifests(fileSpecs, configDir, namespace)
	if err != nil {
		return nil, nil, nil, err
	}
	configMaps := make(map[string]*v1.ConfigMap)
	for name, manifest := range configMapManifests {
		cmap, err := getConfigMap(manifest)
		if err != nil {
			return nil, nil, nil, err
		}
		configMaps[name] = cmap
	}
	resources, err := createConfigFileResourcesFromConfigMaps(fileSpecs, configMaps)
	if err != nil {
		return nil, nil, nil, err
	}
	return configMapManifests, configMaps, resources, nil
}

func createConfigFileResourcesFromConfigMaps(fileSpecs []baremetalspecv1.FileSpec, configMaps map[string]*v1.ConfigMap) ([]*resource.File, error) {
	fileResources := make([]*resource.File, len(fileSpecs))
	for idx, file := range fileSpecs {
		source := &file.Source
		fileResource := &resource.File{Destination: file.Destination}
		fileContents, ok := configMaps[source.ConfigMap].Data[source.Key]
		if ok {
			fileResource.Content = fileContents
			fileResources[idx] = fileResource
			continue
		}
		// if not in Data, check BinaryData
		binaryContents, ok := configMaps[source.ConfigMap].BinaryData[source.Key]
		if !ok {
			return nil, fmt.Errorf("No config data for filespec: %v", file)
		}
		fileResource.Content = string(binaryContents)
		fileResources[idx] = fileResource
	}
	return fileResources, nil
}

func getConfigMapManifests(fileSpecs []baremetalspecv1.FileSpec, configDir, namespace string) (map[string][]byte, error) {
	configMapManifests := map[string][]byte{}
	for _, fileSpec := range fileSpecs {
		mapName := fileSpec.Source.ConfigMap
		if _, ok := configMapManifests[mapName]; !ok {
			manifest, err := getConfigMapManifest(configDir, mapName, namespace)
			if err != nil {
				return nil, err
			}
			configMapManifests[mapName] = manifest
		}
	}
	return configMapManifests, nil
}

func getConfigMap(manifest []byte) (*v1.ConfigMap, error) {
	configMap := &v1.ConfigMap{}
	if err := yaml.Unmarshal(manifest, configMap); err != nil {
		return nil, errors.Wrapf(err, "failed to parse config:\n%s", manifest)
	}
	return configMap, nil
}

// getConfigMapManifest reads a config map manifest from a file in the config directory. The file should be named:
// "<mapName>-config.yaml"
func getConfigMapManifest(configDir, mapName, namespace string) ([]byte, error) {
	bytes, err := getConfigFileContents(configDir, mapName+"-config.yaml")
	if err != nil {
		return nil, err
	}
	content, err := manifest.WithNamespace(string(bytes), namespace)
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// getConfigFileContents reads a config manifest from a file in the config directory.
func getConfigFileContents(fileNameComponent ...string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(fileNameComponent...))
}

type secretResourceSpec struct {
	secretName string
	resource   plan.Resource
}

// processPemFilesIfAny reads the SealedSecret from the config
// directory, decrypts it using the GitHub deploy key, creates file
// resources for .pem files stored in the secret, and creates a SealedSecret resource
// for them that can be used by the machine actuator
func processPemFilesIfAny(builder *plan.Builder, providerSpec *baremetalspecv1.BareMetalClusterProviderSpec, configDir string, ns, privateKeyPath, certPath string) (map[string]*secretResourceSpec, *v1.ConfigMap, []byte, error) {
	if err := checkPemValues(providerSpec, privateKeyPath, certPath); err != nil {
		return nil, nil, nil, err
	}
	if providerSpec.Authentication == nil && providerSpec.Authorization == nil {
		// no auth specified
		return nil, nil, nil, nil
	}
	b := plan.NewBuilder()
	b.AddResource("create:pem-dir", &resource.Dir{Path: object.String(PemDestDir)})
	b.AddResource("set-perms:pem-dir", &resource.Run{Script: object.String(fmt.Sprintf("chmod 600 %s", PemDestDir))}, plan.DependOn("create:pem-dir"))
	privateKey, err := getPrivateKey(privateKeyPath)
	if err != nil {
		return nil, nil, nil, err
	}
	var authenticationSecretFileName, authorizationSecretFileName, authenticationSecretName, authorizationSecretName string
	var authenticationSecretManifest, authorizationSecretManifest, authenticationConfig, authorizationConfig []byte
	if providerSpec.Authentication != nil {
		authenticationSecretFileName = providerSpec.Authentication.SecretFile
		authenticationSecretManifest, authenticationSecretName, authenticationConfig, err = processSecret(
			b, privateKey, configDir, authenticationSecretFileName, providerSpec.Authentication.URL)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if providerSpec.Authorization != nil {
		authorizationSecretFileName = providerSpec.Authorization.SecretFile
		authorizationSecretManifest, authorizationSecretName, authorizationConfig, err = processSecret(
			b, privateKey, configDir, authorizationSecretFileName, providerSpec.Authorization.URL)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	filePlan, err := b.Plan()
	if err != nil {
		log.Infof("Plan creation failed:\n%s\n", err)
		return nil, nil, nil, err
	}
	builder.AddResource("install:pem-files", &filePlan, plan.DependOn("install:config"))
	authConfigMap, authConfigMapManifest, err := createAuthConfigMapManifest(authenticationSecretName, authorizationSecretName,
		authenticationConfig, authorizationConfig)
	if err != nil {
		return nil, nil, nil, err
	}
	secretResources := createPemSecretResources(authenticationSecretManifest, authorizationSecretManifest, authenticationSecretName, authorizationSecretName, ns)
	return secretResources, authConfigMap, authConfigMapManifest, nil
}

func getPrivateKey(privateKeyPath string) (*rsa.PrivateKey, error) {
	privateKeyBytes, err := getConfigFileContents(privateKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "Could not read private key")
	}
	privateKeyData, err := certUtil.ParsePrivateKeyPEM(privateKeyBytes)
	if err != nil {
		return nil, err
	}
	privateKey, ok := privateKeyData.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Private key file %q did not contain valid private key", privateKeyPath)
	}
	return privateKey, nil
}

func checkPemValues(providerSpec *baremetalspecv1.BareMetalClusterProviderSpec, privateKeyPath, certPath string) error {
	if privateKeyPath == "" || certPath == "" {
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

func createPemSecretResources(authenticationManifest, authorizationManifest []byte, authenticationSecretName, authorizationSecretName, namespace string) map[string]*secretResourceSpec {
	result := map[string]*secretResourceSpec{}
	if authenticationSecretName != "" {
		result["authentication"] = &secretResourceSpec{
			secretName: authenticationSecretName,
			resource:   &resource.KubectlApply{Namespace: object.String(namespace), Manifest: authenticationManifest, Filename: object.String(authenticationSecretName)}}
	}
	if authorizationSecretName != "" {
		result["authorization"] = &secretResourceSpec{
			secretName: authorizationSecretName,
			resource:   &resource.KubectlApply{Namespace: object.String(namespace), Manifest: authorizationManifest, Filename: object.String(authorizationSecretName)}}
	}
	return result
}

func createAuthConfigMapManifest(authnSecretName, authzSecretName string, authnConfig, authzConfig []byte) (*v1.ConfigMap, []byte, error) {
	data := map[string]string{}
	storeIfNotEmpty(data, "authentication-secret-name", authnSecretName)
	storeIfNotEmpty(data, "authorization-secret-name", authzSecretName)
	storeIfNotEmpty(data, "authentication-config", string(authnConfig))
	storeIfNotEmpty(data, "authorization-config", string(authzConfig))
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

func processSecret(b *plan.Builder, key *rsa.PrivateKey, configDir, secretFileName, URL string) ([]byte, string, []byte, error) {
	contents, err := getConfigFileContents(configDir, secretFileName)
	if err != nil {
		return nil, "", nil, err
	}
	object, err := runtime.Decode(scheme.Codecs.UniversalDecoder(ssv1alpha1.SchemeGroupVersion), contents)
	if err != nil {
		return nil, "", nil, err
	}
	switch s := object.(type) {
	case *ssv1alpha1.SealedSecret:
		secret, err := s.Unseal(scheme.Codecs, key)
		if err != nil {
			return nil, "", nil, errors.Wrap(err, "Could not unseal auth secret")
		}
		secretName := secret.Name
		for _, key := range pemKeys {
			fileContents, ok := secret.Data[key]
			if !ok {
				return nil, "", nil, fmt.Errorf("Missing auth config value for: %q in secret %q", key, secretName)
			}
			resName := secretName + "-" + key
			fileName := filepath.Join(PemDestDir, secretName, key+".pem")
			b.AddResource("install:"+resName, &resource.File{Content: string(fileContents), Destination: fileName}, plan.DependOn("set-perms:pem-dir"))
		}
		contextName := secretName + "-webhook"
		userName := secretName + "-api-server"
		config := &clientcmdapi.Config{
			Kind:       "Config",
			APIVersion: "v1",
			Clusters: map[string]*clientcmdapi.Cluster{
				secretName: {
					CertificateAuthority: filepath.Join(PemDestDir, secretName, "certificate-authority.pem"),
					Server:               URL,
				},
			},
			AuthInfos: map[string]*clientcmdapi.AuthInfo{
				userName: {
					ClientCertificate: filepath.Join(PemDestDir, secretName, "client-certificate.pem"),
					ClientKey:         filepath.Join(PemDestDir, secretName, "client-key.pem"),
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
			return nil, "", nil, err
		}
		configResource := &resource.File{Content: string(authConfig), Destination: filepath.Join(ConfigDestDir, secretName+".yaml")}
		b.AddResource("install:"+secretName, configResource, plan.DependOn("set-perms:pem-dir"))

		return contents, secretName, authConfig, nil
	default:
		return nil, "", nil, fmt.Errorf("File %q does not contain a sealed secret", secretFileName)
	}
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

func (o OS) configureFlux(b *plan.Builder, params SeedNodeParams) error {
	gitData := params.GitData
	if gitData.GitURL == "" {
		return nil
	}
	fluxManifestPath, err := findFluxManifest(params.ConfigDirectory)
	if err != nil {
		// We haven't found a flux.yaml manifest in the git repository, use the flux addon.
		gitParams := map[string]string{"gitURL": gitData.GitURL, "gitBranch": gitData.GitBranch, "gitPath": gitData.GitPath}
		err := processDeployKey(gitParams, gitData.GitDeployKeyPath)
		if err != nil {
			return errors.Wrap(err, "failed to process the git deploy key")
		}
		fluxAddon := baremetalspecv1.Addon{Name: "flux", Params: gitParams}
		manifests, err := buildAddon(fluxAddon, params.ImageRepository, params.ClusterManifestPath, params.Namespace)
		if err != nil {
			return errors.Wrap(err, "failed to generate manifests for flux")
		}
		for i, m := range manifests {
			resName := fmt.Sprintf("%s-%02d", "flux", i)
			fluxRsc := &resource.KubectlApply{Manifest: m, Filename: object.String(resName + ".yaml")}
			b.AddResource("install:flux:"+resName, fluxRsc, plan.DependOn("kubectl:apply:cluster", "kubectl:apply:machines"))
		}
		return nil
	}

	// Use flux.yaml from the git repository.
	manifest, err := createFluxSecretFromGitData(gitData, params)
	if err != nil {
		return errors.Wrap(err, "failed to generate git deploy secret manifest for flux")
	}
	secretResName := "flux-git-deploy-secret"
	fluxSecretRsc := &resource.KubectlApply{OpaqueManifest: manifest, Filename: object.String(secretResName + ".yaml")}
	b.AddResource("install:flux:"+secretResName, fluxSecretRsc, plan.DependOn("kubectl:apply:cluster", "kubectl:apply:machines"))

	fluxRsc := &resource.KubectlApply{ManifestPath: object.String(fluxManifestPath)}
	b.AddResource("install:flux:main", fluxRsc, plan.DependOn("install:flux:flux-git-deploy-secret"))
	return nil
}

func findManifest(dir, name string) (string, error) {
	result := ""
	err := fmt.Errorf("No %q manifest found in directory: %q", name, dir)
	filepath.Walk(dir,
		func(path string, info os.FileInfo, e error) error {
			if e != nil {
				return nil // Other files may still be okay
			}
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			if info.Name() == name {
				result = path
				err = nil
				return filepath.SkipDir
			}
			return nil
		})
	return result, err
}

func findFluxManifest(dir string) (string, error) {
	return findManifest(dir, "flux.yaml")
}

func findControllerManifest(dir string) (string, error) {
	return findManifest(dir, "wks-controller.yaml")
}

func replaceGitFields(templateBody string, gitParams map[string]string) ([]byte, error) {
	t, err := template.New("flux-secret").Parse(templateBody)
	if err != nil {
		return nil, err
	}
	var populated bytes.Buffer
	err = t.Execute(&populated, struct {
		Namespace   string
		SecretValue string
	}{gitParams["namespace"], gitParams["gitDeployKey"]})
	if err != nil {
		return nil, err
	}
	return populated.Bytes(), nil
}

func createFluxSecretFromGitData(gitData GitParams, params SeedNodeParams) ([]byte, error) {
	gitParams := map[string]string{"namespace": params.Namespace}
	err := processDeployKey(gitParams, gitData.GitDeployKeyPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process the git deploy key")
	}
	return replaceGitFields(fluxSecretTemplate, gitParams)
}

func wksControllerManifest(controller ControllerParams, namespace, configDir string) ([]byte, error) {
	var manifestbytes []byte

	// The controller manifest is taken, in order:
	// 1. from the specified git repository checkout.
	// 2. from the YAML manifest built-in the binary.
	//
	// The controller image is, in priority order:
	// 1. controllerImageOverride provided on the apply command line.
	// 2. the image from the manifest if we have found a manifest in the git repository checkout.
	// 3. quay.io/wksctl/controller:version.ImageTag
	filepath, err := findControllerManifest(configDir)
	if err != nil {
		file, openErr := manifests.Manifests.Open("04_controller.yaml")
		if openErr != nil {
			return nil, openErr
		}
		manifestbytes, err = ioutil.ReadAll(file)
		if controller.ImageOverride == "" {
			controller.ImageOverride = controller.ImageBuiltin
		}
	} else {
		manifestbytes, err = ioutil.ReadFile(filepath)
	}
	if err != nil {
		return nil, err
	}
	content, err := manifest.WithNamespace(string(manifestbytes), namespace)
	if err != nil {
		return nil, err
	}
	return updateControllerImage([]byte(content), controller.ImageOverride)
}

const deployment = "Deployment"

// updateControllerImage replaces the controller image in the manifest and
// returns the updated manifest
func updateControllerImage(manifest []byte, controllerImageOverride string) ([]byte, error) {
	if controllerImageOverride == "" {
		return manifest, nil
	}
	d := &v1beta2.Deployment{}
	if err := yaml.Unmarshal(manifest, d); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal WKS controller's manifest")
	}
	if d.Kind != deployment {
		return nil, fmt.Errorf("invalid kind for WKS controller's manifest: expected %q but got %q", deployment, d.Kind)
	}
	var updatedController bool
	for i := 0; i < len(d.Spec.Template.Spec.Containers); i++ {
		if d.Spec.Template.Spec.Containers[i].Name == "controller" {
			d.Spec.Template.Spec.Containers[i].Image = controllerImageOverride
			updatedController = true
		}
	}
	if !updatedController {
		return nil, errors.New("failed to update WKS controller's manifest: container not found")
	}
	return yaml.Marshal(d)
}

// NodeParams groups required inputs to configure a Kubernetes node.
type NodeParams struct {
	IsMaster                 bool // true if this node is a master, false else.
	MasterIP                 string
	MasterPort               int
	Token                    string // kubeadm's --token
	DiscoveryTokenCaCertHash string // kubeadm's --discovery-token-ca-cert-hash
	CertificateKey           string // kubeadm's --certificate-key
	KubeletConfig            config.KubeletConfig
	KubernetesVersion        string
	CRI                      baremetalspecv1.ContainerRuntime
	ConfigFileSpecs          []baremetalspecv1.FileSpec
	ProviderConfigMaps       map[string]*v1.ConfigMap
	AuthConfigMap            *v1.ConfigMap
	Namespace                string
	ExternalLoadBalancer     string // used instead of MasterIP if existed
}

// Validate generally validates this NodeParams struct, e.g. ensures it
// contains mandatory values, that these are well-formed, etc.
func (params NodeParams) Validate() error {
	if len(params.KubeletConfig.NodeIP) == 0 {
		return errors.New("empty kubelet node IP")
	}
	return nil
}

// SetupNode installs Kubernetes on this machine and configures it based on the
// manifests stored during the initialization of the cluster, when
// SetupSeedNode was called.
func (o OS) SetupNode(p *plan.Plan) error {
	// We don't know the state of the machine so undo at the beginning
	p.Undo(o.runner, plan.EmptyState)

	_, err := p.Apply(o.runner, plan.EmptyDiff())
	if err != nil {
		log.Errorf("Apply of Plan failed:\n%s\n", err)
	}
	return err
}

// CreateNodeSetupPlan creates the plan that will be used to set up a node.
func (o OS) CreateNodeSetupPlan(params NodeParams) (*plan.Plan, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	cfg, err := envcfg.GetEnvSpecificConfig(o.PkgType, params.Namespace, params.KubeletConfig.CloudProvider, o.runner)
	if err != nil {
		return nil, err
	}

	configFileResources, err := createConfigFileResourcesFromConfigMaps(params.ConfigFileSpecs, params.ProviderConfigMaps)
	if err != nil {
		return nil, err
	}

	b := plan.NewBuilder()

	baseRsrc := recipe.BuildBasePlan(o.PkgType)
	b.AddResource("install:base", baseRsrc)
	authConfigMap := params.AuthConfigMap
	if authConfigMap != nil && params.IsMaster {
		for _, authType := range []string{"authentication", "authorization"} {
			addAuthConfigResources(b, authConfigMap, authType, params.Namespace)
		}
	}

	configRes := recipe.BuildConfigPlan(configFileResources)
	b.AddResource("install:config", configRes, plan.DependOn("install:base"))
	instCriRsrc := recipe.BuildCRIPlan(&params.CRI, cfg, o.PkgType)
	b.AddResource("install.cri", instCriRsrc, plan.DependOn("install:config"))

	instK8sRsrc := recipe.BuildK8SPlan(params.KubernetesVersion, params.KubeletConfig.NodeIP, cfg.SetSELinuxPermissive, cfg.DisableSwap, cfg.LockYUMPkgs, o.PkgType, params.KubeletConfig.CloudProvider)

	b.AddResource("install:k8s", instK8sRsrc, plan.DependOn("install.cri"))

	kadmPJRsrc := recipe.BuildKubeadmPrejoinPlan(params.KubernetesVersion, cfg.UseIPTables)
	b.AddResource("kubeadm:prejoin", kadmPJRsrc, plan.DependOn("install:k8s"))

	kadmJoinRsrc := &resource.KubeadmJoin{
		IsMaster:                 params.IsMaster,
		NodeIP:                   params.KubeletConfig.NodeIP,
		NodeName:                 cfg.HostnameOverride,
		MasterIP:                 params.MasterIP,
		MasterPort:               params.MasterPort,
		Token:                    params.Token,
		DiscoveryTokenCaCertHash: params.DiscoveryTokenCaCertHash,
		CertificateKey:           params.CertificateKey,
		IgnorePreflightErrors:    cfg.IgnorePreflightErrors,
		ExternalLoadBalancer:     params.ExternalLoadBalancer,
	}
	b.AddResource("kubeadm:join", kadmJoinRsrc, plan.DependOn("kubeadm:prejoin"))
	return createPlan(b)
}

func addAuthConfigResources(b *plan.Builder, authConfigMap *v1.ConfigMap, authType, ns string) error {
	secretName := authConfigMap.Data[authType+"-secret-name"]
	if secretName != "" {
		var authPemRsrc plan.Resource
		for {
			res, err := resource.NewKubeSecretResource(secretName, filepath.Join(PemDestDir, secretName), ns,
				func(s string) string {
					return s + ".pem"
				})
			if err != nil {
				return err
			}
			if res != nil {
				authPemRsrc = res
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		b.AddResource("install:"+authType+"-pem-files", authPemRsrc, plan.DependOn("install:base"))
		b.AddResource("install:"+authType+"-config", &resource.File{Content: authConfigMap.Data[authType+"-config"], Destination: filepath.Join(ConfigDestDir, secretName+".yaml")})
	}
	return nil
}

const (
	centOS = "centos"
	ubuntu = "ubuntu"
)

// Identify uses the provided SSH client to identify the operating system of
// the machine it is configured to talk to.
func Identify(sshClient *ssh.Client) (*OS, error) {
	osID, err := fetchOSID(sshClient)
	if err != nil {
		return nil, err
	}
	switch osID {
	case centOS:
		return &OS{Name: osID, runner: &sudo.Runner{Runner: sshClient}, PkgType: resource.PkgTypeRPM}, nil
	case ubuntu:
		return &OS{Name: osID, runner: &sudo.Runner{Runner: sshClient}, PkgType: resource.PkgTypeDeb}, nil
	default:
		return nil, fmt.Errorf("unknown operating system %q", osID)
	}
}

var osIDRegexp = regexp.MustCompile("(?m)^ID=(.+)")

const (
	numExpectedMatches = 2
	idxOSID            = 1
)

func fetchOSID(sshClient *ssh.Client) (string, error) {
	stdOut, err := sshClient.RunCommand("cat /etc/*release", nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch operating system ID")
	}
	matches := osIDRegexp.FindStringSubmatch(stdOut)
	if len(matches) != numExpectedMatches {
		return "", errors.New("failed to identify operating system")
	}
	return strings.Trim(matches[idxOSID], ` "`), nil
}

// parseCluster converts the manifest file into a Cluster
func parseCluster(clusterManifestPath string) (*clusterv1.Cluster, error) {
	f, err := os.Open(clusterManifestPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	cluster := &clusterv1.Cluster{}
	err = yaml.Unmarshal(bytes, cluster)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse cluster manifest")
	}
	return cluster, nil
}

// createPlan generates a plan from a plan builder
func createPlan(b *plan.Builder) (*plan.Plan, error) {
	p, err := b.Plan()
	if err != nil {
		log.Infof("Plan creation failed:\n%s\n", err)
		return nil, err
	}
	return &p, nil
}

// parseAddons reads the cluster config and if any addons are defined, it generates
// the manifest and returns the manifest filenames
func parseAddons(ClusterManifestPath, namespace string) (map[string][][]byte, error) {
	cluster, err := parseCluster(ClusterManifestPath)
	if err != nil {
		return nil, err
	}
	codec, err := baremetalspecv1.NewCodec()
	if err != nil {
		log.Fatal("Failed to create codec: ", err)
	}

	clusterSpec, err := codec.ClusterProviderFromProviderSpec(cluster.Spec.ProviderSpec)
	if err != nil {
		log.Fatal("Failed to parse cluster manifest: ", err)
	}
	ret := make(map[string][][]byte)
	for _, addonDesc := range clusterSpec.Addons {
		log.WithField("addon", addonDesc.Name).Debug("building addon")
		retManifests, err := buildAddon(addonDesc, clusterSpec.ImageRepository, ClusterManifestPath, namespace)
		if err != nil {
			return nil, err
		}
		ret[addonDesc.Name] = retManifests

	}
	return ret, nil
}

func buildAddon(addonDefn baremetalspecv1.Addon, imageRepository string, ClusterManifestPath, namespace string) ([][]byte, error) {
	log.WithField("addon", addonDefn.Name).Debug("building addon")
	// Generate the addon manifest.
	addon, err := addons.Get(addonDefn.Name)
	if err != nil {
		return nil, err
	}

	tmpDir, err := ioutil.TempDir("", "wksctl-apply-addons")
	if err != nil {
		return nil, err
	}

	manifests, err := addon.Build(addons.BuildOptions{
		// assume unqualified addon file params are in the same directory as the cluster.yaml
		BasePath:        filepath.Dir(ClusterManifestPath),
		OutputDirectory: tmpDir,
		ImageRepository: imageRepository,
		Params:          addonDefn.Params,
	})
	if err != nil {
		return nil, err
	}
	retManifests := [][]byte{}
	// An addon can specify dependent YAML which needs to be added to the list of manifests
	retManifests, err = processDeps(addonDefn.Deps, retManifests, namespace)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to process dependent Yaml for addon: %s", addonDefn.Name)
	}
	// The build puts files in a temp dir we read them into []byte and return those
	// so we can cleanup the temp files
	for _, m := range manifests {
		content, err := manifest.WithNamespace(m, namespace)
		if err != nil {
			return nil, err
		}
		retManifests = append(retManifests, []byte(content))
	}
	return retManifests, nil
}

func processDeps(deps []string, manifests [][]byte, namespace string) ([][]byte, error) {
	var retManifests = manifests
	for _, URL := range deps {
		logger := log.WithField("dep", URL)
		resp, err := http.Get(URL)
		if err != nil {
			logger.Warnf("Failed to load addon dependency - %v", err)
			continue
		}
		defer resp.Body.Close()
		contents, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			logger.Warnf("Failed to load addon dependency - %v", err)
		}
		ctent, err := manifest.WithNamespace(string(contents), namespace)
		if err != nil {
			logger.Warnf("Failed to set namespace for manifest:\n%s\n", ctent)
		}
		logger.Debugln("Loading dependency")
		retManifests = append(retManifests, []byte(ctent))
	}
	return retManifests, nil
}
