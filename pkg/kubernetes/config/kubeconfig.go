package config

import (
	"fmt"
	"net/url"
	"regexp"

	yaml "github.com/ghodss/yaml"
	"github.com/pkg/errors"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/plan/runners/sudo"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/path"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// NewKubeConfig generates a Kubernetes configuration (e.g. for kubectl to use)
// from the provided machines, and places it in the provided directory.
func NewKubeConfig(artifactDirectory string, machines []*clusterv1.Machine) (string, error) {
	master := machine.FirstMaster(machines)
	if master == nil {
		return "", errors.New("at least one master node is required to create a Kubernetes configuration file")
	}
	config, err := machine.Config(master)
	if err != nil {
		return "", err
	}
	return path.WKSResourcePath(artifactDirectory, config.Address), nil
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
	if sp.ClusterSpec.APIServer.ExternalLoadBalancer != "" {
		endpoint = sp.ClusterSpec.APIServer.ExternalLoadBalancer
	}

	return Sanitize(configStr, Params{
		APIServerExternalEndpoint: endpoint,
		SkipTLSVerify:             skipTLSVerify,
	})
}
