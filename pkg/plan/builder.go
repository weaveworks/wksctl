package plan

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/uuid"
)

// Builder is a plan builder.
type Builder struct {
	plan   *Plan
	errors []error
}

// NewBuilder creates a new Builder.
func NewBuilder(idSegments ...string) *Builder {
	p := newPlan()
	if len(idSegments) == 0 {
		p.id = fmt.Sprintf("plan-%s", uuid.NewUUID())
	} else {
		p.id = strings.Join(idSegments, "-")
	}
	return &Builder{plan: p}
}

type addOptions struct {
	deps []string
}

// DependOn expresses dependency between Resources.
func DependOn(dep string, deps ...string) func(*addOptions) {
	return func(o *addOptions) {
		o.deps = append(o.deps, dep)
		o.deps = append(o.deps, deps...)
	}
}

// AddResource adds a resource to the plan.
func (b *Builder) AddResource(id string, r Resource, options ...func(*addOptions)) *Builder {
	o := &addOptions{}
	for _, option := range options {
		option(o)
	}
	if err := b.plan.addResource(id, r, o); err != nil {
		b.errors = append(b.errors, err)
	}
	return b
}

func (b *Builder) checkIfValidResource(n string) {
	_, ok := b.plan.resources[n]
	if !ok {
		err := fmt.Errorf("graph node %s is not a valid resource", n)
		b.errors = append(b.errors, err)
	}
}

func (b *Builder) validateGraph() {
	for n := range b.plan.graph.nodes {
		b.checkIfValidResource(n)
	}
}

// Errors returns the errors that occurred during the build. The user is
// expected to check the return value of this function before using the Plan.
func (b *Builder) Errors() []error {
	return b.errors
}

// Plan returns the built Plan.
func (b *Builder) Plan() (Plan, error) {
	b.validateGraph()
	if len(b.errors) > 0 {
		var errs bytes.Buffer
		fmt.Fprintf(&errs, "Invalid plan:\n")
		for _, err := range b.Errors() {
			fmt.Fprintf(&errs, "  %s\n", err.Error())
		}
		return *b.plan, errors.New(errs.String())
	}
	return *b.plan, nil
}
