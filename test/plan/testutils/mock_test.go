package testutils

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestSetRunCommandNoError(t *testing.T) {
	r := &MockRunner{}
	r.SetRunCommand("foo", nil)
	out, err := r.RunCommand("bar", nil)
	assert.Equal(t, "foo", out)
	assert.NoError(t, err)
}

func TestSetRunCommandWithError(t *testing.T) {
	r := &MockRunner{}
	errstr := "error running command bar"
	r.SetRunCommand("", errors.New(errstr))
	out, err := r.RunCommand("bar", nil)
	assert.Empty(t, out)
	assert.Error(t, err)
	assert.EqualError(t, err, errstr)
}
