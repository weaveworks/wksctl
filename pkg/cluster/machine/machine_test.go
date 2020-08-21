package machine_test

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	capeiv1alpha3 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeimachine "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/cluster/machine"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

var master = clusterv1.Machine{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			"set": "master",
		},
	},
}

var worker = clusterv1.Machine{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			"set": "node",
		},
	},
}

func TestIsMaster(t *testing.T) {
	assert.True(t, machine.IsMaster(&master))
	assert.False(t, machine.IsMaster(&worker))
}

func TestIsNode(t *testing.T) {
	assert.False(t, machine.IsNode(&master))
	assert.True(t, machine.IsNode(&worker))
}

func TestFirstMasterInPointersArray(t *testing.T) {
	bl := []*capeiv1alpha3.ExistingInfraMachine{nil, nil}
	v1, _ := machine.FirstMaster([]*clusterv1.Machine{
		&worker,
		&master,
	}, bl)
	assert.Equal(t, &master, v1)
	v2, _ := machine.FirstMaster([]*clusterv1.Machine{
		&worker,
	}, bl)
	assert.Nil(t, v2)
	v3, _ := machine.FirstMaster([]*clusterv1.Machine{}, bl)
	assert.Nil(t, v3)
}

const machinesValid = `
  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: master-0
    labels:
      set: master
  spec:
    infrastructureRef:
        kind: ExistingInfraMachine
        name: master-0
    version: "1.16.2"
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: master-0
  spec:
    private:
      address: "172.17.8.101"
---
  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: node-0
    labels:
      set: node
  spec:
    infrastructureRef:
        kind: ExistingInfraMachine
        name: node-0
    version: "1.16.2"
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: node-0
  spec:
    private:
      address: "172.17.8.102"
`

// A machine doesn't have a matching Kubernetes version.
const machinesInconsistentKubeVersion = `
  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: master-0
    labels:
      set: master
  spec:
    infrastructureRef:
        kind: ExistingInfraMachine
        name: master-0
    version: "1.16.4"
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: master-0
  spec:
    private:
      address: "172.17.8.101"
---
  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: node-0
    labels:
      set: node
  spec:
    infrastructureRef:
        kind: ExistingInfraMachine
        name: node-0
    version: "1.16.3"
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: node-0
  spec:
    private:
      address: "172.17.8.102"
`

// Unsupported Kubernetes version.
const machinesUnsupportedKubernetesVersion = `  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: master-0
    labels:
      set: master
  spec:
    infrastructureRef:
        kind: ExistingInfraMachine
        name: master-0
    version: "1.13.2"
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: master-0
  spec:
    private:
      address: "172.17.8.101"
---
  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: node-0
    labels:
      set: node
  spec:
    infrastructureRef:
        kind: ExistingInfraMachine
        name: node-0
    version: "1.13.2"
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: node-0
  spec:
    private:
      address: "172.17.8.102"
`

const machinesNoGodNoMaster = `
  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: node-0
    labels:
      set: node
  spec:
    infrastructureRef:
        kind: ExistingInfraMachine
        name: node-0
    version: "1.16.2"
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: node-0
  spec:
    private:
      address: "172.17.8.102"
`

func machinesFromString(t *testing.T, s string) ([]*clusterv1.Machine, []*capeiv1alpha3.ExistingInfraMachine) {
	r := ioutil.NopCloser(strings.NewReader(s))
	machines, bml, err := machine.Parse(r)
	assert.NoError(t, err)
	return machines, bml
}

// Gather the list of fields paths that didn't pass validation.
func fieldsInError(errors field.ErrorList) []string {
	fields := []string{}
	for _, err := range errors {
		fields = append(fields, err.Field)
	}
	return fields
}

func TestValidateMachines(t *testing.T) {
	tests := []struct {
		input  string
		errors []string
	}{
		{machinesValid, []string{}},
		{machinesInconsistentKubeVersion, []string{
			"machines[1].spec.version",
		}},
		{machinesUnsupportedKubernetesVersion, []string{
			"machines[0].spec.version",
		}},
		{machinesNoGodNoMaster, []string{
			"metadata.labels.set",
		}},
	}

	for i, test := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			machines, bl := machinesFromString(t, test.input)
			errors := machine.Validate(machines, bl)
			assert.Equal(t, len(test.errors), len(errors))
			assert.Equal(t, test.errors, fieldsInError(errors))

			if t.Failed() {
				t.Log(errors)
				t.FailNow()
			}
		})
	}
}

const machinesWithoutVersions = `
  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: master-0
    labels:
      set: master
  spec:
    infrastructureRef:
      kind: ExistingInfraMachine
      name: master-0
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: master-0
  spec:
    private:
      address: "172.17.8.101"
---
  apiVersion: "cluster.x-k8s.io/v1alpha3"
  kind: Machine
  metadata:
    name: node-0
    labels:
      set: node
  spec:
    infrastructureRef:
        kind: ExistingInfraMachine
        name: node-0
---
  apiVersion: "cluster.weave.works/v1alpha3"
  kind: "ExistingInfraMachine"
  metadata:
    name: node-0
  spec:
    private:
      address: "172.17.8.102"
`

// Ensure we populate the Kubernetes version if not provided.
func TestPopulateVersions(t *testing.T) {
	machinesWithoutVersions, _ := machinesFromString(t, machinesWithoutVersions)
	machine.Populate(machinesWithoutVersions)

	for _, m := range machinesWithoutVersions {
		v := *m.Spec.Version
		assert.Equal(t, kubernetes.DefaultVersion, v)
	}
}

func TestGetKubernetesVersionFromMasterInDefaultsVersionWhenMachinesDoNotSpecifyAny(t *testing.T) {
	version, namespace, err := machine.GetKubernetesVersionFromMasterIn(machinesFromString(t, machinesWithoutVersions))
	assert.NoError(t, err)
	assert.Equal(t, kubernetes.DefaultVersion, version)
	assert.Equal(t, manifest.DefaultNamespace, namespace)
}

func TestGetKubernetesVersionFromMasterInGetsControlPlaneVersion(t *testing.T) {
	version, _, err := machine.GetKubernetesVersionFromMasterIn(machinesFromString(t, machinesValid))
	assert.NoError(t, err)
	assert.Equal(t, "1.16.2", version)
}

func TestGetKubernetesVersionDefaultsVersionWhenMachinesDoNotSpecifyAny(t *testing.T) {
	machines, _ := machinesFromString(t, machinesWithoutVersions)
	version := capeimachine.GetKubernetesVersion(machines[0])
	assert.Equal(t, kubernetes.DefaultVersion, version)
}
