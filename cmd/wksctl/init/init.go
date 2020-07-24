package init

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"

	"github.com/pelletier/go-toml"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	wksos "github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/wksctl/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/version"
)

// A command that initializes a user's cloned git repository with a correct image tag for wks-controller and
// updated git information for flux manifests.

type initOptionType struct {
	dependencyPath       string
	gitURL               string
	gitBranch            string
	gitPath              string
	localRepoDirectory   string
	namespace            string
	version              string
	clusterManifestPath  string
	machinesManifestPath string
}

type manifestUpdate struct {
	name     string
	selector func([]byte) bool
	updater  func([]byte, initOptionType) ([]byte, error)
}

var (
	// Cmd represents the init command
	Cmd = &cobra.Command{
		Use:          "init",
		Short:        "Update stored kubernetes manifests to match the local cluster environment",
		Long:         "'wksctl init' configures existing kubernetes 'flux.yaml', 'wks-controller.yaml' and 'weave-net.yaml' manifests in a repository with information about the local GitOps repository, the preferred weave system namespace, and current container image tags. The files can be anywhere in the repository. If either file is absent, 'wksctl init' will return an error.",
		Example:      "wksctl init --namespace=wksctl --git-url=git@github.com:haskellcurry/lambda.git --git-branch=development --git-path=src",
		RunE:         initRun,
		SilenceUsage: true,
	}

	initOptions initOptionType

	namespacePrefixPattern = "kind: Namespace\n  metadata:\n    name: "
	namespaceNamePattern   = multiLineRegexp(namespacePrefixPattern + `\S+`)
	controllerImageSegment = multiLineRegexp(`(image:\s*\S*[-]controller)(:\s*\S+)?`)
	namespacePattern       = multiLineRegexp(`namespace:\s*\S+`)
	gitURLPattern          = multiLineRegexp(`(--git-url)=\S+`)
	gitBranchPattern       = multiLineRegexp(`(--git-branch)=\S+`)
	gitPathPattern         = multiLineRegexp(`(--git-path)=\S+`)

	updates = []manifestUpdate{
		{name: "weave-net", selector: equal("weave-net.yaml"), updater: updateWeaveNetManifests},
		{name: "wks-controller", selector: equal("wks-controller.yaml"), updater: updateControllerManifests},
		{name: "flux", selector: and(prefix("flux"), extension("yaml")), updater: updateFluxManifests}}

	dependencies = &toml.Tree{}
)

func multiLineRegexp(pattern string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)` + pattern)
}

func init() {
	Cmd.Flags().StringVar(
		&initOptions.localRepoDirectory, "gitk8s-clone", ".", "Local location of cloned git repository")
	Cmd.Flags().StringVar(&initOptions.gitURL, "git-url", "",
		"Git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&initOptions.gitBranch, "git-branch", "master",
		"Branch within git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&initOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	Cmd.Flags().StringVar(
		&initOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace portion of kubeconfig path")
	Cmd.Flags().StringVar(
		&initOptions.version, "controller-version", version.Version, "version of wks-controller to use")
	Cmd.Flags().StringVar(&initOptions.clusterManifestPath, "cluster", "cluster.yaml", "Location of cluster manifest")
	Cmd.Flags().StringVar(&initOptions.machinesManifestPath, "machines", "machines.yaml", "Location of machines manifest")
	Cmd.Flags().StringVar(
		&initOptions.dependencyPath, "dependency-file", "./dependencies.toml", "path to file containing version information for all dependencies")
	_ = Cmd.MarkPersistentFlagRequired("git-url")
}

// selectors
func equal(name string) func([]byte) bool {
	return func(fname []byte) bool {
		return name == string(fname)
	}
}

func prefix(pre string) func([]byte) bool {
	return func(fname []byte) bool {
		return strings.HasPrefix(string(fname), pre)
	}
}

func extension(ext string) func([]byte) bool {
	return func(fname []byte) bool {
		extension := filepath.Ext(string(fname))
		return len(extension) > 0 && extension[1:] == ext
	}
}

func and(checks ...func([]byte) bool) func([]byte) bool {
	if len(checks) == 0 {
		return func(_ []byte) bool { return true }
	}
	return func(name []byte) bool {
		return checks[0](name) && and(checks[1:]...)(name)
	}
}

func updatedArg(item string) []byte {
	return []byte(fmt.Sprintf("$1=%s", item))
}

func updateControllerManifests(contents []byte, options initOptionType) ([]byte, error) {
	controllerVersion, ok := dependencies.Get("controller.version").(string)
	if !ok {
		controllerVersion = options.version
	}
	var withVersion []byte
	if strings.Contains(controllerVersion, "/") {
		withVersion = controllerImageSegment.ReplaceAll(contents, []byte(`image: `+controllerVersion))
	} else {
		withVersion = controllerImageSegment.ReplaceAll(contents, []byte(`$1:`+controllerVersion))
	}
	return withVersion, nil
}

func updateWeaveNetManifests(contents []byte, options initOptionType) ([]byte, error) {
	clusterManifestPath := (path.Join(options.localRepoDirectory, options.clusterManifestPath))
	machinesManifestPath := (path.Join(options.localRepoDirectory, options.machinesManifestPath))
	sp := specs.NewFromPaths(clusterManifestPath, machinesManifestPath)

	podsCIDRBlocks := sp.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks
	if len(podsCIDRBlocks) > 0 && podsCIDRBlocks[0] != "" {
		// setting the pod CIDR block is currently only supported for the weave-net CNI
		log.Debug("Updating weave-net manifest.")
		manifests, err := wksos.SetWeaveNetPodCIDRBlock([][]byte{contents}, podsCIDRBlocks[0])
		if err != nil {
			return nil, errors.Wrap(err, "failed to inject ipalloc_range")
		}
		return manifests[0], nil
	}

	log.Debug("No change to weave-net manifest")
	return contents, nil
}

func updateFluxManifests(contents []byte, options initOptionType) ([]byte, error) {
	withNamespaceName := namespaceNamePattern.ReplaceAll(contents, []byte(namespacePrefixPattern+options.namespace))
	withNamespace := namespacePattern.ReplaceAll(withNamespaceName, []byte(`namespace: `+options.namespace))
	withGitURL := gitURLPattern.ReplaceAll(withNamespace, updatedArg(options.gitURL))
	withGitBranch := gitBranchPattern.ReplaceAll(withGitURL, updatedArg(options.gitBranch))
	withGitPath := gitPathPattern.ReplaceAll(withGitBranch, updatedArg(options.gitPath))
	return withGitPath, nil
}

func updateManifests(options initOptionType) error {
	found := map[string]bool{}
	err := filepath.Walk(options.localRepoDirectory,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			fname := []byte(info.Name())
			for _, u := range updates {
				if u.selector(fname) {
					log.Debugf("Matched %s", fname)
					found[u.name] = true
					contents, err := ioutil.ReadFile(path)
					if err != nil {
						return err
					}
					newContents, err := u.updater(contents, options)
					if err != nil {
						return err
					}
					if err := ioutil.WriteFile(path, newContents, info.Mode()); err != nil {
						return err
					}
					// Don't break; if multiple files "match", make sure we update all of them
				}
			}
			return nil
		})
	if !found["flux"] || !found["wks-controller"] {
		return errors.New("Both 'flux.yaml' and 'wks-controller.yaml' must be present in the repository")
	}
	return err
}

func initRun(cmd *cobra.Command, args []string) error {
	if initOptions.version == "" {
		initOptions.version = version.Version // from main command
	}
	bytes, err := ioutil.ReadFile(initOptions.dependencyPath)
	if err != nil {
		return err
	}
	dependencies, err = toml.Load(string(bytes))
	if err != nil {
		return err
	}
	return updateManifests(initOptions)
}
