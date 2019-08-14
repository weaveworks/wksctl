package main

import (
	"fmt"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config"
	wksos "github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
)

// planCmd represents the plan command
var planCmd = &cobra.Command{
	Use:    "plan",
	Hidden: true,
	Short:  "Debugging commands for the cluster plan.",
	// PreRun: globalPreRun,
	// Run:    planRun,
}

// viewCmd represents the plan view command
var viewCmd = &cobra.Command{
	Use:    "view",
	Hidden: false,
	Short:  "View a cluster plan.",
	PreRun: globalPreRun,
	Run:    planRun,
}

var viewOptions struct {
	output               string
	clusterManifestPath  string
	machinesManifestPath string
	controllerImage      string
	gitURL               string
	gitBranch            string
	gitPath              string
	gitDeployKeyPath     string
	sealedSecretKeyPath  string
	sealedSecretCertPath string
	configDirectory      string
}

func init() {
	viewCmd.Flags().StringVarP(&viewOptions.output, "output", "o", "dot", "Output format (dot|json)")
	viewCmd.PersistentFlags().StringVar(&viewOptions.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	viewCmd.PersistentFlags().StringVar(&viewOptions.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	viewCmd.PersistentFlags().StringVar(&viewOptions.controllerImage, "controller-image", "quay.io/wksctl/controller:"+imageTag, "Controller image override")
	viewCmd.PersistentFlags().StringVar(&viewOptions.gitURL, "git-url", "", "Git repo containing your cluster and machine information")
	viewCmd.PersistentFlags().StringVar(&viewOptions.gitBranch, "git-branch", "master", "Git branch WKS should use to read your cluster")
	viewCmd.PersistentFlags().StringVar(&viewOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	viewCmd.PersistentFlags().StringVar(&viewOptions.gitDeployKeyPath, "git-deploy-key", "", "Path to the Git deploy key")
	viewCmd.PersistentFlags().StringVar(&viewOptions.sealedSecretKeyPath, "sealed-secret-key", "", "Path to a key used to decrypt sealed secrets")
	viewCmd.PersistentFlags().StringVar(&viewOptions.sealedSecretCertPath, "sealed-secret-cert", "", "Path to a certificate used to encrypt sealed secrets")
	viewCmd.PersistentFlags().StringVar(&viewOptions.configDirectory, "config-directory", ".", "Directory containing configuration information for the cluster")
	// Default to using the git deploy key to decrypt sealed secrets
	if viewOptions.sealedSecretKeyPath == "" && viewOptions.gitDeployKeyPath != "" {
		viewOptions.sealedSecretKeyPath = viewOptions.gitDeployKeyPath
	}

	planCmd.AddCommand(viewCmd)
	rootCmd.AddCommand(planCmd)
}

func planRun(cmd *cobra.Command, args []string) {
	// Default to using the git deploy key to decrypt sealed secrets
	if viewOptions.sealedSecretKeyPath == "" && viewOptions.gitDeployKeyPath != "" {
		viewOptions.sealedSecretKeyPath = viewOptions.gitDeployKeyPath
	}

	displayPlan(getManifests(qualifyPath(viewOptions.clusterManifestPath), qualifyPath(viewOptions.machinesManifestPath), viewOptions.gitURL,
		viewOptions.gitBranch, viewOptions.gitDeployKeyPath, viewOptions.gitPath))
}

func displayPlan(clusterManifestPath, machinesManifestPath string, closer func()) {
	defer closer()
	specs := getSpecs(clusterManifestPath, machinesManifestPath)
	sshClient, err := specs.getSSHClient(options.verbose)
	if err != nil {
		log.Fatal("Failed to create SSH client: ", err)
	}
	defer sshClient.Close()
	installer, err := wksos.Identify(sshClient)
	if err != nil {
		log.Fatalf("Failed to identify operating system for seed node (%s): %v",
			specs.getMasterPublicAddress(), err)
	}

	// Point config dir at sync repo if using github and the user didn't override it
	configDir := viewOptions.configDirectory
	if configDir == "." && viewOptions.gitURL != "" {
		configDir = filepath.Dir(clusterManifestPath)
	}

	params := wksos.SeedNodeParams{
		PublicIP:             specs.getMasterPublicAddress(),
		PrivateIP:            specs.getMasterPrivateAddress(),
		ClusterManifestPath:  clusterManifestPath,
		MachinesManifestPath: machinesManifestPath,
		SSHKeyPath:           specs.getSSHKeyPath(),
		KubeletConfig: config.KubeletConfig{
			NodeIP:        specs.getMasterPrivateAddress(),
			CloudProvider: specs.getCloudProvider(),
		},
		ControllerImageOverride: viewOptions.controllerImage,
		GitData: wksos.GitParams{
			GitURL:           viewOptions.gitURL,
			GitBranch:        viewOptions.gitBranch,
			GitPath:          viewOptions.gitPath,
			GitDeployKeyPath: viewOptions.gitDeployKeyPath,
		},
		SealedSecretCertPath: viewOptions.sealedSecretCertPath,
		Namespace:            manifest.DefaultNamespace,
		ConfigDirectory:      configDir,
	}
	plan, err := installer.CreateSeedNodeSetupPlan(params)
	if err != nil {
		fmt.Printf("Could not generate plan: %v\n", err)
		return
	}
	switch viewOptions.output {
	case "dot":
		fmt.Println(plan.ToDOT())
	case "json":
		fmt.Println(plan.ToJSON())
	}
}
