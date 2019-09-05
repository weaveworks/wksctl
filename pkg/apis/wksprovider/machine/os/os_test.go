package os

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
