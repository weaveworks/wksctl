package version

import (
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
