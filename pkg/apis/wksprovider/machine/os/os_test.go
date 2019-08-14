package os

import (
	"encoding/base64"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
	"github.com/weaveworks/wksctl/pkg/utilities/object"
	"github.com/weaveworks/wksctl/test/plan/testutils"
	v1beta2 "k8s.io/api/apps/v1beta2"

	"sigs.k8s.io/yaml"
)

func TestReplaceImage(t *testing.T) {
	var tests = []struct {
		yaml                  string
		newImage              string
		expInFileOutFileMatch bool
		expErr                bool
		msg                   string
	}{
		{"", "", true, false, "Expected files to match"},
		{`apiVersion: v1
kind: Secret`, "", true, false, "Exected files to match"},
		{`apiVersion: v1
kind: Deployment`, "newimage", false, true, "Expected err - no containers"},
		{`apiVersion: v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: foo
        image: fooimage`, "newimage", false, true, "Expected new file even though there isn't a Controller container"},
		{`apiVersion: v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: controller
        image: controllerimage`, "newimage", false, false, "Expected new file"},
		{`apiVersion: v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: controller
        image: controllerimage
      - name: foo
        image: controllerimage`, "newimage", false, false, "Expected new file"},
	}
	for _, test := range tests {
		in := []byte(test.yaml)
		out, err := updateControllerImage(in, test.newImage)
		if test.expErr {
			assert.NotNil(t, err, test.msg)
			assert.Empty(t, out, test.msg)
			continue
		}
		assert.Nil(t, err, test.msg)
		if test.expInFileOutFileMatch {
			assert.Equal(t, in, out, test.msg)
		} else {
			assert.NotEqual(t, in, out, test.msg)
		}
		if test.newImage == "" {
			continue
		}
		d := &v1beta2.Deployment{}
		yaml.Unmarshal(out, d)
		if len(d.Spec.Template.Spec.Containers) == 0 {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == "controller" {
				assert.Equal(t, test.newImage, c.Image, test.msg)
			} else {
				assert.NotEqual(t, test.newImage, c.Image, test.msg)
			}
		}
	}
}

func TestFlux(t *testing.T) {
	var gitURL = "git@github.com/testorg/repo1"
	var gitBranch = "my-unique-prod-branch"
	dk := "the deploy key"
	f, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	f.WriteString(dk)
	f.Close()
	var gitDeployKeyPath = f.Name()
	var tests = []struct {
		URL, branch, deployKeyPath, notExp, expManifestText, notExpManifestText, msg string
	}{
		{"", "", "", "flux", "", "", "expected plan without flux"},
		{gitURL, "", "", "", gitURL, "", "expected plan w/o branch or deploy key"},
		{gitURL, "", gitDeployKeyPath, "", "identity: " + base64.StdEncoding.EncodeToString([]byte(dk)), "", "expected flux yaml with deploy key"},
		{gitURL, "", "", "", "", "identity: " + base64.StdEncoding.EncodeToString([]byte(dk)), "expected flux yaml without deploy key"},
		{gitURL, gitBranch, "", "", "--git-branch=" + gitBranch, "", "expected flux yaml with branch"},
		{gitURL, gitBranch, "", "", "namespace: system", "", "expected to be in the system namespace"},
		{gitURL, gitBranch, "", "", "", "namespace: flux", "flux should not be the namespace"},
	}

	for _, test := range tests {

		b := plan.NewBuilder()
		o := &OS{
			Name:    centOS,
			runner:  &testutils.MockRunner{Output: "ID=\"centos\"\nVERSION=\"7 (Core)\"\nVERSION_ID=\"7\"", Err: nil},
			PkgType: resource.PkgTypeRPM,
		}
		applyClstrRsc := &resource.KubectlApply{ManifestPath: object.String("")}
		b.AddResource("kubectl:apply:cluster", applyClstrRsc)
		applyMachinesRsc := &resource.KubectlApply{ManifestPath: object.String("")}
		b.AddResource("kubectl:apply:machines", applyMachinesRsc)
		o.configureFlux(b, SeedNodeParams{GitData: GitParams{GitURL: test.URL, GitBranch: test.branch, GitDeployKeyPath: test.deployKeyPath},
			Namespace: "system"})
		p, err := b.Plan()
		assert.NoError(t, err)
		rjson := p.ToJSON()
		if test.URL == "" {
			assert.NotContains(t, rjson, test.notExp)
			continue
		}
		mani, err := p.State().GetObject("install:flux:flux-00")
		assert.NoError(t, err)
		mf, ok := mani["manifest"]
		assert.True(t, ok)
		m := string(mf.([]byte)[:])
		if test.expManifestText != "" {
			assert.Contains(t, m, test.expManifestText)
		} else {
			assert.NotContains(t, m, test.notExpManifestText)
		}
	}
}
