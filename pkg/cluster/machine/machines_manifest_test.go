package machine_test

import (
	"fmt"

	"io/ioutil"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
)

const manifestWithGenerateNameFields = `
  apiVersion: cluster.x-k8s.io/v1alpha3
  kind: Machine
  metadata:
    generateName: master-
---
  apiVersion: cluster.x-k8s.io/v1alpha3
  kind: Machine
  metadata:
    generateName: master-
---
  apiVersion: cluster.x-k8s.io/v1alpha3
  kind: Machine
  metadata:
    generateName: node-
---
  apiVersion: cluster.x-k8s.io/v1alpha3
  kind: Machine
  metadata:
    generateName: node-
`

func TestUpdateWithGeneratedNamesWithGenerateNameFieldsShouldGenerateThese(t *testing.T) {
	r := ioutil.NopCloser(strings.NewReader(manifestWithGenerateNameFields))
	updatedManifest, err := machine.UpdateWithGeneratedNames(r)
	assert.NoError(t, err)
	assert.NotEqual(t, manifestWithGenerateNameFields, updatedManifest, "processing a manifest with generateName fields should modify it")
	assert.NotContains(t, updatedManifest, "generateName:")
	assert.Contains(t, updatedManifest, "name:")
	for _, line := range strings.Split(updatedManifest, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "name:") {
			assert.Regexp(t, regexp.MustCompile("^\\s+name: (master|node)-[0-9A-Za-z]{5}-[0-9A-Za-z]{5}$"), line)
		}
	}
	fmt.Print(updatedManifest)
	r = ioutil.NopCloser(strings.NewReader(updatedManifest))
	updatedManifest2, err := machine.UpdateWithGeneratedNames(r)
	assert.NoError(t, err)
	assert.Equal(t, updatedManifest, updatedManifest2, "processing the same manifest twice shouldn't modify it")
}

const manifestWithCustomNameFields = `
---
  apiVersion: cluster.x-k8s.io/v1alpha3
  kind: Machine
  metadata:
    generateName: master-
---
  apiVersion: cluster.x-k8s.io/v1alpha3
  kind: Machine
  metadata:
    name: seed-12345
---
  apiVersion: cluster.x-k8s.io/v1alpha3
  kind: Machine
  metadata:
    generateName: node-
---
  apiVersion: cluster.x-k8s.io/v1alpha3
  kind: Machine
  metadata:
    name: very-custom-worker-node
`

func TestUpdateWithGeneratedNamesWithCustomNameAndGenerateNameFieldsShouldOnlyChangeTheGenerateNameFields(t *testing.T) {
	r := ioutil.NopCloser(strings.NewReader(manifestWithCustomNameFields))
	updatedManifest, err := machine.UpdateWithGeneratedNames(r)
	assert.NoError(t, err)
	assert.NotEqual(t, manifestWithCustomNameFields, updatedManifest, "processing a manifest with generateName fields should modify it")
	assert.NotContains(t, updatedManifest, "generateName:")
	assert.Contains(t, updatedManifest, "name:")
	assert.Contains(t, updatedManifest, "name: seed-12345")
	assert.Contains(t, updatedManifest, "name: very-custom-worker-node")
	for _, line := range strings.Split(updatedManifest, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "name: master-") {
			assert.Regexp(t, regexp.MustCompile("^\\s+name: master-[0-9A-Za-z]{5}-[0-9A-Za-z]{5}$"), line)
		} else if strings.HasPrefix(strings.TrimSpace(line), "name: node-") {
			assert.Regexp(t, regexp.MustCompile("^\\s+name: node-[0-9A-Za-z]{5}-[0-9A-Za-z]{5}$"), line)
		}
	}
	r = ioutil.NopCloser(strings.NewReader(updatedManifest))
	updatedManifest2, err := machine.UpdateWithGeneratedNames(r)
	assert.NoError(t, err)
	assert.Equal(t, updatedManifest, updatedManifest2, "processing the same manifest twice shouldn't modify it")
}
