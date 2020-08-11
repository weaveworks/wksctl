package plan

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLocalRunnerRunCommand(t *testing.T) {
	var r Runner = &LocalRunner{}
	output, err := r.RunCommand(context.Background(), `echo "success"`, nil)
	assert.NoError(t, err)
	assert.Equal(t, "success\n", output)
}

func TestLocalRunnerCommandNotFound(t *testing.T) {
	var r Runner = &LocalRunner{}
	output, err := r.RunCommand(context.Background(), `foofoo`, nil)
	assert.Error(t, err)
	assert.Regexp(t, regexp.MustCompile("foofoo:.*not found"), output)
}

func TestLocalRunnerExitCode(t *testing.T) {
	for _, tt := range []struct {
		command      string
		wantExitCode int
	}{
		{"(exit 0)", 0},
		{"(exit 1)", 1},
		{"(exit 2)", 2},
		{"(exit 58)", 58},
	} {
		wantError := (tt.wantExitCode != 0)

		t.Run(tt.command, func(t *testing.T) {
			var r Runner = &LocalRunner{}
			_, gotErr := r.RunCommand(context.Background(), tt.command, nil)

			assert.Equal(t, wantError, gotErr != nil)

			if wantError {
				assert.Equal(t, tt.wantExitCode, gotErr.(*RunError).ExitCode)
			}
		})
	}
}
