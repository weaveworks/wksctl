package run

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunCmd(t *testing.T) {
	e := New(Options{}).NewExecutor()

	cmd, err := e.RunCmd(exec.Command("ls", "/"))
	assert.NoError(t, err)
	assert.True(t, cmd.Contains("etc"))
	assert.Equal(t, 0, cmd.ExitCode())
}

func TestRunNonExistentCmd(t *testing.T) {
	e := New(Options{}).NewExecutor()
	_, err := e.RunCmd(exec.Command("foofoobar", "--help"))
	assert.Error(t, err)
}

func TestExitStatus(t *testing.T) {
	e := New(Options{}).NewExecutor()

	cmd, err := e.RunCmd(exec.Command("ls", "/"))
	assert.NoError(t, err)
	assert.Equal(t, 0, cmd.ExitCode())

	cmd, err = e.RunCmd(exec.Command("ls", "/foofoobar"))
	assert.NoError(t, err)
	assert.NotEqual(t, 0, cmd.ExitCode())
}
