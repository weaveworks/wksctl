package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	"github.com/weaveworks/wksctl/test/container/images"
)

func TestExitCode(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	for _, tt := range []struct {
		command      string
		wantExitCode int
	}{
		{"/bin/true", 0},
		{"/bin/false", 1},
		{"(exit 2)", 2},
		{"(exit 58)", 58},
	} {
		wantError := (tt.wantExitCode != 0)

		t.Run(tt.command, func(t *testing.T) {
			_, gotErr := r.RunCommand(tt.command, nil)

			assert.Equal(t, wantError, gotErr != nil)

			if wantError {
				assert.Equal(t, tt.wantExitCode, gotErr.(*plan.RunError).ExitCode)
			}
		})
	}
}
