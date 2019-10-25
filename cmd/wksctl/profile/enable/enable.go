package enable

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/cmd/wksctl/profile/constants"
	"github.com/weaveworks/wksctl/pkg/git"
)

// Cmd is the command for profile enable
var Cmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable profile",
	Long: `To enable the profile from a specific git URL, run

wksctl profile enable --git-url=<profile_repository> [--revision=master] [--profile-dir=profiles] [--push=true]

If you'd like to specify the revision other than the master branch, use --revision flag.
To disable auto-push, pass --push=false.
`,
	Args: profileEnableArgs,
	Run: func(_ *cobra.Command, _ []string) {
		err := profileEnableRun(profileEnableParams)
		if err != nil {
			log.Fatal(err)
		}
	},
	SilenceUsage: true,
}

type profileEnableFlags struct {
	gitURL     string
	revision   string
	push       bool
	profileDir string
}

var profileEnableParams profileEnableFlags

func init() {
	Cmd.Flags().StringVar(&profileEnableParams.profileDir, "profile-dir", "profiles", "specify a directory for storing profiles")
	Cmd.Flags().StringVar(&profileEnableParams.gitURL, "git-url", "", "enable profile from the gitUrl")
	Cmd.Flags().StringVar(&profileEnableParams.revision, "revision", "master", "use this revision of the profile")
	Cmd.Flags().BoolVar(&profileEnableParams.push, "push", true, "auto push after enable the profile")
}

func profileEnableArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return errors.New("profile enable does not require any argument")
	}
	return nil
}

func profileEnableRun(params profileEnableFlags) error {
	repoURL := params.gitURL

	if repoURL == constants.AppDevAlias {
		repoURL = constants.AppDevRepoURL
	}

	if err := git.IsGitURL(repoURL); err != nil {
		return err
	}

	hostName, repoName, err := git.HostAndRepoPath(repoURL)
	if err != nil {
		return err
	}
	profilePath := path.Join(params.profileDir, hostName, repoName)

	log.Info("Adding the profile to the local repository...")
	err = git.SubtreeAdd(profilePath, repoURL, params.revision)
	if err != nil {
		return err
	}
	log.Info("Added the profile to the local repository.")

	// Detect and process the ignore file if found at the top most directory of the profile
	if err := doIgnoreFiles(profilePath); err != nil {
		return err
	}

	// The default behaviour is auto-commit and push
	if params.push {
		if err := git.Push(); err != nil {
			return err
		}
	}

	return nil
}

func doIgnoreFiles(profilePath string) error {
	ignoreFilePath := path.Join(profilePath, constants.WKSctlIgnoreFilename)
	if _, err := os.Stat(ignoreFilePath); err == nil {
		log.Infof("Ignoring files declared in %s...", constants.WKSctlIgnoreFilename)
		file, err := os.Open(ignoreFilePath)
		if err != nil {
			return err
		}
		defer file.Close()
		pathsToIgnores, err := parseDotIgnorefile(profilePath, file)
		if err != nil {
			return err
		}
		if err := removePathsFromGit(pathsToIgnores...); err != nil {
			return err
		}
		log.Info("Ignored files successfully.")
	}
	return nil
}

func parseDotIgnorefile(dir string, reader io.Reader) ([]string, error) {
	result := []string{}
	scanner := bufio.NewScanner(reader)
	re := regexp.MustCompile(`(?ms)^\s*(?P<pathToIgnore>[^\s#]+).*$`)
	for scanner.Scan() {
		groups := re.FindStringSubmatch(scanner.Text())
		if len(groups) != 2 {
			continue
		}
		pathToIgnore := groups[1]
		result = append(result, path.Join(dir, pathToIgnore))
	}
	return result, nil
}

func removePathsFromGit(paths ...string) error {
	if err := git.RmRecursive(paths...); err != nil {
		return err
	}
	if err := git.Commit(fmt.Sprintf("Ignored files declared in %s", constants.WKSctlIgnoreFilename)); err != nil {
		return err
	}
	return nil
}
