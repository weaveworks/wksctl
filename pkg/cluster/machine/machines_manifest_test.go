package machine_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
)

const manifestWithGenerateNameFields = `apiVersion: v1
kind: List
items:
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    generateName: master-
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    generateName: master-
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    generateName: node-
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    generateName: node-
`

// disabled: not implemented for v1alpha3
func xTestUpdateWithGeneratedNamesWithGenerateNameFieldsShouldGenerateThese(t *testing.T) {
	updatedManifest, err := machine.UpdateWithGeneratedNames(manifestWithGenerateNameFields)
	assert.NoError(t, err)
	assert.NotEqual(t, manifestWithGenerateNameFields, updatedManifest, "processing a manifest with generateName fields should modify it")
	assert.NotContains(t, updatedManifest, "generateName:")
	assert.Contains(t, updatedManifest, "name:")
	for _, line := range strings.Split(updatedManifest, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "name:") {
			assert.Regexp(t, regexp.MustCompile("^\\s+name: (master|node)-[0-9A-Za-z]{5}-[0-9A-Za-z]{5}$"), line)
		}
	}
	updatedManifest2, err := machine.UpdateWithGeneratedNames(updatedManifest)
	assert.NoError(t, err)
	assert.Equal(t, updatedManifest, updatedManifest2, "processing the same manifest twice shouldn't modify it")
}

const manifestWithCustomNameFields = `apiVersion: v1
kind: List
items:
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    generateName: master-
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    name: seed-12345
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    generateName: node-
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    name: very-custom-worker-node
`

// disabled: not implemented for v1alpha3
func xTestUpdateWithGeneratedNamesWithCustomNameAndGenerateNameFieldsShouldOnlyChangeTheGenerateNameFields(t *testing.T) {
	updatedManifest, err := machine.UpdateWithGeneratedNames(manifestWithCustomNameFields)
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
	updatedManifest2, err := machine.UpdateWithGeneratedNames(updatedManifest)
	assert.NoError(t, err)
	assert.Equal(t, updatedManifest, updatedManifest2, "processing the same manifest twice shouldn't modify it")
}
