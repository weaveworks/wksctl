package version_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/utilities/version"
)

func TestMatchesRangeDefaultVersion(t *testing.T) {
	matches, err := version.MatchesRange(kubernetes.DefaultVersion, kubernetes.DefaultVersionsRange)
	assert.NoError(t, err)
	assert.True(t, matches)
}
func TestMatchesRangeAnyVersionsRangeWildcard(t *testing.T) {
	matches, err := version.MatchesRange("1.anything", "*")
	assert.NoError(t, err)
	assert.True(t, matches)
}
func TestMatchesRangeAnyVersionsRangeBlank(t *testing.T) {
	matches, err := version.MatchesRange("1.anything", "")
	assert.NoError(t, err)
	assert.True(t, matches)
}

func TestMatchesInvalidVersionsRange(t *testing.T) {
	matches, err := version.MatchesRange("1.2.3", "bad range")
	assert.EqualError(t, err, "invalid versions range \"bad range\": Could not get version from string: \"bad\"")
	assert.False(t, matches)
}

func TestMatchesInvalidVersion(t *testing.T) {
	matches, err := version.MatchesRange("bad version", ">1.0.0 <=1.2.3")
	assert.EqualError(t, err, "invalid version \"bad version\": Invalid character(s) found in major number \"bad version\"")
	assert.False(t, matches)
}

func TestVersionLessthanWithBothVs(t *testing.T) {
	lt, err := version.LessThan("v1.14.7", "v1.15.0")
	assert.NoError(t, err)
	assert.True(t, lt)
}

func TestVersionLessthanWithFormerV(t *testing.T) {
	lt, err := version.LessThan("v1.14.7", "1.15.0")
	assert.NoError(t, err)
	assert.True(t, lt)
}

func TestVersionLessthanWithLatterV(t *testing.T) {
	lt, err := version.LessThan("1.14.7", "v1.15.0")
	assert.NoError(t, err)
	assert.True(t, lt)
}

func TestVersionLessthanWithOutV(t *testing.T) {
	lt, err := version.LessThan("1.14.7", "1.15.0")
	assert.NoError(t, err)
	assert.True(t, lt)
}
