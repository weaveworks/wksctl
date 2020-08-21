package machine

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	capeiv1alpha3 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeimachine "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/cluster/machine"
	capeikubernetes "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/kubernetes"
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/wksctl/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/scheme"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
// Machines and ExistingInfraMachines must be in the same order
func FirstMaster(machines []*clusterv1.Machine, bl []*capeiv1alpha3.ExistingInfraMachine) (*clusterv1.Machine, *capeiv1alpha3.ExistingInfraMachine) {
	// TODO: validate size and ordering of lists
	for i, machine := range machines {
		if IsMaster(machine) {
			return machine, bl[i]
		}
	}
	return nil, nil
}

// ParseManifest parses the provided machines manifest file.
func ParseManifest(file string) (ml []*clusterv1.Machine, bl []*capeiv1alpha3.ExistingInfraMachine, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	ml, bl, err = Parse(f)
	if err != nil {
		err = errors.Wrapf(err, "parsing %q", file)
	}
	return
}

// Parse parses the provided machines io.Reader.
func Parse(rc io.ReadCloser) (ml []*clusterv1.Machine, bl []*capeiv1alpha3.ExistingInfraMachine, err error) {
	// Read from the ReadCloser YAML document-by-document
	fr := serializer.NewYAMLFrameReader(rc)

	// Decode all objects in the FrameReader
	objs, err := scheme.Serializer.Decoder().DecodeAll(fr)
	if err != nil {
		return
	}

	// Loop through the untyped objects we got and add them to the specific lists
	for _, obj := range objs {
		switch typed := obj.(type) {
		case *clusterv1.Machine:
			ml = append(ml, typed)
		case *capeiv1alpha3.ExistingInfraMachine:
			bl = append(bl, typed)
		default:
			return nil, nil, fmt.Errorf("unexpected type %T", obj)
		}
	}

	return
}

// Validate validates the provided machines.
func Validate(machines []*clusterv1.Machine, bl []*capeiv1alpha3.ExistingInfraMachine) field.ErrorList {
	if len(machines) == 0 { // Some other validations crash on empty list
		return field.ErrorList{nonFieldError("no machines")}
	}

	var errors field.ErrorList

	// Run global validation functions that operate on the full list of machines.
	for _, f := range []machineListValidationFunc{
		validateAtLeastOneMaster,
		validateVersions,
		validateKubernetesVersion,
	} {
		errors = append(errors, f(machines)...)
	}

	// Check 1-1 correspondence between lists
	if len(machines) != len(bl) {
		errors = append(errors, nonFieldError("mismatch: %d Machines and %d ExistingInfraMachines", len(machines), len(bl)))
	} else {
		// TODO: what if the user has a mixture of our machines and someone else's?
		for i, m := range machines {
			ref := m.Spec.InfrastructureRef
			if ref.Name != bl[i].ObjectMeta.Name {
				errors = append(errors, nonFieldError("mismatch [%d]: reference %q != %q", i, ref.Name, bl[i].ObjectMeta.Name))
			}
		}
	}

	return errors
}

// Map an error which can't be expressed as a single-field error into one,
// TODO: fix the rest of the code which assumes all errors are field errors
func nonFieldError(format string, args ...interface{}) *field.Error {
	return field.Invalid(field.NewPath("spec"), "[...]", fmt.Sprintf(format, args...))
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
				field.NewPath("metadata", "labels", "set"),
				"",
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
	reference := machines[0].Spec.Version

	for i, m := range machines {
		if reference == nil {
			if m.Spec.Version != nil {
				errors = append(errors, field.Invalid(
					machinePath(i, "spec", "version"),
					m.Spec.Version,
					"inconsistent kubernetes version, expected nil"))
			}
		} else {
			if m.Spec.Version == nil {
				errors = append(errors, field.Invalid(
					machinePath(i, "spec", "version"),
					nil,
					fmt.Sprintf("inconsistent kubernetes version, expected %q", *reference)))
			} else if *reference != *m.Spec.Version {
				errors = append(errors, field.Invalid(
					machinePath(i, "spec", "version"),
					*m.Spec.Version,
					fmt.Sprintf("inconsistent kubernetes version, expected %q", *reference)))
			}
		}
	}

	return errors
}

// We restrict the Kubernetes versions to a tested subset. This test needs to be
// run after validateVersions. It's also a global test as opposed to a
// per-machine test to not repeat the validation errors many times if the
// specified versions don't match the ranges.
func validateKubernetesVersion(machines []*clusterv1.Machine) field.ErrorList {
	s := machines[0].Spec.Version
	if s == nil {
		return field.ErrorList{}
	}

	version, err := semver.ParseTolerant(*s)
	if err != nil {
		return field.ErrorList{
			field.Invalid(
				machinePath(0, "spec", "version"),
				machines[0].Spec.Version,
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
					machinePath(0, "spec", "version"),
					machines[0].Spec.Version,
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
	if m.Spec.Version != nil {
		return
	}
	versionCopy := capeikubernetes.DefaultVersion
	m.Spec.Version = &versionCopy
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
type InvalidMachinesHandler = func(machines []*clusterv1.Machine, bl []*capeiv1alpha3.ExistingInfraMachine, errors field.ErrorList) ([]*clusterv1.Machine, []*capeiv1alpha3.ExistingInfraMachine, error)

// NoOpInvalidMachinesHandler does nothing when an invalid machines manifest
// is being provided.
var NoOpInvalidMachinesHandler = func(machines []*clusterv1.Machine, errors field.ErrorList) ([]*clusterv1.Machine, error) {
	return nil, nil
}

// ParseAndDefaultAndValidate parses the provided manifest, validates it and
// defaults values where possible.
func ParseAndDefaultAndValidate(machinesManifestPath string, errorsHandler InvalidMachinesHandler) ([]*clusterv1.Machine, []*capeiv1alpha3.ExistingInfraMachine, error) {
	machines, bl, err := ParseManifest(machinesManifestPath)
	if err != nil {
		return nil, nil, err
	}
	Populate(machines)

	errors := Validate(machines, bl)
	return errorsHandler(machines, bl, errors)
}

// GetKubernetesVersionFromManifest reads the version of the Kubernetes control
// plane from the provided machines' manifest. If no version is configured, the
// default Kubernetes version will be returned.
func GetKubernetesVersionFromManifest(machinesManifestPath string) (string, string, error) {
	machines, bl, err := ParseManifest(machinesManifestPath)
	if err != nil {
		return "", "", err
	}
	return GetKubernetesVersionFromMasterIn(machines, bl)
}

// GetKubernetesVersionFromMasterIn reads the version of the Kubernetes control
// plane from the provided machines. If no version is configured, the default
// Kubernetes version will be returned.
func GetKubernetesVersionFromMasterIn(machines []*clusterv1.Machine, bl []*capeiv1alpha3.ExistingInfraMachine) (string, string, error) {
	// Ensures all machines have the same version (either specified or empty):
	errs := Validate(machines, bl)
	if len(errs) > 0 {
		return "", "", errs.ToAggregate()
	}
	machine, _ := FirstMaster(machines, bl)
	version := capeimachine.GetKubernetesVersion(machine)
	ns := machine.ObjectMeta.Namespace
	if ns == "" {
		ns = manifest.DefaultNamespace
	}
	log.WithField("machine", machine.Name).WithField("version", version).WithField("namespace", ns).Debug("Kubernetes version used")
	return version, ns, nil
}

// GetKubernetesNamespaceFromMachines reads the namespace of the Kubernetes control
// plane from the applied machines. If no namespace is found, the
// default Kubernetes namespace will be returned.
func GetKubernetesNamespaceFromMachines(ctx context.Context, c client.Client) (string, error) {
	mlist := &clusterv1.MachineList{}

	if err := c.List(ctx, mlist); err != nil {
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
