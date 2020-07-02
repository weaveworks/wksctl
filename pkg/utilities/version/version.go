package version

import (
	"github.com/blang/semver"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/version"
)

const (
	// AnyRange represents any versions range.
	AnyRange   = "*"
	emptyRange = ""
)

// MatchesRange parses the provided version and versions range, and checks if
// the provided version matches the provided range.
func MatchesRange(version, versionsRange string) (bool, error) {
	if versionsRange == AnyRange || versionsRange == emptyRange {
		// Check specifically for the above special cases, as otherwise semver
		// fails with "Last element in range is '||'".
		return true, nil
	}
	v, err := semver.ParseTolerant(version)
	if err != nil {
		return false, errors.Wrapf(err, "invalid version \"%s\"", version)
	}
	r, err := semver.ParseRange(versionsRange)
	if err != nil {
		return false, errors.Wrapf(err, "invalid versions range \"%s\"", versionsRange)
	}
	return r(v), nil
}

func Jump(nodeVersion, machineVersion string) (bool, error) {
	nodemajor, nodeminor, _, err := parseVersion(nodeVersion)
	if err != nil {
		return false, err
	}
	machinemajor, machineminor, _, err := parseVersion(machineVersion)
	if err != nil {
		return false, err
	}
	return machinemajor == nodemajor && machineminor-nodeminor > 1, nil
}

func LessThan(s1, s2 string) (bool, error) {
	v1, err := version.ParseSemantic(s1)
	if err != nil {
		return false, err
	}
	v2, err := version.ParseSemantic(s2)
	if err != nil {
		return false, err
	}
	return v1.LessThan(v2), nil
}

func parseVersion(s string) (int, int, int, error) {
	v, err := version.ParseSemantic(s)
	if err != nil {
		return -1, -1, -1, errors.Wrap(err, "invalid kubernetes version")
	}
	return int(v.Major()), int(v.Minor()), int(v.Patch()), nil
}
