package applyaddons

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/launcher/pkg/kubectl"
	"github.com/weaveworks/wksctl/pkg/addons"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/utilities/path"
)

var Cmd = &cobra.Command{
	Use:    "apply-addons",
	Short:  "Apply Addons",
	Hidden: true,
	Run:    applyAddonsRun,
}

var applyAddonsOptions struct {
	clusterManifestPath  string
	machinesManifestPath string
	artifactDirectory    string
	namespace            string
}

func init() {
	opts := &applyAddonsOptions
	Cmd.Flags().StringVar(&opts.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	Cmd.Flags().StringVar(&opts.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	Cmd.Flags().StringVar(
		&opts.artifactDirectory, "artifact-directory", "", "Location of WKS artifacts ")
	Cmd.Flags().StringVar(
		&applyAddonsOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace portion of kubeconfig path")
}

func applyAddonsUsingConfig(sp *specs.Specs, basePath, kubeconfig string) error {
	fmt.Println("==> Applying addons (2)")

	for _, addonDesc := range sp.ClusterSpec.Addons {
		log.Debugf("applying addon '%s'", addonDesc.Name)

		// Generate the addon manifest.
		addon, err := addons.Get(addonDesc.Name)
		if err != nil {
			return err
		}

		tmpDir, err := ioutil.TempDir("", "wksctl-apply-addons")
		if err != nil {
			return err
		}

		manifests, err := addon.Build(addons.BuildOptions{
			OutputDirectory: tmpDir,
			BasePath:        basePath,
			ImageRepository: sp.ClusterSpec.ImageRepository,
			Params:          addonDesc.Params,
		})
		if err != nil {
			return err
		}

		log.Debugf("using kubeconfig %s", kubeconfig)
		c := &kubectl.LocalClient{
			Env: []string{
				fmt.Sprintf("KUBECONFIG=%s", kubeconfig),
			},
		}
		for _, manifest := range manifests {
			if err := kubectl.Apply(c, manifest); err != nil {
				return err
			}
		}

		// Remove the generated manifest files.
		os.RemoveAll(tmpDir)
	}

	return nil
}

func applyAddonsRun(cmd *cobra.Command, args []string) {
	opts := &applyAddonsOptions
	sp := specs.NewFromPaths(opts.clusterManifestPath, opts.machinesManifestPath)
	configPath := path.Kubeconfig(opts.artifactDirectory, applyAddonsOptions.namespace, sp.GetClusterName())

	if !configExists(configPath) {
		log.Fatal(strings.Join([]string{
			"==> Kubernetes configuration doesn't exist.",
			"    Please generate one using wksctl kubeconfig",
		}, "\n"))

	}

	if err := applyAddonsUsingConfig(sp, filepath.Dir(opts.clusterManifestPath), configPath); err != nil {
		log.Fatal("Error applying addons: ", err)
	}
}

// configExists checks to see if a config file already exists on the client
func configExists(configPath string) bool {
	_, err := os.Stat(configPath)
	return !os.IsNotExist(err)
}
