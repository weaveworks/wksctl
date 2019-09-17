package view

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
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
	RunE:   planRun,
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
	sealedSecretCertPath string
	configDirectory      string
	verbose              bool
}

func init() {
	Cmd.Flags().StringVarP(&viewOptions.output, "output", "o", "dot", "Output format (dot|json)")
	Cmd.Flags().StringVar(&viewOptions.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	Cmd.Flags().StringVar(&viewOptions.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	Cmd.Flags().StringVar(&viewOptions.controllerImage, "controller-image", "quay.io/wksctl/controller:"+version.ImageTag, "Controller image override")
	Cmd.Flags().StringVar(&viewOptions.gitURL, "git-url", "", "Git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&viewOptions.gitBranch, "git-branch", "master", "Git branch WKS should use to read your cluster")
	Cmd.Flags().StringVar(&viewOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	Cmd.Flags().StringVar(&viewOptions.gitDeployKeyPath, "git-deploy-key", "", "Path to the Git deploy key")
	Cmd.Flags().StringVar(&viewOptions.sealedSecretCertPath, "sealed-secret-cert", "", "Path to a certificate used to encrypt sealed secrets")
	Cmd.Flags().StringVar(&viewOptions.configDirectory, "config-directory", ".", "Directory containing configuration information for the cluster")

	// Intentionally shadows the globally defined --verbose flag.
	Cmd.Flags().BoolVar(&viewOptions.verbose, "verbose", false, "Enable verbose output")
}

func planRun(cmd *cobra.Command, args []string) error {
	var cpath, mpath string

	// TODO: deduplicate cpath/mpath evaluation between here and cmd/wksctl/apply
	if viewOptions.gitURL == "" {
		// Cluster and Manifests come from the local filesystem.
		cpath, mpath = viewOptions.clusterManifestPath, viewOptions.machinesManifestPath
	} else {
		// Cluster and Machine manifests come from a Git repo that we'll clone for the duration of this command.
		repo, err := manifests.CloneClusterAPIRepo(viewOptions.gitURL, viewOptions.gitBranch, viewOptions.gitDeployKeyPath, viewOptions.gitPath)
		if err != nil {
			return errors.Wrap(err, "CloneClusterAPIRepo")
		}
		defer repo.Close()

		if cpath, err = repo.ClusterManifestPath(); err != nil {
			return errors.Wrap(err, "ClusterManifestPath")
		}
		if mpath, err = repo.MachinesManifestPath(); err != nil {
			return errors.Wrap(err, "MachinesManifestPath")
		}
	}

	return displayPlan(cpath, mpath)
}

func displayPlan(clusterManifestPath, machinesManifestPath string) error {
	// TODO: reuse the actual plan created by `wksctl apply`, rather than trying to construct a similar plan and printing it.
	sp := specs.NewFromPaths(clusterManifestPath, machinesManifestPath)
	sshClient, err := sp.GetSSHClient(viewOptions.verbose)
	if err != nil {
		return errors.Wrap(err, "failed to create SSH client: ")
	}
	defer sshClient.Close()
	installer, err := os.Identify(sshClient)
	if err != nil {
		return errors.Wrapf(err, "failed to identify operating system for seed node (%s)", sp.GetMasterPublicAddress())
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
		return errors.Wrap(err, "could not generate plan")
	}
	switch viewOptions.output {
	case "dot":
		fmt.Println(plan.ToDOT())
	case "json":
		fmt.Println(plan.ToJSON())
	}
	return nil
}
