package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/kubernetes/config"
	"github.com/weaveworks/wksctl/pkg/plan/runners/sudo"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/utilities/path"
)

// A new version of the kubeconfig command that retrieves the config from
// /etc/kubernetes/admin.conf on a cluster master node

// kubeconfigCmd represents the kubeconfig command
var kubeconfigCmd = &cobra.Command{
	Use:   "kubeconfig",
	Short: "Generate a kubeconfig file for the cluster",
	Run:   kubeconfigRun,
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
}

func init() {
	kubeconfigCmd.PersistentFlags().StringVar(
		&kubeconfigOptions.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	kubeconfigCmd.PersistentFlags().StringVar(
		&kubeconfigOptions.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	kubeconfigCmd.PersistentFlags().StringVar(&kubeconfigOptions.gitURL, "git-url", "",
		"Git repo containing your cluster and machine information")
	kubeconfigCmd.PersistentFlags().StringVar(&kubeconfigOptions.gitBranch, "git-branch", "master",
		"Branch within git repo containing your cluster and machine information")
	kubeconfigCmd.PersistentFlags().StringVar(&kubeconfigOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	kubeconfigCmd.PersistentFlags().StringVar(&kubeconfigOptions.gitDeployKeyPath, "git-deploy-key", "", "Path to the Git deploy key")
	kubeconfigCmd.PersistentFlags().StringVar(
		&kubeconfigOptions.artifactDirectory, "artifact-directory", "", "Write output files in the specified directory")
	kubeconfigCmd.PersistentFlags().StringVar(
		&kubeconfigOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace portion of kubeconfig path")
	kubeconfigCmd.PersistentFlags().BoolVar(
		&kubeconfigOptions.skipTLSVerify, "insecure-skip-tls-verify", false,
		"Enables kubectl to communicate with the API w/o verifying the certificate")
	kubeconfigCmd.PersistentFlags().MarkHidden("insecure-skip-tls-verify")

	rootCmd.AddCommand(kubeconfigCmd)
}

func configPath(sp *specs.Specs, wksHome string) string {
	clusterName := sp.GetClusterName()
	configDir := path.WKSResourcePath(wksHome, sp.GetClusterNamespace(), clusterName)
	return filepath.Join(configDir, "kubeconfig")
}

// TODO this should be refactored into a common place - i.e. pkg/cluster
func generateConfig(sp *specs.Specs, configPath string) string {
	sshClient, err := sp.GetSSHClient(options.verbose)
	if err != nil {
		log.Fatal("Failed to create SSH client: ", err)
	}
	defer sshClient.Close()

	runner := sudo.Runner{Runner: sshClient}
	configStr, err := runner.RunCommand("cat /etc/kubernetes/admin.conf", nil)
	if err != nil {
		log.Fatalf("Failed to retrieve Kubernetes configuration: %v", err)
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
	return configStr
}

func kubeconfigRun(cmd *cobra.Command, args []string) {
	clusterManifestPath, machinesManifestPath, closer := getManifests(kubeconfigOptions.clusterManifestPath,
		kubeconfigOptions.machinesManifestPath, kubeconfigOptions.gitURL, kubeconfigOptions.gitBranch, kubeconfigOptions.gitDeployKeyPath,
		kubeconfigOptions.gitPath)
	defer closer()
	wksHome, err := path.CreateDirectory(
		path.WKSHome(kubeconfigOptions.artifactDirectory))
	if err != nil {
		log.Fatalf("Failed to create WKS home directory: %v", err)
	}
	sp := specs.NewFromPaths(clusterManifestPath, machinesManifestPath)

	configPath := configPath(sp, wksHome)

	_, err = path.CreateDirectory(filepath.Dir(configPath))
	if err != nil {
		log.Fatalf("Failed to create configuration directory: %v", err)
	}

	configStr := generateConfig(sp, configPath)

	err = ioutil.WriteFile(configPath, []byte(configStr), 0644)
	if err != nil {
		log.Fatalf("Failed to write Kubernetes configuration locally: %v", err)
	}
	fmt.Printf("To use kubectl with the %s cluster, enter:\n$ export KUBECONFIG=%s\n", sp.GetClusterName(), configPath)
}
