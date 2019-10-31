package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	giturls "github.com/whilp/git-urls"
)

func gitExec(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func HasNoStagedChanges() error {
	return errors.Wrap(gitExec("diff", "--quiet", "--staged", "--exit-code"), "repository contains staged changes")
}

func RmRecursive(paths ...string) error {
	return gitExec(append([]string{"rm", "-r", "--"}, paths...)...)
}

func AddAll(path string) error {
	return gitExec("add", "-A", path)
}

func SubtreeAdd(path, repository, revision string) error {
	return gitExec("subtree", "add", "--squash", "--prefix", path, repository, revision)
}

func Commit(message string) error {
	log.Infof("Committing the changes...")
	err := gitExec("commit", "-m", message)
	if err == nil {
		log.Info("Committed the changes.")
	}
	return err
}

func Push() error {
	log.Info("Pushing to the remote...")
	err := gitExec("push", "-v", "origin", "HEAD")
	if err == nil {
		log.Info("Pushed successfully.")
	}
	return err
}

func HostAndRepoPath(repoURL string) (string, string, error) {
	url, err := giturls.Parse(repoURL)
	if err != nil {
		return "", "", errors.Wrapf(err, "unable to parse git URL '%s'", repoURL)
	}

	return url.Hostname(), strings.TrimRight(url.Path, ".git"), nil
}

func IsGitURL(rawURL string) error {
	parsedURL, err := giturls.Parse(rawURL)
	if err != nil {
		return err
	}
	if !parsedURL.IsAbs() {
		return fmt.Errorf("URL %q does not have a scheme", parsedURL)
	}
	if parsedURL.Hostname() == "" {
		return fmt.Errorf("URL %q has an empty hostname", parsedURL)
	}
	return nil
}
