package specs

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	capeiv1alpha3 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

const clusterMinimumValid = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  infrastructureRef:
    kind: ExistingInfraCluster
    name: example
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
`

const clusterHasSSHKey = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  infrastructureRef:
    kind: ExistingInfraCluster
    name: example
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  sshKeyPath: "/etc/hosts"
  user: "vagrant"
`

const clusterNonDefaultServiceDomain = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
    serviceDomain: "foo.bar"
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
`

const clusterBadCIDRBlocks = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12", "10.100.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/72"]
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
`

const clusterServicePodNetworksOverlap = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["10.96.0.0/16"]
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
`

//nolint:unused
const ClusterAuthenticationBadCacheTTL = `items:
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
  authenticationWebhook:
    cacheTTL: foo
    server:
      url: http://127.0.0.1:5000/authenticate
`

//nolint:unused
const ClusterAuthenticationBadServerURL = `items:
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
  authenticationWebhook:
	cacheTTL: 2m0s
	server:
	  url: file:///127.0.0.1:5000/authenticate
`

//nolint:unused
const ClusterAuthenticationNoClientCert = `items:
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
  authenticationWebhook:
	cacheTTL: 2m0s
	client:
	  keyData: SGVsbG8sIFdvcmxkIQo=
	server:
	  url: https://127.0.0.1:5000/authenticate
	  certificateAuthorityData: SGVsbG8sIFdvcmxkIQo=
`

//nolint:unused
const ClusterAuthorizationNoServerCert = `items:
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
  authorizationWebhook:
	cacheAuthorizedTTL: 5m0s
	cacheUnauthorizedTTL: 30s
	client:
	  keyData: SGVsbG8sIFdvcmxkIQo=
	  certificateData: SGVsbG8sIFdvcmxkIQo=
	server:
	  url: https://127.0.0.1:5000/authenticate
`

const ClusterAddonBadName = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
  addons:
  - name: foo
`

//nolint:unused
const ClusterAddonBadParameters = `
apiVersion: "cluster.x-k8s.io/v1alpha3"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
---
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
  addons:
  - name: kube-kerberos
	params:
	  keytab: /foo
`

func clusterFromString(t *testing.T, s string) (*clusterv1.Cluster, *capeiv1alpha3.ExistingInfraCluster) {
	r := ioutil.NopCloser(strings.NewReader(s))
	cluster, eic, err := ParseCluster(r)
	assert.NoError(t, err)
	return cluster, eic
}

// Gather the list of fields paths that didn't pass validation.
func fieldsInError(errors field.ErrorList) []string {
	fields := []string{}
	for _, err := range errors {
		fields = append(fields, err.Field)
	}
	return fields
}

func TestValidateCluster(t *testing.T) {
	tests := []struct {
		input  string
		errors []string
	}{
		{clusterMinimumValid, []string{}},
		{clusterHasSSHKey, []string{
			"cluster.spec.providerSpec.value.sshKeyPath",
		}},
		{clusterNonDefaultServiceDomain, []string{
			"cluster.spec.clusterNetwork.serviceDomain",
		}},
		{clusterBadCIDRBlocks, []string{
			"cluster.spec.clusterNetwork.services.cidrBlocks",
			"cluster.spec.clusterNetwork.pods.cidrBlocks",
		}},
		{clusterServicePodNetworksOverlap, []string{
			"cluster.spec.clusterNetwork.services.cidrBlocks",
		}},
		{ClusterAddonBadName, []string{
			"cluster.spec.providerSpec.value.addons[0].foo",
		}},
	}

	for _, test := range tests {
		cluster, eic := clusterFromString(t, test.input)
		populateCluster(cluster)
		errors := validateCluster(cluster, eic, "/tmp/test.yaml")
		assert.Equal(t, len(test.errors), len(errors))
		assert.Equal(t, test.errors, fieldsInError(errors))

		if t.Failed() {
			t.Log(errors)
			t.FailNow()
		}
	}
}

func TestValidCIDR(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"10.96.0.0/12", true},
		{"10.96.0.1/12", false},
	}

	for _, test := range tests {
		_, err := isValidCIDR(test.input)
		if test.valid {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestDefaultClusterValues(t *testing.T) {
	cluster, _ := clusterFromString(t, clusterMinimumValid)
	populateCluster(cluster)
	assert.Equal(t, "cluster.local", cluster.Spec.ClusterNetwork.ServiceDomain)
}
