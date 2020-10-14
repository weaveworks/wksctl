package view

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/config"
	capeios "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/wksctl/pkg/manifests"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
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
	sshKeyPath           string
	sealedSecretCertPath string
	sealedSecretKeyPath  string
	configDirectory      string
	verbose              bool
}

func init() {
	Cmd.Flags().StringVarP(&viewOptions.output, "output", "o", "dot", "Output format (dot|json)")
	Cmd.Flags().StringVar(&viewOptions.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	Cmd.Flags().StringVar(&viewOptions.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	Cmd.Flags().StringVar(&viewOptions.controllerImage, "controller-image", "", "Controller image override")
	Cmd.Flags().StringVar(&viewOptions.gitURL, "git-url", "", "Git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&viewOptions.gitBranch, "git-branch", "master", "Git branch WKS should use to read your cluster")
	Cmd.Flags().StringVar(&viewOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	Cmd.Flags().StringVar(&viewOptions.gitDeployKeyPath, "git-deploy-key", "", "Path to the Git deploy key")
	Cmd.Flags().StringVar(&viewOptions.sshKeyPath, "ssh-key", "./cluster-key", "Path to a key authorized to log in to machines by SSH")
	Cmd.Flags().StringVar(&viewOptions.sealedSecretCertPath, "sealed-secret-cert", "", "Path to a certificate used to encrypt sealed secrets")
	Cmd.Flags().StringVar(&viewOptions.sealedSecretKeyPath, "sealed-secret-key", "", "Path to a key used to encrypt sealed secrets")
	Cmd.Flags().StringVar(&viewOptions.configDirectory, "config-directory", ".", "Directory containing configuration information for the cluster")

	// Intentionally shadows the globally defined --verbose flag.
	Cmd.Flags().BoolVarP(&viewOptions.verbose, "verbose", "v", false, "Enable verbose output")
}

func planRun(cmd *cobra.Command, args []string) error {
	var clusterPath, machinesPath string

	// TODO: deduplicate clusterPath/machinesPath evaluation between here and cmd/wksctl/apply
	// https://github.com/weaveworks/wksctl/issues/58
	if viewOptions.gitURL == "" {
		// Cluster and Machine manifests come from the local filesystem.
		clusterPath, machinesPath = viewOptions.clusterManifestPath, viewOptions.machinesManifestPath
	} else {
		// Cluster and Machine manifests come from a Git repo that we'll clone for the duration of this command.
		repo, err := manifests.CloneClusterAPIRepo(viewOptions.gitURL, viewOptions.gitBranch, viewOptions.gitDeployKeyPath, viewOptions.gitPath)
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

	return displayPlan(cmd.Context(), clusterPath, machinesPath)
}

func displayPlan(ctx context.Context, clusterManifestPath, machinesManifestPath string) error {
	// TODO: reuse the actual plan created by `wksctl apply`, rather than trying to construct a similar plan and printing it.
	sp := specs.NewFromPaths(clusterManifestPath, machinesManifestPath)
	sshClient, err := ssh.NewClientForMachine(sp.MasterSpec, sp.ClusterSpec.User, viewOptions.sshKeyPath, viewOptions.verbose)
	if err != nil {
		return errors.Wrap(err, "failed to create SSH client: ")
	}
	defer sshClient.Close()
	installer, err := capeios.Identify(ctx, sshClient)
	if err != nil {
		return errors.Wrapf(err, "failed to identify operating system for seed node (%s)", sp.GetMasterPublicAddress())
	}

	// Point config dir at sync repo if using github and the user didn't override it
	configDir := viewOptions.configDirectory
	if configDir == "." && viewOptions.gitURL != "" {
		configDir = filepath.Dir(clusterManifestPath)
	}

	// Read manifests and pass in the contents
	clusterManifest, err := ioutil.ReadFile(clusterManifestPath)
	if err != nil {
		return errors.Wrap(err, "failed to read cluster manifest: ")
	}

	machinesManifest, err := ioutil.ReadFile(machinesManifestPath)
	if err != nil {
		return errors.Wrap(err, "failed to read machines manifest: ")
	}

	sealedSecretKeyPath := viewOptions.sealedSecretKeyPath
	if sealedSecretKeyPath == "" {
		// Default to using the git deploy key to decrypt sealed secrets
		sealedSecretKeyPath = viewOptions.gitDeployKeyPath
	}

	// Read sealed secret cert and key
	var cert []byte
	var key []byte
	if utilities.FileExists(viewOptions.sealedSecretCertPath) && utilities.FileExists(viewOptions.sealedSecretKeyPath) {
		cert, err = ioutil.ReadFile(viewOptions.sealedSecretCertPath)
		if err != nil {
			return errors.Wrap(err, "failed to read sealed secret certificate: ")
		}

		key, err = ioutil.ReadFile(viewOptions.sealedSecretKeyPath)
		if err != nil {
			return errors.Wrap(err, "failed to read sealed secret key: ")
		}
	}

	params := capeios.SeedNodeParams{
		PublicIP:         sp.GetMasterPublicAddress(),
		PrivateIP:        sp.GetMasterPrivateAddress(),
		ClusterManifest:  string(clusterManifest),
		MachinesManifest: string(machinesManifest),
		KubeletConfig: config.KubeletConfig{
			NodeIP:        sp.GetMasterPrivateAddress(),
			CloudProvider: sp.GetCloudProvider(),
		},
		Controller: capeios.ControllerParams{
			ImageOverride: viewOptions.controllerImage,
		},
		GitData: capeios.GitParams{
			GitURL:           viewOptions.gitURL,
			GitBranch:        viewOptions.gitBranch,
			GitPath:          viewOptions.gitPath,
			GitDeployKeyPath: viewOptions.gitDeployKeyPath,
		},
		SealedSecretCert: string(cert),
		SealedSecretKey:  string(key),
		Namespace:        manifest.DefaultNamespace,
		AddonNamespaces:  manifest.DefaultAddonNamespaces,
		ConfigDirectory:  configDir,
	}
	plan, err := wksos.CreateSeedNodeSetupPlan(ctx, installer, params)
	if err != nil {
		return errors.Wrap(err, "could not generate plan")
	}
	switch viewOptions.output {
	case "dot":
		fmt.Println(plan.ToDOT())
	case "json":
		fmt.Println(plan.ToHumanReadableJSON())
	}
	return nil
}
