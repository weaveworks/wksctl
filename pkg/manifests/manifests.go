package manifests

import (
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	xcryptossh "golang.org/x/crypto/ssh"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	gogitssh "gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

func Get(clusterOption, machinesOption, gitURL, gitBranch, gitDeployKeyPath, gitPath string) (string, string, func()) {
	clusterManifestPath := clusterOption
	machinesManifestPath := machinesOption
	var closer = func() {
	}
	if gitURL != "" {
		clusterManifestPath, machinesManifestPath, closer = syncRepo(gitURL, gitBranch, gitDeployKeyPath, gitPath)
		log.WithFields(log.Fields{"cluster": clusterManifestPath, "machines": machinesManifestPath}).Debug("manifests")
	}
	return clusterManifestPath, machinesManifestPath, closer
}

func syncRepo(url, branch, deployKeyPath, relativeRoot string) (string, string, func()) {
	srcDir, err := ioutil.TempDir("", "wkp")
	if err != nil {
		log.Fatalf("Failed to create temp dir - %v", err)
	}
	closer := func() {
		os.RemoveAll(srcDir)
	}
	lCtx := log.WithField("repo", url)
	opt := cloneOptions(url, deployKeyPath, branch)
	r, err := gogit.PlainClone(srcDir, false, &opt)

	if err != nil {
		closer()
		lCtx.Fatal(err)
	}
	lCtx.WithField("config", r.Config).Debug("cloned")

	rootDir := filepath.Join(srcDir, relativeRoot)
	files, err := ioutil.ReadDir(rootDir)
	if err != nil {
		closer()
		lCtx.Fatalf("Failed to read directory %s - %v", rootDir, err)
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
	return cYaml, mYaml, closer
}

func cloneOptions(url, deployKeyPath, branch string) gogit.CloneOptions {
	co := gogit.CloneOptions{
		URL: url,
	}
	if branch != "" {
		co.SingleBranch = true
		co.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}
	if deployKeyPath == "" {
		return co
	}
	pem, err := ioutil.ReadFile(deployKeyPath)
	if err != nil {
		log.Fatalf("Failed to read deploy key %s - %v", deployKeyPath, err)
	}
	signer, err := xcryptossh.ParsePrivateKey(pem)
	if err != nil {
		log.Fatalf("Failed to parse private key - %v", err)
	}

	aith := &gogitssh.PublicKeys{User: "git", Signer: signer}
	co.Auth = aith

	return co
}
