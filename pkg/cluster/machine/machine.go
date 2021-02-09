package machine

import (
	"fmt"

	"github.com/blang/semver"
	existinginfra1 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeimachine "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/cluster/machine"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// Validate validates the provided machines.
func Validate(machines []*clusterv1.Machine, bl []*existinginfra1.ExistingInfraMachine) field.ErrorList {
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
		if capeimachine.IsMaster(m) {
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

// GetKubernetesVersionFromManifest reads the version of the Kubernetes control
// plane from the provided machines' manifest. If no version is configured, the
// default Kubernetes version will be returned.
func GetKubernetesVersionFromManifest(machinesManifestPath string) (string, string, error) {
	machines, bl, err := capeimachine.ParseManifest(machinesManifestPath)
	if err != nil {
		return "", "", err
	}
	return capeimachine.GetKubernetesVersionFromMasterIn(machines, bl)
}
