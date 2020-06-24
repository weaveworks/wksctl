package test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Check "help" has some help blurb about the the "apply", "help", "version"
// commands.
func TestHelp(t *testing.T) {
	exe := run.NewExecutor()

	run, err := exe.RunCmd(exec.Command(cmd, "help"))
	require.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode())
	assert.True(t, run.Contains("apply"))
	assert.True(t, run.Contains("help"))
	assert.True(t, run.Contains("version"))
}
