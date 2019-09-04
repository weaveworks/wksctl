package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// A command that initializes a user's cloned git repository with a correct image tag for wks-controller and
// git information for flux manifests.

type manifestUpdate struct {
	selector func([]byte) bool
	updater  func([]byte) ([]byte, error)
}

var (
	// initCmd represents the init command
	initCmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize kubernetes manifests for the cluster",
		Run:   initRun,
	}

	initOptions struct {
		localRepoDirectory string
	}

	controllerImageSegment = regexp.MustCompile(`(image:[ ]*[^ ]*[/]controller):[^ ]*`)

	updates = []manifestUpdate{
		{selector: equal("wks-controller.yaml"), updater: updateControllerManifests},
		{selector: and(prefix("flux"), extension("yaml")), updater: updateFluxManifests}}
)

func init() {
	initCmd.PersistentFlags().StringVar(
		&initOptions.localRepoDirectory, "gitk8s-clone", ".", "Local location of cloned git repository")
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
		return filepath.Ext(string(fname)[1:]) == ext
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

func updateControllerManifests(contents []byte) ([]byte, error) {
	return controllerImageSegment.ReplaceAll(contents, []byte(`$1:`+version)), nil
}

// Not doing anything yet...
func updateFluxManifests(contents []byte) ([]byte, error) {
	return controllerImageSegment.ReplaceAll(contents, []byte(`$1:`+version)), nil
}

func updateManifests() {
	filepath.Walk(initOptions.localRepoDirectory,
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
					newContents, err := u.updater(contents)
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
	updateManifests()
}
