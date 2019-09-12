package view

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/wksctl/pkg/manifests"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/version"
)

// Cmd represents the plan view command
var Cmd = &cobra.Command{
	Use:    "view",
	Hidden: false,
	Short:  "View a cluster plan.",
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
	verbose              bool
}

func init() {
	Cmd.Flags().StringVarP(&viewOptions.output, "output", "o", "dot", "Output format (dot|json)")
	Cmd.PersistentFlags().StringVar(&viewOptions.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	Cmd.PersistentFlags().StringVar(&viewOptions.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	Cmd.PersistentFlags().StringVar(&viewOptions.controllerImage, "controller-image", "quay.io/wksctl/controller:"+version.ImageTag, "Controller image override")
	Cmd.PersistentFlags().StringVar(&viewOptions.gitURL, "git-url", "", "Git repo containing your cluster and machine information")
	Cmd.PersistentFlags().StringVar(&viewOptions.gitBranch, "git-branch", "master", "Git branch WKS should use to read your cluster")
	Cmd.PersistentFlags().StringVar(&viewOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	Cmd.PersistentFlags().StringVar(&viewOptions.gitDeployKeyPath, "git-deploy-key", "", "Path to the Git deploy key")
	Cmd.PersistentFlags().StringVar(&viewOptions.sealedSecretKeyPath, "sealed-secret-key", "", "Path to a key used to decrypt sealed secrets")
	Cmd.PersistentFlags().StringVar(&viewOptions.sealedSecretCertPath, "sealed-secret-cert", "", "Path to a certificate used to encrypt sealed secrets")
	Cmd.PersistentFlags().StringVar(&viewOptions.configDirectory, "config-directory", ".", "Directory containing configuration information for the cluster")

	// Intentionally shadows the globally defined --verbose flag.
	Cmd.Flags().BoolVar(&viewOptions.verbose, "verbose", false, "Enable verbose output")

	// Default to using the git deploy key to decrypt sealed secrets
	// BUG: CLI flags are not evaluated yet at this point!
	if viewOptions.sealedSecretKeyPath == "" && viewOptions.gitDeployKeyPath != "" {
		viewOptions.sealedSecretKeyPath = viewOptions.gitDeployKeyPath
	}
}

func planRun(cmd *cobra.Command, args []string) {
	// Default to using the git deploy key to decrypt sealed secrets
	if viewOptions.sealedSecretKeyPath == "" && viewOptions.gitDeployKeyPath != "" {
		viewOptions.sealedSecretKeyPath = viewOptions.gitDeployKeyPath
	}

	cpath := filepath.Join(viewOptions.gitPath, viewOptions.clusterManifestPath)
	mpath := filepath.Join(viewOptions.gitPath, viewOptions.machinesManifestPath)
	displayPlan(manifests.Get(cpath, mpath, viewOptions.gitURL, viewOptions.gitBranch, viewOptions.gitDeployKeyPath, viewOptions.gitPath))
}

func displayPlan(clusterManifestPath, machinesManifestPath string, closer func()) {
	defer closer()
	sp := specs.NewFromPaths(clusterManifestPath, machinesManifestPath)
	sshClient, err := sp.GetSSHClient(viewOptions.verbose)
	if err != nil {
		log.Fatal("Failed to create SSH client: ", err)
	}
	defer sshClient.Close()
	installer, err := os.Identify(sshClient)
	if err != nil {
		log.Fatalf("Failed to identify operating system for seed node (%s): %v",
			sp.GetMasterPublicAddress(), err)
	}

	// Point config dir at sync repo if using github and the user didn't override it
	configDir := viewOptions.configDirectory
	if configDir == "." && viewOptions.gitURL != "" {
		configDir = filepath.Dir(clusterManifestPath)
	}

	params := os.SeedNodeParams{
		PublicIP:             sp.GetMasterPublicAddress(),
		PrivateIP:            sp.GetMasterPrivateAddress(),
		ClusterManifestPath:  clusterManifestPath,
		MachinesManifestPath: machinesManifestPath,
		SSHKeyPath:           sp.GetSSHKeyPath(),
		KubeletConfig: config.KubeletConfig{
			NodeIP:        sp.GetMasterPrivateAddress(),
			CloudProvider: sp.GetCloudProvider(),
		},
		ControllerImageOverride: viewOptions.controllerImage,
		GitData: os.GitParams{
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
