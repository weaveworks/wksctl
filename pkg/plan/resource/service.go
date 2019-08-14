package resource

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/weaveworks/wksctl/pkg/plan"
)

const (
	// ServiceInactive is a non-started service.
	ServiceInactive = "inactive"
	// ServiceActivating is a starting service.
	ServiceActivating = "activating"
	// ServiceActive is a started service.
	ServiceActive = "active"
	// ServiceFailed is a service that failed to start
	ServiceFailed = "failed"
)

// Service represents a systemd service.
type Service struct {
	// Name of the systemd unit.
	Name string `structs:"name"`
	// Status is the desired service status. Only "active" or "inactive" are valid
	// input.
	Status string `structs:"status"`
	// Whether the service is enabled (systemctl enable) or not.
	Enabled bool `structs:"enabled"`
}

var _ plan.Resource = plan.RegisterResource(&Service{})

// State implements plan.Resource.
func (p *Service) State() plan.State {
	return toState(p)
}

func systemd(r plan.Runner, format string, args ...interface{}) (string, error) {
	return r.RunCommand(fmt.Sprintf("systemctl "+format, args...), nil)
}

// QueryState implements plan.Resource.
func (p *Service) QueryState(r plan.Runner) (plan.State, error) {
	// See https://bugzilla.redhat.com/show_bug.cgi?id=1073481#c11
	output, err := systemd(r, "show %s -p ActiveState", p.Name)
	if err != nil {
		return plan.EmptyState, err
	}
	status := keyval(output, "ActiveState")
	if status == "" {
		return plan.EmptyState, errors.Wrapf(err, "service %s: query: could not query active state", p.Name)
	}

	output, err = systemd(r, "is-enabled %s", p.Name)
	// is-enabled exits with non-zero status when the unit is disabled.
	if err != nil && line(output) != "disabled" {
		return plan.EmptyState, errors.Wrapf(err, "service %s: query: could not query enabled state", p.Name)
	}
	enabled := line(output) == "enabled"

	service := Service{
		Name:    p.Name,
		Status:  status,
		Enabled: enabled,
	}
	return service.State(), nil
}

// Apply implements plan.Resource.
func (p *Service) Apply(r plan.Runner, diff plan.Diff) (bool, error) {
	var err error
	var output string

	current := diff.CurrentState

	// Enabled
	if !current.Bool("enabled") && p.Enabled {
		output, err = systemd(r, "enable %s", p.Name)
	} else if current.Bool("enabled") && !p.Enabled {
		output, err = systemd(r, "disable %s", p.Name)
	}
	if err != nil {
		return false, fmt.Errorf("%s: %s", output, err.Error())
	}

	// Active
	// XXX: We need to think about what happens when a unit is in failed status
	// (current["status"] is "failed").
	if p.Status == ServiceActive && current.String("status") == "inactive" {
		output, err = systemd(r, "start %s", p.Name)
	} else if p.Status == ServiceInactive && current.String("status") != "inactive" {
		output, err = systemd(r, "stop %s", p.Name)
	}
	if err != nil {
		return false, fmt.Errorf("%s: %s", output, err.Error())
	}

	return true, nil
}

// Undo implements plan.Resource
func (p *Service) Undo(r plan.Runner, current plan.State) error {
	if current.Bool("enabled") == true {
		output, err := systemd(r, "disable %s", p.Name)
		if err != nil {
			if strings.Contains(output, "not loaded") {
				return nil
			}
			return fmt.Errorf("%s: %s", output, err.Error())
		}
	}

	if current.String("status") != "inactive" {
		output, err := systemd(r, "stop %s", p.Name)
		if err != nil {
			if strings.Contains(output, "not loaded") {
				return nil
			}
			return fmt.Errorf("%s: %s", output, err.Error())
		}
	}

	return nil
}
