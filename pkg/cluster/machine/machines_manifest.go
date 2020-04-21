package machine

import (
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
	"github.com/thanhpk/randstr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/yaml"
)

// GetMachinesManifest reads a manifest from the filesystem and updates it with generated names (see: UpdateWithGeneratedNames)
func GetMachinesManifest(path string) (string, error) {
	machinesManifestBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return UpdateWithGeneratedNames(string(machinesManifestBytes))
}

// UpdateWithGeneratedNames generates names for machines, rather than using
// Kubernetes "generateName". This is necessary as:
// - one can only "kubectl create" manifests with "generateName" fields, not
//   "kubectl apply" them,
// - WKS needs to be as idempotent as possible.
// Note that if the customer updates the manifest with their own names, we'll
// honor those.
func UpdateWithGeneratedNames(manifest string) (string, error) {
	return "", errors.New("generateName not implemented for v1alpha3")

	var machineList clusterv1.MachineList
	if err := yaml.Unmarshal([]byte(manifest), &machineList); err != nil {
		return "", errors.Wrap(err, "failed to deserialize machines' manifest")
	}

	// Get all the machine names currently used, either set by a previous call
	// to this function, or set by the end-user.
	namesTaken := readNames(&machineList)
	for i := range machineList.Items {
		if machineList.Items[i].ObjectMeta.GenerateName != "" {
			name := uniqueNameFrom(machineList.Items[i].ObjectMeta.GenerateName, namesTaken)
			machineList.Items[i].SetName(name)
			// Blank generateName out, now that a name has been generated.
			machineList.Items[i].SetGenerateName("")
		}
	}

	manifestBytes, err := yaml.Marshal(machineList)
	if err != nil {
		return "", err
	}
	return string(manifestBytes), nil
}

func readNames(machineList *clusterv1.MachineList) map[string]struct{} {
	namesTaken := map[string]struct{}{}
	for _, machine := range machineList.Items {
		if machine.ObjectMeta.Name != "" {
			namesTaken[machine.ObjectMeta.Name] = struct{}{}
		}
	}
	return namesTaken
}

func uniqueNameFrom(prefix string, namesTaken map[string]struct{}) string {
	for {
		suffix := strings.Join(randomStrings(5, 2), "-")
		name := prefix + suffix
		if _, taken := namesTaken[name]; !taken {
			namesTaken[name] = struct{}{}
			return name
		}
	}
}

// randomStrings generates an array of random alpha-numerical strings, each of
// them matching the following regular expression: [A-Za-z0-9]+
// - size is the length of the each random string
// - num is the number of strings to generate.
func randomStrings(size, num int) []string {
	strSlice := make([]string, num)
	for i := range strSlice {
		strSlice[i] = strings.ToLower(randstr.String(size))
	}
	return strSlice
}
