package version

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/pkg/errors"
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

// MustMatchRange delegates to MatchesRange but panics on error instead of
// returning the error.
func MustMatchRange(version, versionsRange string) bool {
	matches, err := MatchesRange(version, versionsRange)
	if err != nil {
		panic(err)
	}
	return matches
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

func LessThan(v1, v2 string) (bool, error) {
	v1major, v1minor, v1patch, err := parseVersion(v1)
	if err != nil {
		return false, err
	}
	v2major, v2minor, v2patch, err := parseVersion(v2)
	if err != nil {
		return false, err
	}
	return (v1major < v2major) ||
		(v1major == v2major && v1minor < v2minor) ||
		(v1major == v2major && v1minor == v2minor && v1patch < v2patch), nil
}

func parseVersion(v string) (int, int, int, error) {
	if strings.HasPrefix(v, "v") {
		v = v[1:]
	}
	chunks := strings.Split(v, ".") // drop "v" at front
	if len(chunks) != 3 {           // major.minor.patch
		return -1, -1, -1, fmt.Errorf("Invalid kubernetes version: %s", v)
	}
	var results = []int{-1, -1, -1}
	for idx, item := range chunks {
		val, err := strconv.Atoi(item)
		if err != nil {
			return -1, -1, -1, errors.Wrapf(err, "version is invalid")
		}
		results[idx] = val
	}
	return results[0], results[1], results[2], nil
}
