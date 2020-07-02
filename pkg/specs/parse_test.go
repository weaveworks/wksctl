package specs

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const clusterMissingClusterDefinition = `
apiVersion: "cluster.weave.works/v1alpha3"
kind: "BareMetalCluster"
metadata:
  name: example
spec:
  user: "vagrant"
`

const clusterMissingBareMetalClusterDefinition = `
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
    kind: BareMetalCluster
    name: example
`

func mergeObjects(a string, b string) string {
	return fmt.Sprintf("%s---%s", a, b)
}

func parseConfig(s string) (err error) {
	r := ioutil.NopCloser(strings.NewReader(s))
	_, _, err = ParseCluster(r)
	return
}

func TestParseCluster(t *testing.T) {
	assert.NoError(t, parseConfig(mergeObjects(clusterMissingBareMetalClusterDefinition, clusterMissingClusterDefinition)))

	// Verify that the objects individually don't result in a successful parse
	assert.Error(t, parseConfig(clusterMissingClusterDefinition))
	assert.Error(t, parseConfig(clusterMissingBareMetalClusterDefinition))
}
