package apply

import (
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/addons"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config"
	wksos "github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/wksctl/pkg/manifests"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/kubeadm"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
)

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:    "apply",
	Short:  "Create or update a Kubernetes cluster",
	PreRun: globalPreRun,
	Run:    applyRun,
}

var applyOptions struct {
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
	namespace            string
	useManifestNamespace bool
}

func init() {
	applyCmd.PersistentFlags().StringVar(&applyOptions.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	applyCmd.PersistentFlags().StringVar(&applyOptions.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	applyCmd.PersistentFlags().StringVar(&applyOptions.controllerImage, "controller-image", "quay.io/wksctl/controller:"+imageTag, "Controller image override")
	applyCmd.PersistentFlags().StringVar(&applyOptions.gitURL, "git-url", "", "Git repo containing your cluster and machine information")
	applyCmd.PersistentFlags().StringVar(&applyOptions.gitBranch, "git-branch", "master", "Git branch WKS should use to sync with your cluster")
	applyCmd.PersistentFlags().StringVar(&applyOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	applyCmd.PersistentFlags().StringVar(&applyOptions.gitDeployKeyPath, "git-deploy-key", "", "Path to the Git deploy key")
	applyCmd.PersistentFlags().StringVar(&applyOptions.sealedSecretKeyPath, "sealed-secret-key", "", "Path to a key used to decrypt sealed secrets")
	applyCmd.PersistentFlags().StringVar(&applyOptions.sealedSecretCertPath, "sealed-secret-cert", "", "Path to a certificate used to encrypt sealed secrets")
	applyCmd.PersistentFlags().StringVar(&applyOptions.configDirectory, "config-directory", ".", "Directory containing configuration information for the cluster")
	applyCmd.PersistentFlags().StringVar(&applyOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace override for WKS components")
	applyCmd.PersistentFlags().BoolVar(&applyOptions.useManifestNamespace, "use-manifest-namespace", false, "use namespaces from supplied manifests (overriding any --namespace argument)")
	// Hide controller-image flag as it is a helper/debug flag.
	applyCmd.PersistentFlags().MarkHidden("controller-image")

	rootCmd.AddCommand(applyCmd)
}

func applyRun(cmd *cobra.Command, args []string) {
	// Default to using the git deploy key to decrypt sealed secrets
	if applyOptions.sealedSecretKeyPath == "" && applyOptions.gitDeployKeyPath != "" {
		applyOptions.sealedSecretKeyPath = applyOptions.gitDeployKeyPath
	}

	cpath := filepath.Join(applyOptions.gitPath, applyOptions.clusterManifestPath)
	mpath := filepath.Join(applyOptions.gitPath, applyOptions.machinesManifestPath)
	initiateCluster(manifests.Get(cpath, mpath, applyOptions.gitURL, applyOptions.gitBranch, applyOptions.gitDeployKeyPath, applyOptions.gitPath))
}

func initiateCluster(clusterManifestPath, machinesManifestPath string, closer func()) {
	defer closer()
	sp := specs.NewFromPaths(clusterManifestPath, machinesManifestPath)
	sshClient, err := sp.GetSSHClient(options.verbose)
	if err != nil {
		log.Fatal("Failed to create SSH client: ", err)
	}
	defer sshClient.Close()
	installer, err := wksos.Identify(sshClient)
	if err != nil {
		log.Fatalf("Failed to identify operating system for seed node (%s): %v",
			sp.GetMasterPublicAddress(), err)
	}

	// N.B.: we generate this bootstrap token where wksctl apply is run hoping
	// that this will be on a machine which has been running for a while, and
	// therefore will generate a "more random" token, than we would on a
	// potentially newly created VM which doesn't have much entropy yet.
	token, err := kubeadm.GenerateBootstrapToken()
	if err != nil {
		log.Fatal("Failed to generate bootstrap token: ", err)
	}

	// Point config dir at sync repo if using github and the user didn't override it
	configDir := applyOptions.configDirectory
	if configDir == "." && applyOptions.gitURL != "" {
		configDir = filepath.Dir(clusterManifestPath)
	}

	ns := ""
	if !applyOptions.useManifestNamespace {
		ns = applyOptions.namespace
	}

	// TODO(damien): Transform the controller image into an addon.
	controllerImage, err := addons.UpdateImage(applyOptions.controllerImage, sp.ClusterSpec.ImageRepository)
	if err != nil {
		log.Fatal("Failed to apply the cluster's image repository to the WKS controller's image: ", err)
	}
	if err := installer.SetupSeedNode(wksos.SeedNodeParams{
		PublicIP:             sp.GetMasterPublicAddress(),
		PrivateIP:            sp.GetMasterPrivateAddress(),
		ClusterManifestPath:  clusterManifestPath,
		MachinesManifestPath: machinesManifestPath,
		SSHKeyPath:           sp.GetSSHKeyPath(),
		BootstrapToken:       token,
		KubeletConfig: config.KubeletConfig{
			NodeIP:        sp.GetMasterPrivateAddress(),
			CloudProvider: sp.GetCloudProvider(),
		},
		ControllerImageOverride: controllerImage,
		GitData: wksos.GitParams{
			GitURL:           applyOptions.gitURL,
			GitBranch:        applyOptions.gitBranch,
			GitPath:          applyOptions.gitPath,
			GitDeployKeyPath: applyOptions.gitDeployKeyPath,
		},
		SealedSecretKeyPath:  applyOptions.sealedSecretKeyPath,
		SealedSecretCertPath: applyOptions.sealedSecretCertPath,
		ConfigDirectory:      configDir,
		ImageRepository:      sp.ClusterSpec.ImageRepository,
		ExternalLoadBalancer: sp.ClusterSpec.APIServer.ExternalLoadBalancer,
		AdditionalSANs:       sp.ClusterSpec.APIServer.AdditionalSANs,
		Namespace:            ns,
	}); err != nil {
		log.Fatalf("Failed to set up seed node (%s): %v",
			sp.GetMasterPublicAddress(), err)
	}
}
