package machine

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/blang/semver"
	yaml "github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	baremetalspecv1 "github.com/weaveworks/wksctl/pkg/baremetalproviderspec/v1alpha1"
	"github.com/weaveworks/wksctl/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientcmd "sigs.k8s.io/cluster-api/cmd/clusterctl/clientcmd"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	apierrors "sigs.k8s.io/cluster-api/pkg/errors"
	clusterutil "sigs.k8s.io/cluster-api/pkg/util"
)

// IsMaster returns true if the provided machine is a "Master", and false
// if it is a "Node" (i.e. worker node) or any other type of machine.
func IsMaster(machine *clusterv1.Machine) bool {
	return isLabeledWithSetMaster(machine)
}

// isLabeledWithSetMaster returns true if the provided machine is labeled with
//   metadata.labels.set: master
// or false otherwise.
func isLabeledWithSetMaster(machine *clusterv1.Machine) bool {
	labels := machine.GetObjectMeta().GetLabels()
	return labels["set"] == "master"
}

// IsNode returns false if the provided machine is a "Master", and true
// if it is a "Node" (i.e. worker node) or any other type of machine.
func IsNode(machine *clusterv1.Machine) bool {
	return !IsMaster(machine)
}

// FirstMaster scans the provided array of machines and return the first
// one which is a "Master" or nil if none.
func FirstMaster(machines []*clusterv1.Machine) *clusterv1.Machine {
	for _, machine := range machines {
		if IsMaster(machine) {
			return machine
		}
	}
	return nil
}

// FirstMasterInArray scans the provided array of machines and return the first
// one which is a "Master" or nil if none.
func FirstMasterInArray(machines []clusterv1.Machine) *clusterv1.Machine {
	for _, machine := range machines {
		if IsMaster(&machine) {
			return &machine
		}
	}
	return nil
}

// Config returns the provided machine's configuration.
func Config(machine *clusterv1.Machine) (*baremetalspecv1.BareMetalMachineProviderSpec, error) {
	codec, err := baremetalspecv1.NewCodec()
	if err != nil {
		return nil, err
	}
	machineSpec, err := codec.MachineProviderFromProviderSpec(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, apierrors.InvalidMachineConfiguration("Cannot unmarshal machine's providerSpec field: %v", err)
	}
	return machineSpec, err
}

// ParseManifest parses the provided machines manifest file.
func ParseManifest(file string) ([]*clusterv1.Machine, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	return Parse(f)
}

// Parse parses the provided machines io.Reader.
func Parse(r io.Reader) ([]*clusterv1.Machine, error) {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	list := &clusterv1.MachineList{}
	err = yaml.Unmarshal(bytes, &list)
	if err != nil {
		return nil, err
	}

	if list == nil {
		return []*clusterv1.Machine{}, nil
	}

	return clusterutil.MachineP(list.Items), nil
}

type machineValidationFunc func(int, *clusterv1.Machine) field.ErrorList

// Validate validates the provided machines.
func Validate(machines []*clusterv1.Machine) field.ErrorList {
	var errors field.ErrorList

	// Run global validation functions that operate on the full list of machines.
	for _, f := range []machineListValidationFunc{
		validateAtLeastOneMaster,
		validateVersions,
		validateKubernetesVersion,
	} {
		errors = append(errors, f(machines)...)
	}

	return errors
}

func machinePath(i int, args ...string) *field.Path {
	return field.NewPath(fmt.Sprintf("machines[%d]", i), args...)
}

type machineListValidationFunc func([]*clusterv1.Machine) field.ErrorList

// We need at least one master.
func validateAtLeastOneMaster(machines []*clusterv1.Machine) field.ErrorList {
	numMasters := 0

	for _, m := range machines {
		if IsMaster(m) {
			numMasters++
		}
	}

	if numMasters == 0 {
		return field.ErrorList{
			field.Invalid(
				field.NewPath("spec", "versions", "controlPlane"),
				machines[0].Spec.Versions.ControlPlane,
				"no master node defined, need at least one master"),
		}
	}

	return field.ErrorList{}
}

// Validate the Spec.Versions Machine field:
// - It's possible to specify no versions at all in any of the objects. In this
// case, we populate the version fields with a default value as they are
// mandatory when persisting the Machine object.
// - If a version is specified, all machine objects must use the same version
// (and they can't be left empty)
func validateVersions(machines []*clusterv1.Machine) field.ErrorList {
	var errors field.ErrorList
	reference := machines[0].Spec.Versions.Kubelet

	for i, m := range machines {
		if m.Spec.Versions.Kubelet != reference {
			errors = append(errors, field.Invalid(
				machinePath(i, "spec", "versions", "kubelet"),
				m.Spec.Versions.Kubelet,
				fmt.Sprintf("inconsistent kubelet version, expected \"%s\"", reference)))
		}

		controlPlaneVersion := m.Spec.Versions.ControlPlane
		if IsMaster(m) && controlPlaneVersion != "" && controlPlaneVersion != reference {
			errors = append(errors, field.Invalid(
				machinePath(i, "spec", "versions", "controlPlane"),
				m.Spec.Versions.ControlPlane,
				fmt.Sprintf("inconsistent controlPlane version, expected \"%s\"", reference)))
		}
	}

	return errors
}

// We restrict the Kubernetes versions to a tested subset. This test needs to be
// run after validateVersions. It's also a global test as opposed to a
// per-machine test to not repeat the validation errors many times if the
// specified versions don't match the ranges.
func validateKubernetesVersion(machines []*clusterv1.Machine) field.ErrorList {
	s := machines[0].Spec.Versions.Kubelet
	if s == "" {
		return field.ErrorList{}
	}

	version, err := semver.ParseTolerant(s)
	if err != nil {
		return field.ErrorList{
			field.Invalid(
				machinePath(0, "spec", "versions", "kubelet"),
				machines[0].Spec.Versions.Kubelet,
				"version isn't a semver version"),
		}
	}

	ranges := []string{
		kubernetes.DefaultVersionsRange,
	}
	for i := range ranges {
		r := semver.MustParseRange(ranges[i])
		if !r(version) {
			return field.ErrorList{
				field.Invalid(
					machinePath(0, "spec", "versions", "kubelet"),
					machines[0].Spec.Versions.Kubelet,
					fmt.Sprintf("version doesn't match range: %s", ranges[i])),
			}
		}
	}

	return field.ErrorList{}
}

type machinePopulateFunc func(*clusterv1.Machine)

func populateVersions(m *clusterv1.Machine) {
	// We have already validated the version fields are either all empty or have
	// the same value. Only populate them if they are empty.
	if m.Spec.Versions.Kubelet != "" {
		return
	}
	m.Spec.Versions.Kubelet = kubernetes.DefaultVersion
	if IsMaster(m) {
		m.Spec.Versions.ControlPlane = kubernetes.DefaultVersion
	}
}

// Kubeadm adds the master role label, but not the node one. Add it ourselves so
// we can have a nicer kubectl get nodes output.
// $ kubectl get nodes
// NAME      STATUS    ROLES     AGE       VERSION
// kube-01   Ready     master    55s       v1.10.5
// kube-02   Ready     node      23s       v1.10.5
func fixupNodeRoleLabel(m *clusterv1.Machine) {
	if IsNode(m) {
		m.Labels["node-role.kubernetes.io/node"] = ""
	}
}

// Populate mutates the machines manifests:
//   - fill in default values
func Populate(machines []*clusterv1.Machine) {
	for _, f := range []machinePopulateFunc{
		populateVersions,
		fixupNodeRoleLabel,
	} {
		for _, machine := range machines {
			f(machine)
		}
	}
}

// InvalidMachinesHandler encapsulates logic to apply in case of an invalid
// machines manifest being provided.
type InvalidMachinesHandler = func(machines []*clusterv1.Machine, errors field.ErrorList) ([]*clusterv1.Machine, error)

// NoOpInvalidMachinesHandler does nothing when an invalid machines manifest
// is being provided.
var NoOpInvalidMachinesHandler = func(machines []*clusterv1.Machine, errors field.ErrorList) ([]*clusterv1.Machine, error) {
	return nil, nil
}

// ParseAndDefaultAndValidate parses the provided manifest, validates it and
// defaults values where possible.
func ParseAndDefaultAndValidate(machinesManifestPath string, errorsHandler InvalidMachinesHandler) ([]*clusterv1.Machine, error) {
	machines, err := ParseManifest(machinesManifestPath)
	if err != nil {
		return nil, err
	}
	Populate(machines)

	errors := Validate(machines)
	return errorsHandler(machines, errors)
}

// GetKubernetesVersionFromManifest reads the version of the Kubernetes control
// plane from the provided machines' manifest. If no version is configured, the
// default Kubernetes version will be returned.
func GetKubernetesVersionFromManifest(machinesManifestPath string) (string, error) {
	machines, err := ParseManifest(machinesManifestPath)
	if err != nil {
		return "", err
	}
	return GetKubernetesVersionFromMasterIn(machines)
}

// GetKubernetesVersionFromMasterIn reads the version of the Kubernetes control
// plane from the provided machines. If no version is configured, the default
// Kubernetes version will be returned.
func GetKubernetesVersionFromMasterIn(machines []*clusterv1.Machine) (string, error) {
	// Ensures all machines have the same version (either specified or empty):
	errs := Validate(machines)
	if len(errs) > 0 {
		return "", errs.ToAggregate()
	}
	return GetKubernetesVersion(FirstMaster(machines)), nil
}

// GetKubernetesVersion reads the Kubernetes version of the provided machine,
// or if missing, returns the default version.
func GetKubernetesVersion(machine *clusterv1.Machine) string {
	if machine == nil {
		return kubernetes.DefaultVersion
	}
	version := getKubernetesVersion(machine)
	log.WithField("machine", machine.Name).WithField("version", version).Debug("Kubernetes version used")
	return version
}

func getKubernetesVersion(machine *clusterv1.Machine) string {
	if machine.Spec.Versions.ControlPlane != "" {
		return machine.Spec.Versions.ControlPlane
	}
	log.WithField("machine", machine.Name).Debug("No Kubernetes control plane version configured in manifest, falling back to kubelet version")
	if machine.Spec.Versions.Kubelet != "" {
		return machine.Spec.Versions.Kubelet
	}
	log.WithField("machine", machine.Name).WithField("defaultVersion", kubernetes.DefaultVersion).Debug("No kubelet version configured in manifest, falling back to default")
	return kubernetes.DefaultVersion
}

// GetKubernetesNamespaceFromMachines reads the namespace of the Kubernetes control
// plane from the applied machines. If no namespace is found, the
// default Kubernetes namespace will be returned.
func GetKubernetesNamespaceFromMachines() (string, error) {
	cs, err := clientcmd.NewClusterApiClientForDefaultSearchPath("", clientcmd.NewConfigOverrides())
	if err != nil {
		return "", err
	}
	client := cs.ClusterV1alpha1()
	mi := client.Machines("")
	if mi == nil {
		return "", errors.New("No MachineInterface found")
	}
	mlist, err := mi.List(v1.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, m := range mlist.Items {
		mNS := m.ObjectMeta.Namespace
		if mNS == "" {
			continue
		}
		return mNS, nil
	}
	return manifest.DefaultNamespace, nil
}
