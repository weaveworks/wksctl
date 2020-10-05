package os

import (
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
func SetupSeedNode(o *capeios.OS, params capeios.SeedNodeParams, updater func(*plan.Plan) (*plan.Plan, error)) error {
	p, err := capeios.CreateSeedNodeSetupPlan(o, params)
	if err != nil {
		return err
	}
	if updater != nil {
		p, err = updater(p)
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
	return o.CreateNodeSetupPlan(nodeParams)
}

func applySeedNodePlan(o *capeios.OS, p *plan.Plan) error {
	err := p.Undo(o.Runner, plan.EmptyState)
	if err != nil {
		log.Infof("Pre-plan cleanup failed:\n%s\n", err)
		return err
	}

	_, err = p.Apply(o.Runner, plan.EmptyDiff())
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
