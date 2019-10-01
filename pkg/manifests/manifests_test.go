package manifests

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"
	"os"
	"path/filepath"

	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

func TestNoDeployKey(t *testing.T) {
	co, err := cloneOptions("foo", "", "")
	assert.NoError(t, err)
	assert.Equal(t, gogit.CloneOptions{URL: "foo"}, co)

}

func TestSSHDeployKey(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	defer os.Remove(f.Name())
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	assert.NoError(t, err)
	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(pk),
	})
	f.Write(keyPem)
	f.Close()
	co, err := cloneOptions("url", f.Name(), "")
	assert.NoError(t, err)
	assert.NotNil(t, co.Auth)
}

func TestBranchClone(t *testing.T) {
	co, err := cloneOptions("foo", "", "develop")
	assert.NoError(t, err)
	assert.Equal(t, gogit.CloneOptions{URL: "foo", SingleBranch: true, ReferenceName: plumbing.NewBranchReferenceName("develop")}, co)
}

func TestMachinesManifestPath(t *testing.T) {
	for _, tt := range []struct {
		name   string
		subdir string
	}{
		{"0 components", ""},
		{"0 components, trailing slash", "/"},
		{"1 component", "aa"},
		{"1 component, trailing slash", "aa/"},
		{"1 component, leading slash", "/aa"},
		{"3 components", "aa/bb/cc"},
		{"3 components, trailing slash", "aa/bb/cc/"},
		{"3 components, leading slash", "/aa/bb/cc"},
		{"3 components, trailing and leading slash", "/aa/bb/cc/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmpdir, err := ioutil.TempDir("", "wksctl-pkg-manifests-test")
			require.NoError(t, err)
			defer os.RemoveAll(tmpdir)

			dir := filepath.Join(tmpdir, tt.subdir)
			require.NoError(t, os.MkdirAll(dir, 0700))

			fp := filepath.Join(tmpdir, tt.subdir, "machines.yaml")
			file, err := os.Create(fp)
			require.NoError(t, err)
			file.Close()

			repo := ClusterAPIRepo{
				worktreePath: tmpdir,
				subdir:       tt.subdir,
			}
			gotPath, gotErr := repo.MachinesManifestPath()
			require.NoError(t, gotErr)
			assert.Equal(t, fp, gotPath)
		})
	}
}
