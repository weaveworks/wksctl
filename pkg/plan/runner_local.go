package plan

import (
	"context"
	"io"
	"os/exec"
	"syscall"
)

// LocalRunner is a runner executing commands on the same host it's running on.
type LocalRunner struct {
}

var _ Runner = &LocalRunner{}

// RunCommand implements Runner.
func (r *LocalRunner) RunCommand(ctx context.Context, cmd string, stdin io.Reader) (stdouterr string, err error) {
	command := exec.Command("sh", "-c", cmd)
	command.Stdin = stdin

	output, err := command.CombinedOutput()
	return string(output), extractExitCode(err)
}

func extractExitCode(err error) error {
	if err, ok := err.(*exec.ExitError); ok {
		if stat, ok := err.Sys().(syscall.WaitStatus); ok {
			return &RunError{ExitCode: stat.ExitStatus()}
		}
	}
	return err
}
