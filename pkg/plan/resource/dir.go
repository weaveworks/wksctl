package resource

import (
	"fmt"
	"strings"

	"github.com/weaveworks/wksctl/pkg/plan"
)

// XXX: Expose file permission (if needed?)

// Dir represents a directory on the file system.
type Dir struct {
	// Path at which to create directory
	Path fmt.Stringer `structs:"path,omitempty"`
	// RecursiveDelete makes the undo operation recursive
	RecursiveDelete bool
}

var _ plan.Resource = plan.RegisterResource(&Dir{})

var protectedDirs = make(map[string]struct{})

func init() {
	for _, dir := range []string{"/", "/etc", "/var", "/dev", "/usr", "/root", "/home", "/opt", "/bin", "/sbin"} {
		protectedDirs[dir] = struct{}{}
	}
}

// State implements plan.Resource.
func (d *Dir) State() plan.State {
	return toState(d)
}

// QueryState implements plan.Resource.
func (d *Dir) QueryState(runner plan.Runner) (plan.State, error) {
	return plan.EmptyState, nil
}

// Apply implements plan.Resource.
func (d *Dir) Apply(runner plan.Runner, diff plan.Diff) (bool, error) {
	_, err := runner.RunCommand(fmt.Sprintf("mkdir -p %s", d.Path), nil)
	if err != nil {
		return false, err
	}
	return true, nil
}

// Undo implements plan.Resource.
func (d *Dir) Undo(runner plan.Runner, current plan.State) error {
	path := strings.TrimRight(d.Path.String(), "/")
	if _, ok := protectedDirs[path]; ok {
		return fmt.Errorf("deletion aborted because dir is blacklisted for deletion: %s", path)
	}

	var cmd string
	if d.RecursiveDelete {
		cmd = fmt.Sprintf("rm -rvf -- %q", path)
	} else {
		cmd = fmt.Sprintf("[ ! -e %q ] || rmdir -v --ignore-fail-on-non-empty -- %q", path, path)
	}

	_, err := runner.RunCommand(cmd, nil)
	return err
}
