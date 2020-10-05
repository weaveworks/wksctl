package os

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	existinginfrav1 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeios "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	capeiresource "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/manifest"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/controller/manifests"
	"k8s.io/api/apps/v1beta2"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// SetupSeedNode installs Kubernetes on this machine, and store the provided
// manifests in the API server, so that the rest of the cluster can then be
// set up by the WKS controller.
func SetupSeedNode(ctx context.Context, o *capeios.OS, params SeedNodeParams) error {
	p, err := CreateSeedNodeSetupPlan(ctx, o, params)
	if err != nil {
		return err
	}
	return applySeedNodePlan(ctx, o, p)
}

// CreateSeedNodeSetupPlan constructs the seed node plan used to setup the initial node
// prior to turning control over to wks-controller
func CreateSeedNodeSetupPlan(ctx context.Context, o *capeios.OS, params SeedNodeParams) (*plan.Plan, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	cfg, err := envcfg.GetEnvSpecificConfig(ctx, o.PkgType, params.Namespace, params.KubeletConfig.CloudProvider, o.Runner)
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
	return applySeedNodePlan(o, p)
}

func storeIfNotEmpty(vals map[string]string, key, value string) {
	if value != "" {
		vals[key] = value
	}
}

func seedNodeSetupPlan(o *capeios.OS, params capeios.SeedNodeParams, providerSpec *existinginfrav1.ClusterSpec, providerConfigMaps map[string]*v1.ConfigMap, authConfigMap *v1.ConfigMap, secretResources map[string]*secretResourceSpec, kubernetesVersion, kubernetesNamespace string) (*plan.Plan, error) {
	secrets := map[string]capeiresource.SecretData{}
	for k, v := range secretResources {
		secrets[k] = v.decrypted
	}
	nodeParams := capeios.NodeParams{
		IsMaster:             true,
		MasterIP:             params.PrivateIP,
		MasterPort:           6443, // See TODO in machine_actuator.go
		KubeletConfig:        params.KubeletConfig,
		KubernetesVersion:    kubernetesVersion,
		CRI:                  providerSpec.CRI,
		ConfigFileSpecs:      providerSpec.OS.Files,
		ProviderConfigMaps:   providerConfigMaps,
		AuthConfigMap:        authConfigMap,
		Secrets:              secrets,
		Namespace:            params.Namespace,
		AddonNamespaces:      params.AddonNamespaces,
		ControlPlaneEndpoint: providerSpec.ControlPlaneEndpoint,
	}
	return o.CreateNodeSetupPlan(ctx, nodeParams)
}

func applySeedNodePlan(ctx context.Context, o *capeios.OS, p *plan.Plan) error {
	err := p.Undo(ctx, o.Runner, plan.EmptyState)
	if err != nil {
		log.Infof("Pre-plan cleanup failed:\n%s\n", err)
		return err
	}

	_, err = p.Apply(ctx, o.Runner, plan.EmptyDiff())
	if err != nil {
		log.Errorf("Apply of Plan failed:\n%s\n", err)
		return err
	}
	return err
}

func createConfigFileResourcesFromFiles(providerSpec *existinginfrav1.ClusterSpec, configDir, namespace string) (map[string][]byte, map[string]*v1.ConfigMap, []*capeiresource.File, error) {
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
	resources, err := capeios.CreateConfigFileResourcesFromConfigMaps(fileSpecs, configMaps)
	if err != nil {
		return nil, nil, nil, err
	}
	return configMapManifests, configMaps, resources, nil
}

func getConfigMapManifests(fileSpecs []existinginfrav1.FileSpec, configDir, namespace string) (map[string][]byte, error) {
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
	content, err := manifest.WithNamespace(serializer.FromBytes(bytes), namespace)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// getConfigFileContents reads a config manifest from a file in the config directory.
func getConfigFileContents(fileNameComponent ...string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(fileNameComponent...))
}

type secretResourceSpec struct {
	secretName string
	decrypted  capeiresource.SecretData
	resource   plan.Resource
}

func findManifest(dir, name string) (result string, err error) {
	err = filepath.Walk(dir,
		func(path string, info os.FileInfo, e error) error {
			if e != nil {
				return nil // Other files may still be okay
			}
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			if info.Name() == name {
				result = path
				return filepath.SkipDir
			}
			return nil
		})
	if err != nil {
		result = ""
		return
	}
	if result == "" {
		err = fmt.Errorf("No %q manifest found in directory: %q", name, dir)
	}
	return
}

func findFluxManifest(dir string) (string, error) {
	return findManifest(dir, "flux.yaml")
}

func findControllerManifest(dir string) (string, error) {
	return findManifest(dir, "wks-controller.yaml")
}

func capiControllerManifest(controller capeios.ControllerParams, namespace, configDir string) ([]byte, error) {
	var file io.ReadCloser
	filepath, err := findManifest(configDir, "capi-controller.yaml")
	if err != nil {
		file, err = manifests.Manifests.Open("04_capi_controller.yaml")
	} else {
		file, err = os.Open(filepath)
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	manifestbytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	content, err := manifest.WithNamespace(serializer.FromBytes(manifestbytes), namespace)
	return content, err
}

func wksControllerManifest(controller capeios.ControllerParams, namespace, configDir string) ([]byte, error) {
	var manifestbytes []byte

	// The controller manifest is taken, in order:
	// 1. from the specified git repository checkout.
	// 2. from the YAML manifest built-in the binary.
	//
	// The controller image is, in priority order:
	// 1. controllerImageOverride provided on the apply command line.
	// 2. the image from the manifest if we have found a manifest in the git repository checkout.
	// 3. docker.io/weaveworks/wksctl-controller:version.ImageTag
	filepath, err := findControllerManifest(configDir)
	if err != nil {
		file, openErr := manifests.Manifests.Open("04_controller.yaml")
		if openErr != nil {
			return nil, openErr
		}
		manifestbytes, err = ioutil.ReadAll(file)
	} else {
		manifestbytes, err = ioutil.ReadFile(filepath)
	}
	if err != nil {
		return nil, err
	}
	content, err := manifest.WithNamespace(serializer.FromBytes(manifestbytes), namespace)
	if err != nil {
		return nil, err
	}
	return updateControllerImage(content, controller.ImageOverride)
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
