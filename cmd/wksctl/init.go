package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
)

// A command that initializes a user's cloned git repository with a correct image tag for wks-controller and
// updated git information for flux manifests.

type initOptionType struct {
	localRepoDirectory string
	gitURL             string
	gitBranch          string
	gitPath            string
	namespace          string
	version            string
}

type manifestUpdate struct {
	selector func([]byte) bool
	updater  func([]byte, initOptionType) ([]byte, error)
}

var (
	// initCmd represents the init command
	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Update stored kubernetes manifests to match the local cluster environment",
		Run:   initRun,
	}

	initOptions initOptionType

	namespacePrefixPattern = "kind: Namespace\n  metadata:\n    name: "
	namespaceNamePattern   = multiLineRegexp(namespacePrefixPattern + `\S+`)
	controllerImageSegment = multiLineRegexp(`(image:\s*\S*[/]controller)(:\s*\S+)?`)
	namespacePattern       = multiLineRegexp(`namespace:\s*\S+`)
	gitURLPattern          = multiLineRegexp(`(--git-url)=\S+`)
	gitBranchPattern       = multiLineRegexp(`(--git-branch)=\S+`)
	gitPathPattern         = multiLineRegexp(`(--git-path)=\S+`)

	updates = []manifestUpdate{
		{selector: equal("wks-controller.yaml"), updater: updateControllerManifests},
		{selector: and(prefix("flux"), extension("yaml")), updater: updateFluxManifests}}
)

func multiLineRegexp(pattern string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)` + pattern)
}

func init() {
	initCmd.PersistentFlags().StringVar(
		&initOptions.localRepoDirectory, "gitk8s-clone", ".", "Local location of cloned git repository")
	initCmd.PersistentFlags().StringVar(&initOptions.gitURL, "git-url", "",
		"Git repo containing your cluster and machine information")
	initCmd.PersistentFlags().StringVar(&initOptions.gitBranch, "git-branch", "master",
		"Branch within git repo containing your cluster and machine information")
	initCmd.PersistentFlags().StringVar(&initOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	initCmd.PersistentFlags().StringVar(
		&initOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace portion of kubeconfig path")
	initCmd.MarkPersistentFlagRequired("git-url")
	rootCmd.AddCommand(initCmd)
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
		return filepath.Ext(string(fname))[1:] == ext
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
	return []byte(fmt.Sprintf("$1=%q", item))
}

func updateControllerManifests(contents []byte, options initOptionType) ([]byte, error) {
	return controllerImageSegment.ReplaceAll(contents, []byte(`$1:`+options.version)), nil
}

func updateFluxManifests(contents []byte, options initOptionType) ([]byte, error) {
	withNamespaceName := namespaceNamePattern.ReplaceAll(contents, []byte(namespacePrefixPattern+options.namespace))
	withNamespace := namespacePattern.ReplaceAll(withNamespaceName, []byte(`namespace: `+options.namespace))
	withGitURL := gitURLPattern.ReplaceAll(withNamespace, updatedArg(options.gitURL))
	withGitBranch := gitBranchPattern.ReplaceAll(withGitURL, updatedArg(options.gitBranch))
	withGitPath := gitPathPattern.ReplaceAll(withGitBranch, updatedArg(options.gitPath))
	return withGitPath, nil
}

func updateManifests(options initOptionType) {
	filepath.Walk(options.localRepoDirectory,
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
					contents, err := ioutil.ReadFile(path)
					if err != nil {
						return err
					}
					newContents, err := u.updater(contents, options)
					if err != nil {
						return err
					}
					ioutil.WriteFile(path, newContents, info.Mode())
					// Don't break; if multiple files "match", make sure we update all of them
				}
			}
			return nil
		})
}

func initRun(cmd *cobra.Command, args []string) {
	initOptions.version = version // from main command
	updateManifests(initOptions)
}
