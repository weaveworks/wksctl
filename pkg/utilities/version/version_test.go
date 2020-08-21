package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchesRangeAnyVersionsRangeWildcard(t *testing.T) {
	matches, err := MatchesRange("1.anything", "*")
	assert.NoError(t, err)
	assert.True(t, matches)
}

func TestMatchesRangeAnyVersionsRangeBlank(t *testing.T) {
	matches, err := MatchesRange("1.anything", "")
	assert.NoError(t, err)
	assert.True(t, matches)
}

func TestMatchesInvalidVersionsRange(t *testing.T) {
	matches, err := MatchesRange("1.2.3", "bad range")
	assert.EqualError(t, err, "invalid versions range \"bad range\": Could not get version from string: \"bad\"")
	assert.False(t, matches)
}

func TestMatchesInvalidVersion(t *testing.T) {
	matches, err := MatchesRange("bad version", ">1.0.0 <=1.2.3")
	assert.EqualError(t, err, "invalid version \"bad version\": Invalid character(s) found in major number \"bad version\"")
	assert.False(t, matches)
}
