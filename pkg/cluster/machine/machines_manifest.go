package machine

import (
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/thanhpk/randstr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/yaml"
)

// GetMachinesManifest reads a manifest from the filesystem and updates it with generated names (see: UpdateWithGeneratedNames)
func GetMachinesManifest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	return UpdateWithGeneratedNames(f)
}

// UpdateWithGeneratedNames generates names for machines, rather than using
// Kubernetes "generateName". This is necessary as:
// - one can only "kubectl create" manifests with "generateName" fields, not
//   "kubectl apply" them,
// - WKS needs to be as idempotent as possible.
// Note that if the customer updates the manifest with their own names, we'll
// honor those.
func UpdateWithGeneratedNames(r io.ReadCloser) (string, error) {
	machines, bml, err := Parse(r)
	if err != nil {
		return "", err
	}

	// Get all the machine names currently used, either set by a previous call
	// to this function, or set by the end-user.
	namesTaken := readNames(machines)
	for i := range machines {
		if machines[i].ObjectMeta.GenerateName != "" {
			// TODO: update BareMetalMachine list here too
			if len(bml) > i && bml[i].ObjectMeta.GenerateName != "" {
				return "", errors.New("generateName not implemented for v1alpha3")
			}
			name := uniqueNameFrom(machines[i].ObjectMeta.GenerateName, namesTaken)
			machines[i].SetName(name)
			// Blank generateName out, now that a name has been generated.
			machines[i].SetGenerateName("")
		}
	}

	var buf strings.Builder
	// Need to do this in a loop because we want a stream not an array
	for _, machine := range machines {
		manifestBytes, err := yaml.Marshal(machine)
		if err != nil {
			return "", err
		}
		buf.WriteString("---\n")
		buf.Write(manifestBytes)
	}
	for _, machine := range bml {
		manifestBytes, err := yaml.Marshal(machine)
		if err != nil {
			return "", err
		}
		buf.WriteString("---\n")
		buf.Write(manifestBytes)
	}
	return buf.String(), nil
}

func readNames(machines []*clusterv1.Machine) map[string]struct{} {
	namesTaken := map[string]struct{}{}
	for _, machine := range machines {
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
