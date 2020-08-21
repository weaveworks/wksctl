package specs

import (
	"fmt"
	"io"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	capeiv1alpha3 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeispecs "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/specs"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/scheme"
	"github.com/weaveworks/wksctl/pkg/utilities"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	apierrors "sigs.k8s.io/cluster-api/errors"
)

// Utilities for managing cluster and machine specs.
// Common code for commands that need to run ssh commands on master cluster nodes.

type Specs struct {
	Cluster      *clusterv1.Cluster
	ClusterSpec  *capeiv1alpha3.ClusterSpec
	MasterSpec   *capeiv1alpha3.MachineSpec
	machineCount int
	masterCount  int
}

// Get a "Specs" object that can create an SSHClient (and retrieve useful nested fields)
func NewFromPaths(clusterManifestPath, machinesManifestPath string) *Specs {
	cluster, eic, machines, bml, err := parseManifests(clusterManifestPath, machinesManifestPath)
	if err != nil {
		log.Fatal("Error parsing manifest: ", err)
	}
	return New(cluster, eic, machines, bml)
}

// Get a "Specs" object that can create an SSHClient (and retrieve useful nested fields)
func New(cluster *clusterv1.Cluster, eic *capeiv1alpha3.ExistingInfraCluster, machines []*clusterv1.Machine, bl []*capeiv1alpha3.ExistingInfraMachine) *Specs {
	_, master := machine.FirstMaster(machines, bl)
	if master == nil {
		log.Fatal("No master provided in manifest.")
	}
	masterCount := 0
	for _, m := range machines {
		if m.Labels["set"] == "master" {
			masterCount++
		}
	}
	return &Specs{
		Cluster:     cluster,
		ClusterSpec: &eic.Spec,
		MasterSpec:  &master.Spec,

		machineCount: len(machines),
		masterCount:  masterCount,
	}
}

func parseManifests(clusterManifestPath, machinesManifestPath string) (*clusterv1.Cluster, *capeiv1alpha3.ExistingInfraCluster, []*clusterv1.Machine, []*capeiv1alpha3.ExistingInfraMachine, error) {
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

	errorsHandler := func(machines []*clusterv1.Machine, bl []*capeiv1alpha3.ExistingInfraMachine, errors field.ErrorList) ([]*clusterv1.Machine, []*capeiv1alpha3.ExistingInfraMachine, error) {
		if len(errors) > 0 {
			utilities.PrintErrors(errors)
			return nil, nil, apierrors.InvalidMachineConfiguration(
				"%s failed validation, use --skip-validation to force the operation", machinesManifestPath)
		}
		return machines, bl, nil
	}

	machines, bl, err := machine.ParseAndDefaultAndValidate(machinesManifestPath, errorsHandler)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return cluster, eic, machines, bl, nil
}

// ParseCluster converts the manifest file into a Cluster
func ParseCluster(rc io.ReadCloser) (cluster *clusterv1.Cluster, eic *capeiv1alpha3.ExistingInfraCluster, err error) {
	// Read from the ReadCloser YAML document-by-document
	fr := serializer.NewYAMLFrameReader(rc)

	// Decode all objects in the FrameReader
	objs, err := scheme.Serializer.Decoder().DecodeAll(fr)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to parse cluster manifest")
	}

	// Loop through the untyped objects we got and add them to the specific lists
	for _, obj := range objs {
		switch typed := obj.(type) {
		case *clusterv1.Cluster:
			cluster = typed
		case *capeiv1alpha3.ExistingInfraCluster:
			eic = typed
		default:
			return nil, nil, fmt.Errorf("unexpected type %T", obj)
		}
	}

	if cluster == nil {
		return nil, nil, errors.New("parsed cluster manifest lacks Cluster definition")
	}

	if eic == nil {
		return nil, nil, errors.New("parsed cluster manifest lacks ExistingInfraCluster definition")
	}

	return
}

func ParseClusterManifest(file string) (*clusterv1.Cluster, *capeiv1alpha3.ExistingInfraCluster, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	return ParseCluster(f)
}

// Getters for nested fields needed externally
func (s *Specs) GetClusterName() string {
	return s.Cluster.ObjectMeta.Name
}

func (s *Specs) GetMasterPublicAddress() string {
	return s.MasterSpec.Public.Address
}

func (s *Specs) GetMasterPrivateAddress() string {
	return s.MasterSpec.Private.Address
}

func (s *Specs) GetCloudProvider() string {
	return s.ClusterSpec.CloudProvider
}

func (s *Specs) GetKubeletArguments() map[string]string {
	return capeispecs.TranslateServerArgumentsToStringMap(s.ClusterSpec.KubeletArguments)
}

func (s *Specs) GetMachineCount() int {
	return s.machineCount
}

func (s *Specs) GetMasterCount() int {
	return s.masterCount
}
