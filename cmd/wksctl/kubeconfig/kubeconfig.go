package kubeconfig

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/kubernetes/config"
	"github.com/weaveworks/wksctl/pkg/manifests"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/utilities/path"
)

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
	if err != nil {
		return errors.Wrapf(err, "GetRemoteKubeconfig")
	}

	configPath := path.Kubeconfig(wksHome, kubeconfigOptions.namespace, sp.GetClusterName())

	_, err = path.CreateDirectory(filepath.Dir(configPath))
	if err != nil {
		return errors.Wrapf(err, "failed to create configuration directory")
	}

	err = ioutil.WriteFile(configPath, []byte(configStr), 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to write Kubernetes configuration locally")
	}
	fmt.Printf("To use kubectl with the %s cluster, enter:\n$ export KUBECONFIG=%s\n", sp.GetClusterName(), configPath)
	return nil
}
