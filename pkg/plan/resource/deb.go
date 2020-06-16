package resource

import (
	"fmt"

	"github.com/hashicorp/go-version"
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

func isLowerRevision(v1, v2 *Deb) (bool, error) {
	// return strings.Compare(v1.Suffix, v2.Suffix) < 0
	compareV1, err := version.NewVersion(v1.Suffix)
	if err != nil {
		return false, err
	}

	compareV2, err := version.NewVersion(v2.Suffix)
	if err != nil {
		return false, err
	}

	if compareV1.LessThan(compareV2) {
		return true, nil
	}
	return false, nil
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

	q := dpkgQuerier{Runner: runner}
	installed, err := q.ShowInstalled(d.Name)

	if err != nil {
		return false, err
	}

	if len(installed) == 0 {
		if err := a.Install(d.Name, d.Suffix); err != nil {
			return false, err
		}
	} else if len(installed) > 0 {
		currentVersion := DebResourceFromPackage(installed[0])
		isLower, err := isLowerRevision(currentVersion, d)

		if err != nil {
			return false, err
		}

		// Check if current version is lower than the new one
		// to either decide to upgrade or install the new version
		if isLower {
			if err := a.Upgrade(d.Name, d.Suffix); err != nil {
				return false, err
			}
		} else if !isLower {
			if err := a.Install(d.Name, d.Suffix); err != nil {
				return false, err
			}
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
