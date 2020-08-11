package testutils

import (
	"context"
	"io"
)

// MockRunner needed for testing plans
type MockRunner struct {
	Output string
	Err    error
}

func (r *MockRunner) clearRunnerState() {
	r.Output = ""
	r.Err = nil
}

func (r *MockRunner) setRunnerState(out string, err error) {
	r.Output = out
	r.Err = err
}

//SetRunCommand allows you to configure the output for RunCommand
func (r *MockRunner) SetRunCommand(out string, err error) {
	r.setRunnerState(out, err)
}

//ClearRunCommand undoes any output configured by SetRunCommand
func (r *MockRunner) ClearRunCommand() {
	r.clearRunnerState()
}

//RunCommand returns the test Output and Err values
func (r *MockRunner) RunCommand(_ context.Context, _ string, _ io.Reader) (string, error) {
	return r.Output, r.Err
}
