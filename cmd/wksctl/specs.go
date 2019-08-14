package main

import (
	"io"
	"io/ioutil"
	"os"

	yaml "github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	baremetalspecv1 "github.com/weaveworks/wksctl/pkg/baremetalproviderspec/v1alpha1"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	apierrors "sigs.k8s.io/cluster-api/pkg/errors"
)

// Utilities for managing cluster and machine specs.
// Common code for commands that need to run ssh commands on master cluster nodes.

type specs struct {
	cluster     *clusterv1.Cluster
	clusterSpec *baremetalspecv1.BareMetalClusterProviderSpec
	masterSpec  *baremetalspecv1.BareMetalMachineProviderSpec
}

// Get a "specs" object that can create an SSHClient (and retrieve useful nested fields)
func getSpecs(clusterManifestPath, machinesManifestPath string) *specs {
	cluster, machines, err := parseManifests(clusterManifestPath, machinesManifestPath)
	if err != nil {
		log.Fatal("Error parsing manifest: ", err)
	}
	return getSpecsForClusterAndMachines(cluster, machines)
}

// Get a "specs" object that can create an SSHClient (and retrieve useful nested fields)
func getSpecsForClusterAndMachines(cluster *clusterv1.Cluster, machines []*clusterv1.Machine) *specs {
	master := machine.FirstMaster(machines)
	if master == nil {
		log.Fatal("No master provided in manifest.")
	}
	codec, err := baremetalspecv1.NewCodec()
	if err != nil {
		log.Fatal("Failed to create codec: ", err)
	}
	clusterSpec, err := codec.ClusterProviderFromProviderSpec(cluster.Spec.ProviderSpec)
	if err != nil {
		log.Fatal("Failed to parse cluster manifest: ", err)
	}
	masterSpec, err := codec.MachineProviderFromProviderSpec(master.Spec.ProviderSpec)
	if err != nil {
		log.Fatal("Failed to parse master: ", err)
	}
	return &specs{cluster, clusterSpec, masterSpec}
}

// Create an SSHClient to the master node referenced by the specs
func (s *specs) getSSHClient(verbose bool) (*ssh.Client, error) {
	var ip string
	var port uint16
	if s.masterSpec.Public.Address != "" {
		ip = s.masterSpec.Public.Address
		port = s.masterSpec.Public.Port
	} else {
		// Fall back to the address at the root
		ip = s.masterSpec.Address
		port = s.masterSpec.Port
	}
	return ssh.NewClient(ssh.ClientParams{
		User:           s.clusterSpec.User,
		Host:           ip,
		Port:           port,
		PrivateKeyPath: s.clusterSpec.SSHKeyPath,
		Verbose:        verbose,
	})
}

func parseManifests(clusterManifestPath, machinesManifestPath string) (*clusterv1.Cluster, []*clusterv1.Machine, error) {
	cluster, err := parseClusterManifest(clusterManifestPath)
	if err != nil {
		return nil, nil, err
	}
	populateCluster(cluster)

	validationErrors := validateCluster(cluster, clusterManifestPath)
	if len(validationErrors) > 0 {
		printValidationErrors(validationErrors)
		return nil, nil, apierrors.InvalidMachineConfiguration(
			"%s failed validation, use --skip-validation to force the operation", clusterManifestPath)
	}

	errorsHandler := func(machines []*clusterv1.Machine, errors field.ErrorList) ([]*clusterv1.Machine, error) {
		if len(errors) > 0 {
			printValidationErrors(errors)
			return nil, apierrors.InvalidMachineConfiguration(
				"%s failed validation, use --skip-validation to force the operation", machinesManifestPath)
		}
		return machines, nil
	}

	machines, err := machine.ParseAndDefaultAndValidate(machinesManifestPath, errorsHandler)
	if err != nil {
		return nil, nil, err
	}

	return cluster, machines, nil
}

func parseCluster(r io.Reader) (*clusterv1.Cluster, error) {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	cluster := &clusterv1.Cluster{}
	err = yaml.Unmarshal(bytes, cluster)
	if err != nil {
		return nil, err
	}

	return cluster, nil

}

func parseClusterManifest(file string) (*clusterv1.Cluster, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return parseCluster(f)
}

// Getters for nested fields needed externally
func (s *specs) getSSHKeyPath() string {
	return s.clusterSpec.SSHKeyPath
}

func (s *specs) getClusterName() string {
	return s.cluster.ObjectMeta.Name
}

func (s *specs) getClusterNamespace() string {
	if applyOptions.useManifestNamespace {
		return ""
	}
	return firstNonDefaultOrDefault(applyOptions.namespace, kubeconfigOptions.namespace)
}

func firstNonDefaultOrDefault(nses ...string) string {
	for _, ns := range nses {
		if ns != manifest.DefaultNamespace {
			return ns
		}
	}
	return manifest.DefaultNamespace
}

func (s *specs) getMasterPublicAddress() string {
	return s.masterSpec.Public.Address
}

func (s *specs) getMasterPrivateAddress() string {
	return s.masterSpec.Private.Address
}

func (s *specs) getCloudProvider() string {
	return s.clusterSpec.CloudProvider
}
