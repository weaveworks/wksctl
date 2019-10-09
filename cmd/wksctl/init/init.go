package init

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/version"
)

// A command that initializes a user's cloned git repository with a correct image tag for wks-controller and
// updated git information for flux manifests.

type initOptionType struct {
	footlooseIP        string
	footlooseBackend   string
	gitURL             string
	gitBranch          string
	gitPath            string
	localRepoDirectory string
	namespace          string
	version            string
}

type manifestUpdate struct {
	selector func([]byte) bool
	updater  func([]byte, initOptionType) ([]byte, error)
}

var (
	// Cmd represents the init command
	Cmd = &cobra.Command{
		Use:           "init",
		Short:         "Update stored kubernetes manifests to match the local cluster environment",
		Long:          "'wksctl init' configures existing kubernetes 'flux.yaml' and 'wks-controller.yaml' manifests in a repository with information about the local GitOps repository, the preferred weave system namespace, and current container image tags. The files can be anywhere in the repository. If either file is absent, 'wksctl init' will return an error.",
		Example:       "wksctl init --namespace=wksctl --git-url=git@github.com:haskellcurry/lambda.git --git-branch=development --git-path=src",
		RunE:          initRun,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	initOptions initOptionType

	namespacePrefixPattern          = "kind: Namespace\n  metadata:\n    name: "
	namespaceNamePattern            = multiLineRegexp(namespacePrefixPattern + `\S+`)
	controllerFootlooseAddrLocation = multiLineRegexp(`(\s*)- name: controller`)
	controllerFootlooseEnvEntry     = multiLineRegexp(`env:\n\s*- name: FOOTLOOSE_SERVER_ADDR`)
	controllerImageSegment          = multiLineRegexp(`(image:\s*\S*[/]controller)(:\s*\S+)?`)
	namespacePattern                = multiLineRegexp(`namespace:\s*\S+`)
	gitURLPattern                   = multiLineRegexp(`(--git-url)=\S+`)
	gitBranchPattern                = multiLineRegexp(`(--git-branch)=\S+`)
	gitPathPattern                  = multiLineRegexp(`(--git-path)=\S+`)

	updates = []manifestUpdate{
		{selector: equal("wks-controller.yaml"), updater: updateControllerManifests},
		{selector: and(prefix("flux"), extension("yaml")), updater: updateFluxManifests}}
)

func multiLineRegexp(pattern string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)` + pattern)
}

func init() {
	Cmd.Flags().StringVar(
		&initOptions.footlooseIP, "footloose-ip", "172.17.0.1", "address of footloose server on host")
	Cmd.Flags().StringVar(
		&initOptions.footlooseBackend, "footloose-backend", "docker", "which machine backend to use: ignite or docker")
	Cmd.Flags().StringVar(
		&initOptions.localRepoDirectory, "gitk8s-clone", ".", "Local location of cloned git repository")
	Cmd.Flags().StringVar(&initOptions.gitURL, "git-url", "",
		"Git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&initOptions.gitBranch, "git-branch", "master",
		"Branch within git repo containing your cluster and machine information")
	Cmd.Flags().StringVar(&initOptions.gitPath, "git-path", ".", "Relative path to files in Git")
	Cmd.Flags().StringVar(
		&initOptions.namespace, "namespace", manifest.DefaultNamespace, "namespace portion of kubeconfig path")
	Cmd.MarkPersistentFlagRequired("git-url")
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
	withVersion := controllerImageSegment.ReplaceAll(contents, []byte(`$1:`+options.version))
	if controllerFootlooseEnvEntry.Find(withVersion) == nil {
		return controllerFootlooseAddrLocation.ReplaceAll(withVersion,
			// We want to add to the matched entry so we start with $0 (the entire match) and use $1 to get the indentation correct.
			// The $1 contains a leading newline.
			[]byte(fmt.Sprintf("$0$1  env:$1  - name: FOOTLOOSE_SERVER_ADDR$1    value: %s$1  - name: FOOTLOOSE_BACKEND$1    value: %s", options.footlooseIP, options.footlooseBackend))), nil
	}
	return withVersion, nil
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
	matches := 0
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
					matches++
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
	if matches < len(updates) {
		return errors.New("Both 'flux.yaml' and 'wks-controller.yaml' must be present in the repository")
	}
	return nil
}

func initRun(cmd *cobra.Command, args []string) error {
	initOptions.version = version.Version // from main command
	return updateManifests(initOptions)
}
