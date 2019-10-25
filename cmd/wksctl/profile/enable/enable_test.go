package enable

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	ignoreFileFixture = `a
b/
 #c
#d
e
f
../g
h..
i # this is i
 j/# this is j
`
)

func TestParseDotIgnoreFile(t *testing.T) {
	r := strings.NewReader(ignoreFileFixture)
	lines, err := parseDotIgnorefile("", r)
	assert.NoError(t, err, "parsing ignore file should not be error")
	assert.Equal(t, 8, len(lines), "ignore file entries should be 8")
	// Entry: 'b/' is resolved to 'b' by path.Join()
	assert.Equal(t, []string{"a", "b", "e", "f", "../g", "h..", "i", "j"}, lines)
}

func TestParseDotIgnoreFileWithPrefix(t *testing.T) {
	r := strings.NewReader(ignoreFileFixture)
	lines, err := parseDotIgnorefile("profiles", r)
	assert.NoError(t, err, "parsing ignore file should not be error")
	assert.Equal(t, 8, len(lines), "ignore file entries should be 8")
	// Note:
	// - 'profiles/b/' is resolved to 'profiles/b'
	// - 'profiles/../g' is resolved to 'g'
	assert.Equal(t, []string{
		"profiles/a", "profiles/b", "profiles/e",
		"profiles/f", "g", "profiles/h..",
		"profiles/i", "profiles/j"}, lines)
}
