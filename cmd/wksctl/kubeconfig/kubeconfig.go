package kubeconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	capeipath "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/path"
	"github.com/weaveworks/wksctl/pkg/kubernetes/config"
	"github.com/weaveworks/wksctl/pkg/manifests"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/utilities/path"
	"k8s.io/client-go/tools/clientcmd"
)

// A new version of the kubeconfig command that retrieves the config from
// /etc/kubernetes/admin.conf on a cluster master node

// Cmd represents the kubeconfig command
var Cmd = &cobra.Command{
	Use:          "kubeconfig",
	Short:        "Generate a kubeconfig file for the cluster",
	RunE:         kubeconfigRun,
	SilenceUsage: true,
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
	useContext           bool
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
	Cmd.Flags().StringVar(&kubeconfigOptions.sshKeyPath, "ssh-key", "./setup/cluster-key", "Path to a key authorized to log in to machines by SSH")
	Cmd.Flags().StringVar(
		&kubeconfigOptions.artifactDirectory, "artifact-directory", "", "Write output files in the specified directory")
	Cmd.Flags().StringVar(
		&kubeconfigOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace portion of kubeconfig path")
	Cmd.Flags().BoolVar(
		&kubeconfigOptions.useContext, "use-context", true,
		"Set current context to the newly created one")
	Cmd.Flags().BoolVar(
		&kubeconfigOptions.skipTLSVerify, "insecure-skip-tls-verify", false,
		"Enables kubectl to communicate with the API w/o verifying the certificate")
	_ = Cmd.Flags().MarkHidden("insecure-skip-tls-verify")

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
		// Check for cluster.yaml and machines.yaml paths
		if clusterPath == "cluster.yaml" && machinesPath == "machines.yaml" {
			if _, err := os.Stat(kubeconfigOptions.clusterManifestPath); os.IsNotExist(err) {
				clusterPath = "./setup/cluster.yaml"
			}
			if _, err := os.Stat(kubeconfigOptions.machinesManifestPath); os.IsNotExist(err) {
				machinesPath = "./setup/machines.yaml"
			}
		}
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

	return writeKubeconfig(cmd.Context(), clusterPath, machinesPath)
}

func writeKubeconfig(ctx context.Context, cpath, mpath string) error {
	var wksHome string
	var err error
	var configPath string

	sp := specs.NewFromPaths(cpath, mpath)

	if kubeconfigOptions.artifactDirectory != "" {
		wksHome, err = path.CreateDirectory(capeipath.ExpandHome(kubeconfigOptions.artifactDirectory))
		if err != nil {
			return errors.Wrapf(err, "failed to create WKS home directory")
		}

		_, err = path.CreateDirectory(filepath.Dir(configPath))
		if err != nil {
			return errors.Wrapf(err, "failed to create configuration directory")
		}
		configPath = path.Kubeconfig(wksHome, kubeconfigOptions.namespace, sp.GetClusterName())
	} else {
		configPath = clientcmd.RecommendedHomeFile
	}

	configStr, err := config.GetRemoteKubeconfig(ctx, sp, kubeconfigOptions.sshKeyPath, kubeconfigOptions.verbose, kubeconfigOptions.skipTLSVerify)
	if err != nil {
		return errors.Wrapf(err, "failed to get remote kubeconfig")
	}

	remoteConfig, err := clientcmd.Load([]byte(configStr))
	if err != nil {
		return errors.Wrapf(err, "failed to load kubeconfig")
	}
	config.RenameConfig(sp, remoteConfig)

	configPath, err = config.Write(configPath, *remoteConfig, kubeconfigOptions.useContext)
	if err != nil {
		return errors.Wrapf(err, "failed to write Kubernetes configuration locally")
	}
	if kubeconfigOptions.artifactDirectory != "" {
		fmt.Printf("To use kubectl with the %s cluster, enter:\n$ export KUBECONFIG=%s\n", sp.GetClusterName(), configPath)
	} else {
		fmt.Printf("The kubeconfig file at %q has been updated\n", configPath)
	}

	return nil
}
