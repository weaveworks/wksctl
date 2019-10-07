package specs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const clusterMinimumValid = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
`

const clusterHasSSHKey = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      sshKeyPath: "/etc/hosts"
      user: "vagrant"
`

const clusterNonDefaultServiceDomain = `
apiVersion: "cluster.k8s.io/v1alpha1"
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
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
`

const clusterBadCIDRBlocks = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12", "10.100.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/72"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
`

const clusterServicePodNetworksOverlap = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["10.96.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
`

const ClusterAuthenticationBadCacheTTL = `items:
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
      authenticationWebhook:
        cacheTTL: foo
        server:
          url: http://127.0.0.1:5000/authenticate
`

const ClusterAuthenticationBadServerURL = `items:
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
      authenticationWebhook:
        cacheTTL: 2m0s
        server:
          url: file:///127.0.0.1:5000/authenticate
`

const ClusterAuthenticationNoClientCert = `items:
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
      authenticationWebhook:
        cacheTTL: 2m0s
        client:
          keyData: SGVsbG8sIFdvcmxkIQo=
        server:
          url: https://127.0.0.1:5000/authenticate
          certificateAuthorityData: SGVsbG8sIFdvcmxkIQo=
`

const ClusterAuthorizationNoServerCert = `items:
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
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
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
      addons:
      - name: foo
`

const ClusterAddonBadParameters = `
apiVersion: "cluster.k8s.io/v1alpha1"
kind: Cluster
metadata:
  name: example
spec:
  clusterNetwork:
    services:
      cidrBlocks: ["10.96.0.0/12"]
    pods:
      cidrBlocks: ["192.168.0.0/16"]
  providerSpec:
    value:
      apiVersion: "baremetalproviderspec/v1alpha1"
      kind: "BareMetalClusterProviderSpec"
      user: "vagrant"
      addons:
      - name: kube-kerberos
        params:
          keytab: /foo
`

func clusterFromString(t *testing.T, s string) *clusterv1.Cluster {
	r := strings.NewReader(s)
	cluster, err := parseCluster(r)
	assert.NoError(t, err)
	return cluster
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
		cluster := clusterFromString(t, test.input)
		populateCluster(cluster)
		errors := validateCluster(cluster, "/tmp/test.yaml")
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
	cluster := clusterFromString(t, clusterMinimumValid)
	populateCluster(cluster)
	assert.Equal(t, "cluster.local", cluster.Spec.ClusterNetwork.ServiceDomain)
}
