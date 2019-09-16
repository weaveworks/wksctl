package kubeconfig

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/kubernetes/config"
	"github.com/weaveworks/wksctl/pkg/manifests"
	"github.com/weaveworks/wksctl/pkg/plan/runners/sudo"
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
	Cmd.Flags().StringVar(
		&kubeconfigOptions.artifactDirectory, "artifact-directory", "", "Write output files in the specified directory")
	Cmd.Flags().StringVar(
		&kubeconfigOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace portion of kubeconfig path")
	Cmd.Flags().BoolVar(
		&kubeconfigOptions.skipTLSVerify, "insecure-skip-tls-verify", false,
		"Enables kubectl to communicate with the API w/o verifying the certificate")
	Cmd.Flags().MarkHidden("insecure-skip-tls-verify")

	// Intentionally shadows the globally defined --verbose flag.
	Cmd.Flags().BoolVar(&kubeconfigOptions.verbose, "verbose", false, "Enable verbose output")
}

// TODO this should be refactored into a common place - i.e. pkg/cluster
func generateConfig(sp *specs.Specs, configPath string) (string, error) {
	sshClient, err := sp.GetSSHClient(kubeconfigOptions.verbose)
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

	configStr, err = config.Sanitize(configStr, config.Params{
		APIServerExternalEndpoint: endpoint,
		SkipTLSVerify:             kubeconfigOptions.skipTLSVerify,
	})
	if err != nil {
		log.Fatal(err)
	}
	return configStr, nil
}

func kubeconfigRun(cmd *cobra.Command, args []string) error {
	clusterManifestPath, machinesManifestPath, closer, err := manifests.Get(kubeconfigOptions.clusterManifestPath,
		kubeconfigOptions.machinesManifestPath, kubeconfigOptions.gitURL, kubeconfigOptions.gitBranch, kubeconfigOptions.gitDeployKeyPath,
		kubeconfigOptions.gitPath)
	if closer != nil {
		defer closer()
	}
	if err != nil {
		return err
	}
	wksHome, err := path.CreateDirectory(
		path.WKSHome(kubeconfigOptions.artifactDirectory))
	if err != nil {
		return errors.Wrapf(err, "failed to create WKS home directory")
	}
	sp := specs.NewFromPaths(clusterManifestPath, machinesManifestPath)

	configPath := path.Kubeconfig(wksHome, kubeconfigOptions.namespace, sp.GetClusterName())

	_, err = path.CreateDirectory(filepath.Dir(configPath))
	if err != nil {
		return errors.Wrapf(err, "failed to create configuration directory")
	}

	configStr, err := generateConfig(sp, configPath)
	if err != nil {
		return nil
	}

	err = ioutil.WriteFile(configPath, []byte(configStr), 0644)
	if err != nil {
		return errors.Wrapf(err, "failed to write Kubernetes configuration locally")
	}
	fmt.Printf("To use kubectl with the %s cluster, enter:\n$ export KUBECONFIG=%s\n", sp.GetClusterName(), configPath)
	return nil
}
