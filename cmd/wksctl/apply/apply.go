package apply

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/config"
	capeios "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/kubeadm"
	"github.com/weaveworks/wksctl/pkg/addons"
	wksos "github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/wksctl/pkg/manifests"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
)

// Cmd represents the apply command
var Cmd = &cobra.Command{
	Use:   "apply",
	Short: "Create or update a Kubernetes cluster",
	RunE:  func(_ *cobra.Command, _ []string) error { a := Applier{&globalParams}; return a.Apply() },
}

type Params struct {
	clusterManifestPath  string
	machinesManifestPath string
	controllerImage      string
	gitURL               string
	gitBranch            string
	gitPath              string
	gitDeployKeyPath     string
	sshKeyPath           string
	sealedSecretKeyPath  string
	sealedSecretCertPath string
	configDirectory      string
	namespace            string
	useManifestNamespace bool
	addonNamespaces      []string
}

var globalParams Params

func init() {
	Cmd.Flags().StringVar(&globalParams.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	Cmd.Flags().StringVar(&globalParams.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	Cmd.Flags().StringVar(&globalParams.gitURL, "git-url", "", "Git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&globalParams.gitBranch, "git-branch", "master", "Git branch WKS should use to sync with your cluster")
	Cmd.Flags().StringVar(&globalParams.gitPath, "git-path", ".", "Relative path to files in Git")
	Cmd.Flags().StringVar(&globalParams.gitDeployKeyPath, "git-deploy-key", "", "Path to the Git deploy key")
	Cmd.Flags().StringVar(&globalParams.sshKeyPath, "ssh-key", "./cluster-key", "Path to a key authorized to log in to machines by SSH")
	Cmd.Flags().StringVar(&globalParams.sealedSecretKeyPath, "sealed-secret-key", "", "Path to a key used to decrypt sealed secrets")
	Cmd.Flags().StringVar(&globalParams.sealedSecretCertPath, "sealed-secret-cert", "", "Path to a certificate used to encrypt sealed secrets")
	Cmd.Flags().StringVar(&globalParams.configDirectory, "config-directory", ".", "Directory containing configuration information for the cluster")
	Cmd.Flags().StringVar(&globalParams.namespace, "namespace", manifest.DefaultNamespace, "namespace override for WKS components")
	Cmd.Flags().BoolVar(&globalParams.useManifestNamespace, "use-manifest-namespace", false, "use namespaces from supplied manifests (overriding any --namespace argument)")
	Cmd.Flags().StringSliceVar(&globalParams.addonNamespaces, "addon-namespace", []string{"weave-net=kube-system"}, "override namespace for specific addons")

	// Hide controller-image flag as it is a helper/debug flag.
	Cmd.Flags().StringVar(&globalParams.controllerImage, "controller-image", "", "Controller image override")
	_ = Cmd.Flags().MarkHidden("controller-image")
}

type Applier struct {
	Params *Params
}

func (a *Applier) Apply() error {
	var clusterPath, machinesPath string

	// TODO: deduplicate clusterPath/machinesPath evaluation between here and other places
	// https://github.com/weaveworks/wksctl/issues/58
	if a.Params.gitURL == "" {
		// Cluster and Machine manifests come from the local filesystem.
		clusterPath, machinesPath = a.Params.clusterManifestPath, a.Params.machinesManifestPath
	} else {
		// Cluster and Machine manifests come from a Git repo that we'll clone for the duration of this command.
		repo, err := manifests.CloneClusterAPIRepo(a.Params.gitURL, a.Params.gitBranch, a.Params.gitDeployKeyPath, a.Params.gitPath)
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

	return a.initiateCluster(clusterPath, machinesPath)
}

func (a *Applier) initiateCluster(clusterManifestPath, machinesManifestPath string) error {
	sp := specs.NewFromPaths(clusterManifestPath, machinesManifestPath)
	sshClient, err := ssh.NewClientForMachine(sp.MasterSpec, sp.ClusterSpec.User, a.Params.sshKeyPath, log.GetLevel() > log.InfoLevel)

	if err != nil {
		return errors.Wrap(err, "failed to create SSH client")
	}
	defer sshClient.Close()
	installer, err := capeios.Identify(sshClient)
	if err != nil {
		return errors.Wrapf(err, "failed to identify operating system for seed node (%s)", sp.GetMasterPublicAddress())
	}

	// N.B.: we generate this bootstrap token where wksctl apply is run hoping
	// that this will be on a machine which has been running for a while, and
	// therefore will generate a "more random" token, than we would on a
	// potentially newly created VM which doesn't have much entropy yet.
	token, err := kubeadm.GenerateBootstrapToken()
	if err != nil {
		return errors.Wrap(err, "failed to generate bootstrap token")
	}

	// Point config dir at sync repo if using github and the user didn't override it
	configDir := a.Params.configDirectory
	if configDir == "." && a.Params.gitURL != "" {
		configDir = filepath.Dir(clusterManifestPath)
	}

	ns := ""
	if !a.Params.useManifestNamespace {
		ns = a.Params.namespace
	}

	addonNamespaces := map[string]string{}
	if len(a.Params.addonNamespaces) > 0 {
		for _, entry := range a.Params.addonNamespaces {
			parts := strings.SplitN(entry, "=", 2)
			if len(parts) == 2 {
				addonNamespaces[parts[0]] = parts[1]
			} else {
				return errors.Errorf("failed to validate the addon namespace (%s)", entry)
			}
		}
	}

	sealedSecretKeyPath := a.Params.sealedSecretKeyPath
	if sealedSecretKeyPath == "" {
		// Default to using the git deploy key to decrypt sealed secrets
		sealedSecretKeyPath = a.Params.gitDeployKeyPath
	}

	// TODO(damien): Transform the controller image into an addon.
	controllerImage := a.Params.controllerImage
	if controllerImage != "" {
		controllerImage, err = addons.UpdateImage(a.Params.controllerImage, sp.ClusterSpec.ImageRepository)
		if err != nil {
			return errors.Wrap(err, "failed to apply the cluster's image repository to the WKS controller's image")
		}
	}

	if err := wksos.SetupSeedNode(installer, wksos.SeedNodeParams{
		PublicIP:             sp.GetMasterPublicAddress(),
		PrivateIP:            sp.GetMasterPrivateAddress(),
		ServicesCIDRBlocks:   sp.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks,
		PodsCIDRBlocks:       sp.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks,
		ClusterManifestPath:  clusterManifestPath,
		MachinesManifestPath: machinesManifestPath,
		SSHKeyPath:           a.Params.sshKeyPath,
		BootstrapToken:       token,
		KubeletConfig: config.KubeletConfig{
			NodeIP:         sp.GetMasterPrivateAddress(),
			CloudProvider:  sp.GetCloudProvider(),
			ExtraArguments: sp.GetKubeletArguments(),
		},
		Controller: wksos.ControllerParams{
			ImageOverride: controllerImage,
		},
		GitData: wksos.GitParams{
			GitURL:           a.Params.gitURL,
			GitBranch:        a.Params.gitBranch,
			GitPath:          a.Params.gitPath,
			GitDeployKeyPath: a.Params.gitDeployKeyPath,
		},
		SealedSecretKeyPath:  sealedSecretKeyPath,
		SealedSecretCertPath: a.Params.sealedSecretCertPath,
		ConfigDirectory:      configDir,
		ImageRepository:      sp.ClusterSpec.ImageRepository,
		ControlPlaneEndpoint: sp.ClusterSpec.ControlPlaneEndpoint,
		AdditionalSANs:       sp.ClusterSpec.APIServer.AdditionalSANs,
		Namespace:            ns,
		AddonNamespaces:      addonNamespaces,
	}); err != nil {
		return errors.Wrapf(err, "failed to set up seed node (%s)", sp.GetMasterPublicAddress())
	}

	return nil
}
