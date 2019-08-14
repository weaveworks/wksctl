package sudo

import (
	"io"
	"strings"

	"github.com/weaveworks/wksctl/pkg/plan"
)

// Runner wraps the inner Runner with sudo.
type Runner struct {
	Runner plan.Runner
}

// RunCommand wraps the command with sudo and passes it on to the wrapped RunCommand.
func (s *Runner) RunCommand(cmd string, stdin io.Reader) (stdouterr string, err error) {
	return s.Runner.RunCommand("sudo -n -- sh -c "+escape(cmd), stdin)
}

func escape(cmd string) string {
	return "'" + strings.Replace(cmd, "'", "'\"'\"'", -1) + "'"
}
