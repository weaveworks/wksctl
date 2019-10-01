package manifests

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	xcryptossh "golang.org/x/crypto/ssh"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	gogitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

type ClusterAPIRepo struct {
	// worktreePath is the absolute path to the cloned worktree.
	worktreePath string
	// subDir is the subdirectory of worktreePath where we look for files.
	subdir string
}

func (r *ClusterAPIRepo) Close() error {
	return os.RemoveAll(r.worktreePath)
}

func (r *ClusterAPIRepo) ClusterManifestPath() (string, error) {
	path := filepath.Join(r.worktreePath, r.subdir, "cluster.yaml")
	if _, err := os.Stat(path); err != nil {
		return "", errors.Wrap(err, "cluster manifest not readable")
	}
	return path, nil
}

func (r *ClusterAPIRepo) MachinesManifestPath() (string, error) {
	candidates := []string{"machine.yaml", "machines.yaml"}

	for _, c := range candidates {
		fqCand := filepath.Join(r.worktreePath, r.subdir, c)
		if _, err := os.Stat(fqCand); err == nil {
			return fqCand, nil
		}
	}
	return "", errors.New("machines manifest not found")
}

func CloneClusterAPIRepo(url, branch, keyPath, subdir string) (*ClusterAPIRepo, error) {
	var worktreePath string
	var err error

	if worktreePath, err = ioutil.TempDir("", "wkp"); err != nil {
		return nil, errors.Wrap(err, "TempDir")
	}

	if worktreePath, err = filepath.Abs(worktreePath); err != nil {
		return nil, errors.Wrap(err, "filepath.Abs")
	}

	r := ClusterAPIRepo{
		worktreePath: worktreePath,
		subdir:       subdir,
	}

	opt, err := cloneOptions(url, keyPath, branch)
	if err != nil {
		r.Close()
		return nil, errors.Wrap(err, "cloneOptions")
	}

	if _, err := gogit.PlainClone(r.worktreePath, false, &opt); err != nil {
		r.Close()
		return nil, errors.Wrapf(err, "failed to clone repository: %s", url)
	}

	return &r, nil
}

func cloneOptions(url, deployKeyPath, branch string) (gogit.CloneOptions, error) {
	co := gogit.CloneOptions{
		URL: url,
	}
	if branch != "" {
		co.SingleBranch = true
		co.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}
	if deployKeyPath == "" {
		return co, nil
	}
	pem, err := ioutil.ReadFile(deployKeyPath)
	if err != nil {
		return co, errors.Wrapf(err, "failed to read deploy key: %s", deployKeyPath)
	}
	signer, err := xcryptossh.ParsePrivateKey(pem)
	if err != nil {
		return co, errors.Wrapf(err, "failed to parse private key")
	}

	aith := &gogitssh.PublicKeys{User: "git", Signer: signer}
	co.Auth = aith

	return co, nil
}
