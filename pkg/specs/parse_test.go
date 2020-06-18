package specs

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	byobv1 "github.com/weaveworks/wksctl/pkg/byob/v1alpha3"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

const clusterMissingClusterDefinition = `
apiVersion: "cluster.weave.works/v1alpha3"
kind: "BYOBCluster"
metadata:
 name: example
spec:
 user: "vagrant"
`

const clusterMissingBYOBClusterDefinition = `
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
     kind: BYOBCluster
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
	assert.NoError(t, clusterv1.AddToScheme(scheme.Scheme))
	assert.NoError(t, byobv1.AddToScheme(scheme.Scheme))
	assert.NoError(t, parseConfig(mergeObjects(clusterMissingBYOBClusterDefinition, clusterMissingClusterDefinition)))

	// Verify that the objects individually don't result in a successful parse
	assert.Error(t, parseConfig(clusterMissingClusterDefinition))
	assert.Error(t, parseConfig(clusterMissingBYOBClusterDefinition))
}
