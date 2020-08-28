package profiles

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"

	log "github.com/sirupsen/logrus"
)

// Profile describes a profile addon to install on the cluster.
type Profile struct {
	Name     string            `json:"name"`
	Location string            `json:"location"`
	Params   map[string]string `json:"params,omitempty"`
}

// TODO: wrap this up similar to 'runners'
// or call kpt as a Go function
func kptExec(args ...string) error {
	cmd := exec.Command("kpt", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Debug("running command", cmd.String())
	return cmd.Run()
}

// Build returns the manifests for a profile
func (profile Profile) Build(imageRepository string) ([][]byte, error) {
	log.WithField("profile", profile.Name).Debug("building profile")

	tmpDir, err := ioutil.TempDir("", "wksctl-profile")
	if err != nil {
		return nil, err
	}
	kptDir := path.Join(tmpDir, profile.Name)

	if err := kptExec("pkg", "get", profile.Location, kptDir); err != nil {
		return nil, err
	}

	for name, value := range profile.Params {
		if err := kptExec("cfg", "set", kptDir, name, value); err != nil {
			return nil, err
		}
	}

	// TODO: update the imageRepository

	// Read in all manifest files
	retManifests := [][]byte{}
	files, err := ioutil.ReadDir(kptDir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.Name() == "Kptfile" {
			continue
		}
		content, err := ioutil.ReadFile(path.Join(kptDir, file.Name()))
		if err != nil {
			return nil, err
		}
		retManifests = append(retManifests, content)
	}
	log.WithField("profile", profile.Name).Debugf("returning %d manifests", len(retManifests))

	// Remove the generated manifest files.
	os.RemoveAll(tmpDir)

	return retManifests, nil
}
