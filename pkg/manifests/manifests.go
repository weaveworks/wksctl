package manifests

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	xcryptossh "golang.org/x/crypto/ssh"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	gogitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

func Get(clusterOption, machinesOption, gitURL, gitBranch, gitDeployKeyPath, gitPath string) (string, string, func(), error) {
	var closer = func() {}
	var err error

	clusterManifestPath := clusterOption
	machinesManifestPath := machinesOption
	if gitURL != "" {
		clusterManifestPath, machinesManifestPath, closer, err = syncRepo(gitURL, gitBranch, gitDeployKeyPath, gitPath)
		log.WithFields(log.Fields{"cluster": clusterManifestPath, "machines": machinesManifestPath}).Debug("manifests")
	}

	return clusterManifestPath, machinesManifestPath, closer, err
}

func syncRepo(url, branch, deployKeyPath, relativeRoot string) (string, string, func(), error) {
	srcDir, err := ioutil.TempDir("", "wkp")
	if err != nil {
		return "", "", nil, errors.Wrap(err, "failed to create temp dir")
	}
	closer := func() {
		os.RemoveAll(srcDir)
	}
	lCtx := log.WithField("repo", url)
	opt, err := cloneOptions(url, deployKeyPath, branch)
	if err != nil {
		return "", "", nil, err
	}
	r, err := gogit.PlainClone(srcDir, false, &opt)

	if err != nil {
		closer()
		return "", "", nil, errors.Wrapf(err, "failed to clone repository: %s", url)
	}
	lCtx.WithField("config", r.Config).Debug("cloned")

	rootDir := filepath.Join(srcDir, relativeRoot)
	files, err := ioutil.ReadDir(rootDir)
	if err != nil {
		closer()
		return "", "", nil, errors.Wrapf(err, "failed to read directory %s", rootDir)
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
	return cYaml, mYaml, closer, nil
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
