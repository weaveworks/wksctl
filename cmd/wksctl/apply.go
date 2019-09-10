package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/weaveworks/wksctl/pkg/addons"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config"
	wksos "github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/kubeadm"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	xcryptossh "golang.org/x/crypto/ssh"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	gogitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
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

	initiateCluster(getManifests(qualifyPath(applyOptions.clusterManifestPath), qualifyPath(applyOptions.machinesManifestPath),
		applyOptions.gitURL, applyOptions.gitBranch, applyOptions.gitDeployKeyPath, applyOptions.gitPath))
}

func qualifyPath(path string) string {
	if applyOptions.gitPath != "" {
		return filepath.Join(applyOptions.gitPath, path)
	}
	return path
}

func getManifests(clusterOption, machinesOption, gitURL, gitBranch, gitDeployKeyPath, gitPath string) (string, string, func()) {
	clusterManifestPath := clusterOption
	machinesManifestPath := machinesOption
	var closer = func() {
	}
	if gitURL != "" {
		clusterManifestPath, machinesManifestPath, closer = syncRepo(gitURL, gitBranch, gitDeployKeyPath, gitPath)
		log.WithFields(log.Fields{"cluster": clusterManifestPath, "machines": machinesManifestPath}).Debug("manifests")
	}
	return clusterManifestPath, machinesManifestPath, closer
}

func cloneOptions(url, deployKeyPath, branch string) gogit.CloneOptions {
	co := gogit.CloneOptions{
		URL: url,
	}
	if branch != "" {
		co.SingleBranch = true
		co.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}
	if deployKeyPath == "" {
		return co
	}
	pem, err := ioutil.ReadFile(deployKeyPath)
	if err != nil {
		log.Fatalf("Failed to read deploy key %s - %v", deployKeyPath, err)
	}
	signer, err := xcryptossh.ParsePrivateKey(pem)
	if err != nil {
		log.Fatalf("Failed to parse private key - %v", err)
	}

	aith := &gogitssh.PublicKeys{User: "git", Signer: signer}
	co.Auth = aith

	return co

}
func syncRepo(url, branch, deployKeyPath, relativeRoot string) (string, string, func()) {
	srcDir, err := ioutil.TempDir("", "wkp")
	if err != nil {
		log.Fatalf("Failed to create temp dir - %v", err)
	}
	closer := func() {
		os.RemoveAll(srcDir)
	}
	lCtx := log.WithField("repo", url)
	ctx := context.Background()
	opt := cloneOptions(url, deployKeyPath, branch)
	r, err := gogit.PlainCloneContext(ctx, srcDir, false, &opt)

	if err != nil {
		closer()
		lCtx.Fatal(err)
	}
	lCtx.WithField("config", r.Config).Debug("cloned")

	rootDir := filepath.Join(srcDir, relativeRoot)
	files, err := ioutil.ReadDir(rootDir)
	if err != nil {
		closer()
		lCtx.Fatalf("Failed to read directory %s - %v", rootDir, err)
	}
	var cYaml, mYaml string
	for _, file := range files {
		switch file.Name() {
		case "cluster.yaml":
			cYaml = filepath.Join(rootDir, file.Name())
		case "machine.yaml", "machines.yaml":
			mYaml = filepath.Join(rootDir, file.Name())
		}
	}
	if cYaml == "" || mYaml == "" {
		closer()
		lCtx.WithField("repo", url).Fatal("Cluster and Machine yaml must be in repo")
	}
	return cYaml, mYaml, closer
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
		Namespace:            sp.GetClusterNamespace(),
	}); err != nil {
		log.Fatalf("Failed to set up seed node (%s): %v",
			sp.GetMasterPublicAddress(), err)
	}
}
