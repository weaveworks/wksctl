package resource

import (
	"fmt"
	"strings"

	"github.com/cavaliercoder/go-rpm/version"
	"github.com/weaveworks/wksctl/pkg/plan"
)

// RPM represents an RPM package.
//
// It isn't legal to provide a Release if no Version is specified.
// TODO: What about epoch?
type RPM struct {
	Name string `structs:"name"`
	// Version is optional
	Version         string `structs:"version,omitempty"`
	Release         string `structs:"release,omitempty"`
	DisableExcludes string `structs:"disableExcludes,omitempty"`
}

type rpmState plan.State

// Name implements version.Interface
func (s rpmState) Name() string {
	if name, ok := s["name"]; ok {
		return name.(string)
	}
	return ""
}

// Epoch implements version.Interface
func (s rpmState) Epoch() int {
	return 0
}

// Version implements version.Interface
func (s rpmState) Version() string {
	if version, ok := s["version"]; ok {
		return version.(string)
	}
	return ""
}

// Release implements version.Interface
func (s rpmState) Release() string {
	if release, ok := s["release"]; ok {
		return release.(string)
	}
	return ""
}

var _ plan.Resource = plan.RegisterResource(&RPM{})

// State implements plan.Resource.
func (p *RPM) State() plan.State {
	return toState(p)
}

func lowerRevisionThan(state1, state2 plan.State) bool {
	return version.Compare(rpmState(state1), rpmState(state2)) < 0
}

func label(name, version, release string) string {
	if release != "" {
		return fmt.Sprintf("%s-%s-%s", name, version, release)
	}
	if version != "" {
		return fmt.Sprintf("%s-%s", name, version)
	}
	return name

}

func (p *RPM) label() string {
	return label(p.Name, p.Version, p.Release)
}

// QueryState implements plan.Resource.
func (p *RPM) QueryState(r plan.Runner) (plan.State, error) {
	output, err := r.RunCommand(fmt.Sprintf("rpm -q --queryformat '%%{NAME} %%{VERSION} %%{RELEASE}\\n' %s", p.label()), nil)
	if err != nil && strings.Contains(output, "is not installed") {
		// Package isn't installed.
		return plan.EmptyState, nil
	}
	if err != nil {
		// An error happened running rpm.
		return plan.EmptyState, fmt.Errorf("Query rpm %s failed: %v -- %s", p.label(), err, output)
	}

	// XXX: in theory rpm queries can return multiple versions of the same package
	// if all of them are installed a the same. This shouldn't be a thing for the
	// packages we query.
	l := line(output)
	parts := strings.Split(l, " ")
	queriedPackage := &RPM{
		Name:    parts[0],
		Version: parts[1],
		Release: parts[2],
	}
	return queriedPackage.State(), nil
}

func (p *RPM) stateDifferent(current plan.State) bool {
	if current.IsEmpty() {
		return true
	}

	desired := p.label()
	installed := label(current.String("name"), current.String("version"), current.String("release"))
	return !strings.HasPrefix(installed, desired)
}

// WouldChangeState returns false if a call to Apply() is guaranteed not to change the installed version of the package, and true otherwise.
func (p *RPM) WouldChangeState(r plan.Runner) (bool, error) {
	current, err := p.QueryState(r)
	if err != nil {
		return false, err
	}
	return p.stateDifferent(current), nil
}

// Apply implements plan.Resource.
func (p *RPM) Apply(r plan.Runner, diff plan.Diff) (bool, error) {
	if !p.stateDifferent(diff.CurrentState) {
		return false, nil
	}

	// First assume the package doesn't exist at all
	var cmd string
	if diff.CurrentState.IsEmpty() {
		cmd = fmt.Sprintf("yum -y install %s", p.label())
	} else if lowerRevisionThan(diff.CurrentState, p.State()) {
		cmd = fmt.Sprintf("yum -y upgrade-to %s", p.label())
	} else if lowerRevisionThan(p.State(), diff.CurrentState) {
		cmd = fmt.Sprintf("yum -y remove %s && yum -y install %s", p.Name, p.label())
	}

	if p.DisableExcludes != "" {
		cmd = fmt.Sprintf("%s --disableexcludes %s", cmd, p.DisableExcludes)
	}
	_, err := r.RunCommand(cmd, nil)
	return err == nil, err
}

func (p *RPM) shouldUndo(current plan.State) bool {
	// If package isn't installed, nothing to do!
	return !current.IsEmpty()
}

// Undo implements plan.Resource
func (p *RPM) Undo(r plan.Runner, current plan.State) error {
	if !p.shouldUndo(current) {
		return nil
	}

	_, err := r.RunCommand(fmt.Sprintf("yum -y remove %s", p.label()), nil)
	return err
}
