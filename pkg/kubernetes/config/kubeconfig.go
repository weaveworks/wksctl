package config

import (
	"fmt"
	"net/url"
	"regexp"

	yaml "github.com/ghodss/yaml"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	baremetalv1 "github.com/weaveworks/wksctl/pkg/baremetal/v1alpha3"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/plan/runners/sudo"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/path"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// DefaultPath defines the default path
var DefaultPath = clientcmd.RecommendedHomeFile
var DefaultClusterName = "kubernetes"
var DefaultClusterAdminName = "kubernetes-admin"
var DefaultContextName = fmt.Sprintf("%s@%s", DefaultClusterAdminName, DefaultClusterName)

// NewKubeConfig generates a Kubernetes configuration (e.g. for kubectl to use)
// from the provided machines, and places it in the provided directory.
func NewKubeConfig(artifactDirectory string, machines []*clusterv1.Machine, bl []*baremetalv1.BareMetalMachine) (string, error) {
	master, bmm := machine.FirstMaster(machines, bl)
	if master == nil {
		return "", errors.New("at least one master node is required to create a Kubernetes configuration file")
	}
	return path.WKSResourcePath(artifactDirectory, bmm.Spec.Address), nil
}

// Params groups the various settings to transform Kubernetes configurations.
type Params struct {
	APIServerExternalEndpoint string
	SkipTLSVerify             bool
}

// In some cases, the raw Kubernetes configuration, read over SSH, contains a
// SSH banner starting with the following. We should remove these prior to
// processing the Kubernetes configuration as it breaks deserialisation.
var sshBannerRegex = regexp.MustCompile(`(?m)^System is booting up.*?$\n`)

// Sanitize sanitizes, and possibly transforms the provided Kubernetes configuration.
func Sanitize(configStr string, params Params) (string, error) {
	if params.APIServerExternalEndpoint == "" {
		return "", fmt.Errorf("need to provide the API server endpoint")
	}

	configStr = sshBannerRegex.ReplaceAllString(configStr, "")
	if params.APIServerExternalEndpoint != "" || params.SkipTLSVerify {
		var config clientcmdv1.Config
		if err := yaml.Unmarshal([]byte(configStr), &config); err != nil {
			return "", errors.Wrap(err, "Failed to parse the config file: ")
		}
		for i := 0; i < len(config.Clusters); i++ {
			if params.SkipTLSVerify {
				config.Clusters[i].Cluster.CertificateAuthorityData = nil
				config.Clusters[i].Cluster.InsecureSkipTLSVerify = true
			}
			if params.APIServerExternalEndpoint != "" {
				u, err := url.Parse(config.Clusters[i].Cluster.Server)
				if err != nil {
					return "", errors.Wrap(err, "Failed to parse the server url: ")
				}
				u.Host = params.APIServerExternalEndpoint + ":" + u.Port()
				config.Clusters[i].Cluster.Server = u.String()
			}
		}
		y, err := yaml.Marshal(config)
		if err != nil {
			return "", errors.Wrap(err, "Failed to parse the updated config file: ")
		}
		configStr = string(y)
	}
	return configStr, nil
}

// GetRemoteKubeconfig retrieves Kubernetes configuration from a master node of the cluster
func GetRemoteKubeconfig(sp *specs.Specs, sshKeyPath string, verbose, skipTLSVerify bool) (string, error) {
	sshClient, err := ssh.NewClientForMachine(sp.MasterSpec, sp.ClusterSpec.User, sshKeyPath, verbose)
	if err != nil {
		return "", errors.Wrap(err, "failed to create SSH client: ")
	}
	defer sshClient.Close()

	runner := sudo.Runner{Runner: sshClient}
	configStr, err := runner.RunCommand("cat /etc/kubernetes/admin.conf", nil)
	if err != nil {
		return "", errors.Wrap(err, "failed to retrieve Kubernetes configuration")
	}

	endpoint := sp.GetMasterPublicAddress()
	if sp.ClusterSpec.ControlPlaneEndpoint != "" {
		endpoint = sp.ClusterSpec.ControlPlaneEndpoint
	}

	return Sanitize(configStr, Params{
		APIServerExternalEndpoint: endpoint,
		SkipTLSVerify:             skipTLSVerify,
	})
}

// Write will write Kubernetes client configuration to a file.
// If path isn't specified then the path will be determined by client-go.
// If file pointed to by path doesn't exist it will be created.
// If the file already exists then the configuration will be merged with the existing file.
func Write(path string, newConfig clientcmdapi.Config, setContext bool) (string, error) {
	configAccess := GetConfigAccess(path)

	existingConfig, err := configAccess.GetStartingConfig()
	if err != nil {
		return "", errors.Wrapf(err, "Unable to read existing kubeconfig file %q", path)
	}

	log.Debug("Merging kubeconfig files")
	mergedConfig := Merge(existingConfig, &newConfig)

	if setContext && newConfig.CurrentContext != "" {
		log.Debugf("setting current-context to %s", newConfig.CurrentContext)
		mergedConfig.CurrentContext = newConfig.CurrentContext
	}

	if err := clientcmd.ModifyConfig(configAccess, *mergedConfig, true); err != nil {
		return "", errors.Wrapf(err, "unable to modify kubeconfig %s", path)
	}

	return configAccess.GetDefaultFilename(), nil
}

func GetConfigAccess(explicitPath string) clientcmd.ConfigAccess {
	pathOptions := clientcmd.NewDefaultPathOptions()
	if explicitPath != "" && explicitPath != DefaultPath {
		pathOptions.LoadingRules.ExplicitPath = explicitPath
	}

	return interface{}(pathOptions).(clientcmd.ConfigAccess)
}

// Merge two kubeconfig objects
func Merge(existing *clientcmdapi.Config, tomerge *clientcmdapi.Config) *clientcmdapi.Config {
	for k, v := range tomerge.Clusters {
		existing.Clusters[k] = v
	}
	for k, v := range tomerge.AuthInfos {
		existing.AuthInfos[k] = v
	}
	for k, v := range tomerge.Contexts {
		existing.Contexts[k] = v
	}

	return existing
}

// RenameConfig renames the default cluster and context names to the values from cluster.yaml
func RenameConfig(sp *specs.Specs, newConfig *clientcmdapi.Config) {
	log.Debug("Renaming cluster")
	newConfig.Clusters[sp.GetClusterName()] = newConfig.Clusters[DefaultClusterName]
	delete(newConfig.Clusters, DefaultClusterName)

	log.Debug("Renaming context")
	newContextName := fmt.Sprintf("%s@%s", DefaultClusterAdminName, sp.GetClusterName())
	newConfig.Contexts[newContextName] = newConfig.Contexts[DefaultContextName]
	newConfig.Contexts[newContextName].Cluster = sp.GetClusterName()
	delete(newConfig.Contexts, DefaultContextName)

	log.Debug("Renaming current context")
	newConfig.CurrentContext = newContextName
}
