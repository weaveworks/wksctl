package resource

import (
	"fmt"

	"github.com/weaveworks/wksctl/pkg/plan"
)

// Deb represents a .deb package.
type Deb struct {
	Name string `structs:"name"`
	// Suffix is either "=" followed by the version, or "/" followed by the release stream (stable|testing|unstable).
	// Examples:
	//   Name: "busybox"
	//   Name: "busybox", Suffix: "/stable"
	//   Name: "busybox", Suffix: "=1:1.27.2-2ubuntu3.2"
	Suffix string `structs:"suffix"`
}

var _ plan.Resource = plan.RegisterResource(&Deb{})

func (d *Deb) State() plan.State {
	return toState(d)
}

func (d *Deb) QueryState(runner plan.Runner) (plan.State, error) {
	q := dpkgQuerier{Runner: runner}
	installed, err := q.ShowInstalled(d.Name)

	if err != nil {
		return nil, err
	}

	if len(installed) == 0 {
		return plan.EmptyState, nil
	}

	return DebResourceFromPackage(installed[0]).State(), nil
}

func (d *Deb) Apply(runner plan.Runner, diff plan.Diff) (propagate bool, err error) {
	a := aptInstaller{Runner: runner}
	if err := a.UpdateCache(); err != nil {
		return false, fmt.Errorf("update cache failed: %v", err)
	}

	if diff.CurrentState.IsEmpty() {
		if err := a.Install(d.Name, d.Suffix); err != nil {
			return false, err
		}
	} else {
		if err := a.Upgrade(d.Name, d.Suffix); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (d *Deb) Undo(runner plan.Runner, current plan.State) error {
	a := aptInstaller{Runner: runner}
	return a.Purge(d.Name)
}

func DebResourceFromPackage(p debPkgInfo) *Deb {
	return &Deb{
		Name:   p.Name,
		Suffix: "=" + p.Version,
	}
}

// WouldChangeState returns false if it's guaranteed that a call to Apply() wouldn't change the package installed, and true otherwise.
func (d *Deb) WouldChangeState(r plan.Runner) (bool, error) {
	current, err := d.QueryState(r)
	if err != nil {
		return false, err
	}
	return !current.Equal(d.State()), nil
}
