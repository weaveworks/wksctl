package specs

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const clusterMissingClusterDefinition = `
apiVersion: "cluster.weave.works/v1alpha3"
kind: "ExistingInfraCluster"
metadata:
  name: example
spec:
  user: "vagrant"
`

const clusterMissingExistingInfraClusterDefinition = `
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
`

func mergeObjects(a string, b string) string {
	return fmt.Sprintf("%s---%s", a, b)
}

func parseConfig(s string) (err error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(s)
	if err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	_, _, err = ParseClusterManifest(f.Name())
	return err
}

func TestParseCluster(t *testing.T) {
	assert.NoError(t, parseConfig(mergeObjects(clusterMissingExistingInfraClusterDefinition, clusterMissingClusterDefinition)))

	// Verify that the objects individually don't result in a successful parse
	assert.Error(t, parseConfig(clusterMissingClusterDefinition))
	assert.Error(t, parseConfig(clusterMissingExistingInfraClusterDefinition))
}
