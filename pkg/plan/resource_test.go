package plan

import (
	"context"
	"errors"
	"reflect"

	"k8s.io/apimachinery/pkg/util/uuid"
)

// base can be embedded into a struct to provide a default implementation of
// plan.Resource.
type testResource struct {
	ID                         string
	DesiredState               State
	StateValue                 State
	QueryShouldError           bool
	QueryShouldErrorAfterApply bool
	ApplyShouldError           bool
	UndoShouldError            bool
	StatesShouldNotMatch       bool
	ApplyShouldNotFix          bool
	ApplyShouldNotPropagate    bool
}

var _ Resource = RegisterResource(&testResource{})
var UnmatchableState = State(map[string]interface{}{"statedata": uuid.NewUUID()})

// State implements Resource.
func (r *testResource) State() State {
	if r.StatesShouldNotMatch {
		return UnmatchableState
	}

	if r.DesiredState == nil {
		return EmptyState
	}

	return r.DesiredState
}

// QueryState implements Resource.
func (r *testResource) QueryState(_ context.Context, runner Runner) (State, error) {
	if r.QueryShouldError {
		return EmptyState, errors.New("Could not query state")
	}

	return r.StateValue, nil
}

// Apply implements Resource.
func (r *testResource) Apply(_ context.Context, runner Runner, diff Diff) (bool, error) {
	if r.ApplyShouldError {
		return false, errors.New("Apply failed")
	}

	invalid := !reflect.DeepEqual(diff.CurrentState, r.State())

	if r.QueryShouldErrorAfterApply {
		r.QueryShouldError = true
	}
	if !r.ApplyShouldNotFix {
		r.StatesShouldNotMatch = false
		if diff.CurrentState != nil {
			r.DesiredState = diff.CurrentState
		} else {
			r.DesiredState = r.StateValue
		}
	}

	if invalid {
		if r.ApplyShouldNotFix {
			return false, errors.New("Apply failed")
		}
		return !r.ApplyShouldNotPropagate, nil
	}

	return !r.ApplyShouldNotPropagate, nil
}

// Undo implements Resource.
func (r *testResource) Undo(_ context.Context, runner Runner, current State) error {
	if r.UndoShouldError {
		return errors.New("Undo failed")
	}

	return nil
}
