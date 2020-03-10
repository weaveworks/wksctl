package kubeconfig

import (
	"fmt"
	"path/filepath"

	"github.com/kris-nova/logger"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/kubernetes/config"
	"github.com/weaveworks/wksctl/pkg/manifests"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/utilities/path"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// DefaultPath defines the default path
var DefaultPath = clientcmd.RecommendedHomeFile

// A new version of the kubeconfig command that retrieves the config from
// /etc/kubernetes/admin.conf on a cluster master node

// Cmd represents the kubeconfig command
var Cmd = &cobra.Command{
	Use:   "kubeconfig",
	Short: "Generate a kubeconfig file for the cluster",
	RunE:  kubeconfigRun,
}

var kubeconfigOptions struct {
	clusterManifestPath  string
	machinesManifestPath string
	gitURL               string
	gitBranch            string
	gitPath              string
	gitDeployKeyPath     string
	artifactDirectory    string
	namespace            string
	sshKeyPath           string
	skipTLSVerify        bool
	useLocalhost         bool
	usePublicAddress     bool
	verbose              bool
}

func init() {
	Cmd.Flags().StringVar(
		&kubeconfigOptions.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	Cmd.Flags().StringVar(
		&kubeconfigOptions.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	Cmd.Flags().StringVar(&kubeconfigOptions.gitURL, "git-url", "",
		"Git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&kubeconfigOptions.gitBranch, "git-branch", "master",
		"Branch within git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&kubeconfigOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	Cmd.Flags().StringVar(&kubeconfigOptions.gitDeployKeyPath, "git-deploy-key", "", "Path to the Git deploy key")
	Cmd.Flags().StringVar(&kubeconfigOptions.sshKeyPath, "ssh-key", "./cluster-key", "Path to a key authorized to log in to machines by SSH")
	Cmd.Flags().StringVar(
		&kubeconfigOptions.artifactDirectory, "artifact-directory", "", "Write output files in the specified directory")
	Cmd.Flags().StringVar(
		&kubeconfigOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace portion of kubeconfig path")
	Cmd.Flags().BoolVar(
		&kubeconfigOptions.skipTLSVerify, "insecure-skip-tls-verify", false,
		"Enables kubectl to communicate with the API w/o verifying the certificate")
	Cmd.Flags().MarkHidden("insecure-skip-tls-verify")

	// Intentionally shadows the globally defined --verbose flag.
	Cmd.Flags().BoolVarP(&kubeconfigOptions.verbose, "verbose", "v", false, "Enable verbose output")
}

func kubeconfigRun(cmd *cobra.Command, args []string) error {
	var clusterPath, machinesPath string

	// TODO: deduplicate clusterPath/machinesPath evaluation between here and cmd/wksctl/apply
	// https://github.com/weaveworks/wksctl/issues/58
	if kubeconfigOptions.gitURL == "" {
		// Cluster and Machine manifests come from the local filesystem.
		clusterPath, machinesPath = kubeconfigOptions.clusterManifestPath, kubeconfigOptions.machinesManifestPath
	} else {
		// Cluster and Machine manifests come from a Git repo that we'll clone for the duration of this command.
		repo, err := manifests.CloneClusterAPIRepo(kubeconfigOptions.gitURL, kubeconfigOptions.gitBranch, kubeconfigOptions.gitDeployKeyPath, kubeconfigOptions.gitPath)
		if err != nil {
			return errors.Wrap(err, "CloneClusterAPIRepo")
		}
		defer repo.Close()

		if clusterPath, err = repo.ClusterManifestPath(); err != nil {
			return errors.Wrap(err, "ClusterManifestPath")
		}
		if machinesPath, err = repo.MachinesManifestPath(); err != nil {
			return errors.Wrap(err, "MachinesManifestPath")
		}
	}

	return writeKubeconfig(clusterPath, machinesPath)
}

func writeKubeconfig(cpath, mpath string) error {
	wksHome, err := path.CreateDirectory(
		path.WKSHome(kubeconfigOptions.artifactDirectory))
	if err != nil {
		return errors.Wrapf(err, "failed to create WKS home directory")
	}
	sp := specs.NewFromPaths(cpath, mpath)

	configStr, err := config.GetRemoteKubeconfig(sp, kubeconfigOptions.sshKeyPath, kubeconfigOptions.verbose, kubeconfigOptions.skipTLSVerify)
	remoteConfig, err := clientcmd.Load([]byte(configStr))
	if err != nil {
		return errors.Wrapf(err, "GetRemoteKubeconfig")
	}

	configPath := path.Kubeconfig(wksHome, kubeconfigOptions.namespace, sp.GetClusterName())

	_, err = path.CreateDirectory(filepath.Dir(configPath))
	if err != nil {
		return errors.Wrapf(err, "failed to create configuration directory")
	}

	configPath, err = Write(configPath, *remoteConfig, true)
	if err != nil {
		return errors.Wrapf(err, "failed to write Kubernetes configuration locally")
	}
	fmt.Printf("To use kubectl with the %s cluster, enter:\n$ export KUBECONFIG=%s\n", sp.GetClusterName(), configPath)
	return nil
}

// Write will write Kubernetes client configuration to a file.
// If path isn't specified then the path will be determined by client-go.
// If file pointed to by path doesn't exist it will be created.
// If the file already exists then the configuration will be merged with the existing file.
func Write(path string, newConfig clientcmdapi.Config, setContext bool) (string, error) {
	configAccess := getConfigAccess(path)

	config, err := configAccess.GetStartingConfig()
	if err != nil {
		return "", errors.Wrapf(err, "unable to read existing kubeconfig file %q", path)
	}

	logger.Debug("merging kubeconfig files")
	merged := merge(config, &newConfig)

	if setContext && newConfig.CurrentContext != "" {
		logger.Debug("setting current-context to %s", newConfig.CurrentContext)
		merged.CurrentContext = newConfig.CurrentContext
	}

	if err := clientcmd.ModifyConfig(configAccess, *merged, true); err != nil {
		return "", errors.Wrapf(err, "unable to modify kubeconfig %s", path)
	}

	return configAccess.GetDefaultFilename(), nil
}

func getConfigAccess(explicitPath string) clientcmd.ConfigAccess {
	pathOptions := clientcmd.NewDefaultPathOptions()
	if explicitPath != "" && explicitPath != DefaultPath {
		pathOptions.LoadingRules.ExplicitPath = explicitPath
	}

	return interface{}(pathOptions).(clientcmd.ConfigAccess)
}

func merge(existing *clientcmdapi.Config, tomerge *clientcmdapi.Config) *clientcmdapi.Config {
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
