package specs

import (
	"os"

	log "github.com/sirupsen/logrus"
	existinginfra1 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeimachine "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/cluster/machine"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/utilities"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	apierrors "sigs.k8s.io/cluster-api/errors"
)

// Utilities for managing cluster and machine specs.
// Common code for commands that need to run ssh commands on master cluster nodes.

// Get a "capeispecs.Specs" object that can create an SSHClient (and retrieve useful nested fields)
func NewFromPaths(clusterManifestPath, machinesManifestPath string) *specs.Specs {
	cluster, eic, machines, bml, err := parseManifests(clusterManifestPath, machinesManifestPath)
	if err != nil {
		log.Fatal("Error parsing manifest: ", err)
	}
	return specs.New(cluster, eic, machines, bml)
}

func parseManifests(clusterManifestPath, machinesManifestPath string) (*clusterv1.Cluster, *existinginfra1.ExistingInfraCluster, []*clusterv1.Machine, []*existinginfra1.ExistingInfraMachine, error) {
	cluster, eic, err := ParseClusterManifest(clusterManifestPath)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	populateCluster(cluster)

	validationErrors := validateCluster(cluster, eic, clusterManifestPath)
	if len(validationErrors) > 0 {
		utilities.PrintErrors(validationErrors)
		return nil, nil, nil, nil, apierrors.InvalidMachineConfiguration(
			"%s failed validation, use --skip-validation to force the operation", clusterManifestPath)
	}

	errorsHandler := func(machines []*clusterv1.Machine, bl []*existinginfra1.ExistingInfraMachine, errors field.ErrorList) ([]*clusterv1.Machine, []*existinginfra1.ExistingInfraMachine, error) {
		if len(errors) > 0 {
			utilities.PrintErrors(errors)
			return nil, nil, apierrors.InvalidMachineConfiguration(
				"%s failed validation, use --skip-validation to force the operation", machinesManifestPath)
		}
		return machines, bl, nil
	}

	machines, bl, err := capeimachine.ParseAndDefaultAndValidate(machinesManifestPath, errorsHandler)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return cluster, eic, machines, bl, nil
}

func ParseClusterManifest(file string) (*clusterv1.Cluster, *existinginfra1.ExistingInfraCluster, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	return specs.ParseCluster(f)
}
