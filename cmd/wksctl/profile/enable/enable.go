package enable

import (
	"errors"
	"path"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/weaveworks/wksctl/cmd/wksctl/profile/constants"
	"github.com/weaveworks/wksctl/pkg/git"
)

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
	gitUrl     string
	revision   string
	push       bool
	profileDir string
}

var profileEnableParams profileEnableFlags

func init() {
	Cmd.Flags().StringVar(&profileEnableParams.profileDir, "profile-dir", "profiles", "specify a directory for storing profiles")
	Cmd.Flags().StringVar(&profileEnableParams.gitUrl, "git-url", "", "enable profile from the gitUrl")
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
	repoUrl := params.gitUrl

	if repoUrl == constants.AppDevAlias {
		repoUrl = constants.AppDevRepoURL
	}

	if err := git.IsGitURL(repoUrl); err != nil {
		return err
	}

	hostName, repoName, err := git.HostAndRepoPath(repoUrl)
	if err != nil {
		return err
	}
	clonePath := path.Join(params.profileDir, hostName, repoName)

	log.Info("Adding the profile to the local repository...")
	err = git.SubtreeAdd(clonePath, repoUrl, params.revision)
	if err != nil {
		return err
	}
	log.Info("Added the profile to the local repository.")

	// The default behaviour is auto-commit and push
	if params.push {
		if err := git.Push(); err != nil {
			return err
		}
	}

	return nil
}
