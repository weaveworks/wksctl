package machine_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
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
	assert.Equal(t, master, *machine.FirstMaster([]*clusterv1.Machine{
		&worker,
		&master,
	}))
	assert.Nil(t, machine.FirstMaster([]*clusterv1.Machine{
		&worker,
	}))
	assert.Nil(t, machine.FirstMaster([]*clusterv1.Machine{}))
}

func TestFirstMasterInArray(t *testing.T) {
	assert.Equal(t, &master, machine.FirstMasterInArray([]clusterv1.Machine{
		worker,
		master,
	}))
	assert.Nil(t, machine.FirstMasterInArray([]clusterv1.Machine{
		worker,
	}))
	assert.Nil(t, machine.FirstMasterInArray([]clusterv1.Machine{}))
}

const machinesValid = `items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: master-
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.101"
    versions:
      kubelet: "1.14.12"
      controlPlane: "1.14.12"
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: node-
    labels:
      set: node
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.102"
        authenticationWebhook:
          cacheTTL: 2m0s
          server:
            url: http://127.0.0.1:5000/authenticate
    versions:
      kubelet: "1.14.12"
`

const machinesValidWithOnlyKubeletVersion = `items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: master-
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.101"
    versions:
      kubelet: "1.14.12"
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: node-
    labels:
      set: node
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.102"
        authenticationWebhook:
          cacheTTL: 2m0s
          server:
            url: http://127.0.0.1:5000/authenticate
    versions:
      kubelet: "1.14.12"
`

// A machine doesn't have a matching Kubelet version.
const machinesInconsistentKubeletVersion = `items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: master-
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.101"
    versions:
      kubelet: "1.14.4"
      controlPlane: "1.14.4"
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: node-
    labels:
      set: node
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.102"
    versions:
      kubelet: "1.14.3"
`

// A machine doesn't have a matching controlPlane version.
const machinesInconsistentControlPlaneVersion = `items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: master-
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.101"
    versions:
      kubelet: "1.14.4"
      controlPlane: "1.14.3"
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: node-
    labels:
      set: node
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.102"
    versions:
      kubelet: "1.14.4"
`

// Unsupported Kubernetes version.
const machinesUnsupportedKubernetesVersion = `items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: master-
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.101"
    versions:
      kubelet: "1.13.2"
      controlPlane: "1.13.2"
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: node-
    labels:
      set: node
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.102"
    versions:
      kubelet: "1.13.2"
`

const machinesNoGodNoMaster = `items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: node-
    labels:
      set: node
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.101"
    versions:
      kubelet: "1.14.12"
`

func machinesFromString(t *testing.T, s string) []*clusterv1.Machine {
	r := strings.NewReader(s)
	machines, err := machine.Parse(r)
	assert.NoError(t, err)
	return machines
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
		{machinesValidWithOnlyKubeletVersion, []string{}},
		{machinesInconsistentKubeletVersion, []string{
			"machines[1].spec.versions.kubelet",
		}},
		{machinesInconsistentControlPlaneVersion, []string{
			"machines[0].spec.versions.controlPlane",
		}},
		{machinesUnsupportedKubernetesVersion, []string{
			"machines[0].spec.versions.kubelet",
		}},
		{machinesNoGodNoMaster, []string{
			"spec.versions.controlPlane",
		}},
	}

	for _, test := range tests {
		machines := machinesFromString(t, test.input)
		errors := machine.Validate(machines)
		assert.Equal(t, len(test.errors), len(errors))
		assert.Equal(t, test.errors, fieldsInError(errors))

		if t.Failed() {
			t.Log(errors)
			t.FailNow()
		}
	}
}

const machinesWithoutVersions = `items:
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: master-
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.101"
- apiVersion: "cluster.k8s.io/v1alpha1"
  kind: Machine
  metadata:
    generateName: node-
    labels:
      set: node
  spec:
    providerSpec:
      value:
        apiVersion: "baremetalproviderspec/v1alpha1"
        kind: "BareMetalMachineProviderSpec"
        address: "172.17.8.102"
        authenticationWebhook:
          cacheTTL: 2m0s
          server:
            url: http://127.0.0.1:5000/authenticate
`

// Ensure we populate the Kubernetes version if not provided.
func TestPopulateVersions(t *testing.T) {
	machinesWithoutVersions := machinesFromString(t, machinesWithoutVersions)
	machine.Populate(machinesWithoutVersions)

	for _, m := range machinesWithoutVersions {
		v := &m.Spec.Versions
		assert.Equal(t, kubernetes.DefaultVersion, v.Kubelet)
		if machine.IsMaster(m) {
			assert.Equal(t, kubernetes.DefaultVersion, v.ControlPlane)
		}
	}
}

func TestGetKubernetesVersionFromMasterInDefaultsVersionWhenMachinesDoNotSpecifyAny(t *testing.T) {
	version, err := machine.GetKubernetesVersionFromMasterIn(machinesFromString(t, machinesWithoutVersions))
	assert.NoError(t, err)
	assert.Equal(t, kubernetes.DefaultVersion, version)
}

func TestGetKubernetesVersionFromMasterInGetsControlPlaneVersion(t *testing.T) {
	version, err := machine.GetKubernetesVersionFromMasterIn(machinesFromString(t, machinesValid))
	assert.NoError(t, err)
	assert.Equal(t, "1.14.12", version)
}

func TestGetKubernetesVersionFallsbackToKubeletVersionForWorkerNodes(t *testing.T) {
	machines := machinesFromString(t, machinesInconsistentControlPlaneVersion)
	version := machine.GetKubernetesVersion(machines[0])
	assert.Equal(t, "1.14.3", version)
	version = machine.GetKubernetesVersion(machines[1])
	assert.Equal(t, "1.14.4", version)
}

func TestGetKubernetesVersionDefaultsVersionWhenMachinesDoNotSpecifyAny(t *testing.T) {
	version := machine.GetKubernetesVersion(machinesFromString(t, machinesWithoutVersions)[0])
	assert.Equal(t, kubernetes.DefaultVersion, version)
}
