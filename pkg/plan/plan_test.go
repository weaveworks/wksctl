package plan

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/test/plan/testutils"
)

// A non-pointer Resource to test that reflective instance creation works correctly
type nonPointerResource struct {
	Dummy string
}

var _ Resource = RegisterResource(nonPointerResource{})

// State implements plan.Resource.
func (npr nonPointerResource) State() State {
	return EmptyState
}

// QueryState implements plan.Resource.
func (npr nonPointerResource) QueryState(runner Runner) (State, error) {
	return EmptyState, nil
}

// Apply implements Resource.
func (npr nonPointerResource) Apply(runner Runner, diff Diff) (bool, error) {
	return true, nil
}

// Undo implements Resource.
func (npr nonPointerResource) Undo(runner Runner, current State) error {
	return nil
}

// Translate a Plan into JSON and back

// Tests that Resources outside of the main 'resource' package work
// correctly and that non-pointer Resources work
func TestPlanToJSON(t *testing.T) {
	b := NewBuilder()
	b.AddResource("test:foo", &testResource{ID: "test:foo"})
	b.AddResource("test:bar", &testResource{ID: "test:bar"}, DependOn("test:foo"))
	b.AddResource("test:baz", &testResource{ID: "test:baz"}, DependOn("test:bar"))
	b.AddResource("test:quux", &testResource{ID: "test:quux"}, DependOn("test:baz"))
	b.AddResource("npr:erf", nonPointerResource{"npr:erf"}, DependOn("test:foo", "test:bar"))

	pin, err := b.Plan()
	assert.NoError(t, err)
	pout, err := NewPlanFromJSON(strings.NewReader(pin.ToJSON()))
	assert.NoError(t, err)
	assert.True(t, EqualPlans(pin, pout))
}

func TestNestedPlanToJSON(t *testing.T) {
	b := NewBuilder()
	sub := NewBuilder()
	sub.AddResource("test:foo", &testResource{ID: "test:foo"})
	sub.AddResource("test:bar", &testResource{ID: "test:bar"}, DependOn("test:foo"))
	sub.AddResource("test:baz", &testResource{ID: "test:baz"}, DependOn("test:bar"))
	sub.AddResource("test:quux", &testResource{ID: "test:quux"}, DependOn("test:baz"))
	sub.AddResource("npr:erf", nonPointerResource{"npr:erf"}, DependOn("test:foo", "test:bar"))
	psub, err := sub.Plan()
	assert.NoError(t, err)
	b.AddResource("test:subplan", &psub)
	pin, err := b.Plan()
	assert.NoError(t, err)
	pout, err := NewPlanFromJSON(strings.NewReader(pin.ToJSON()))
	assert.NoError(t, err)
	assert.True(t, EqualPlans(pin, pout))
}

// Tests that Resources outside of the main 'resource' package work
// correctly and that non-pointer Resources work
func TestPlanToDOT(t *testing.T) {
	b := NewBuilder()
	b.AddResource("foo", &testResource{ID: "foo"})
	b.AddResource("bar", &testResource{ID: "bar"}, DependOn("foo"))
	b.AddResource("baz", &testResource{ID: "baz"}, DependOn("bar"))
	b.AddResource("quux", &testResource{ID: "quux"}, DependOn("baz"))
	b.AddResource("erf", nonPointerResource{"erf"}, DependOn("foo", "bar"))
	b.AddResource("alone", nonPointerResource{"alone"})

	testcases := []struct {
		exp string
		msg string
	}{
		{"\t\"alone\"", "Didn't find resource w/o deps"},
		{"\t\"foo\"", "should be w/o deps"},
		{"\t\"baz\" -> \"quux\" [style=bold color=blue]\n", "quux should depend on bar"},
		{"\t\"bar\" -> \"erf\" [style=bold color=blue]\n", "erf should depend on bar"},
		{"\t\"foo\" -> \"erf\" [style=bold color=blue]\n", "erf should depend on foo"},
	}
	p, err := b.Plan()
	assert.NoError(t, err)
	s := p.ToDOT()
	for _, tc := range testcases {
		assert.True(t, strings.Index(s, tc.exp) > 0, tc.msg)
	}
}

// Test invalidation behavior using testResource as a mock
func TestSimpleDepEverythingValid(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("dependee", &testResource{ID: "dependee"})
	b.AddResource("dependent", &testResource{ID: "dependent"}, DependOn("dependee"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	resourceValidity := p.EnsureResourceValid("dependent", r)
	assert.Equal(
		t,
		ValidityTree{
			ResourceID:     "dependent",
			ValidityStatus: Valid,
			Reason:         None,
			Dependencies: []ValidityTree{
				{
					ResourceID:     "dependee",
					ValidityStatus: Valid,
					Reason:         None,
					Dependencies:   []ValidityTree{},
				}}},
		resourceValidity)
}

func TestMultiLevelMultiDepEverythingValid(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("bottom-1", &testResource{ID: "bottom-1"})
	b.AddResource("bottom-2", &testResource{ID: "bottom-2"})
	b.AddResource("middle-1", &testResource{ID: "middle-1"}, DependOn("bottom-1"))
	b.AddResource("middle-2", &testResource{ID: "middle-2"}, DependOn("bottom-1", "bottom-2"))
	b.AddResource("top", &testResource{ID: "top"}, DependOn("bottom-2", "middle-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	resourceValidity := p.EnsureResourceValid("top", r)
	bot1 := ValidityTree{
		ResourceID:     "bottom-1",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{}}
	bot2 := ValidityTree{
		ResourceID:     "bottom-2",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{}}
	mid1 := ValidityTree{
		ResourceID:     "middle-1",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{bot1}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{bot1, bot2}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{bot2, mid1, mid2}}
	assert.Equal(t, top, resourceValidity)
}

func TestSimpleDepCorrectablyInvalid(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("dependee", &testResource{ID: "dependee", StatesShouldNotMatch: true})
	b.AddResource("dependent", &testResource{ID: "dependent"}, DependOn("dependee"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	resourceValidity := p.EnsureResourceValid("dependent", r)
	assert.Equal(
		t,
		ValidityTree{
			ResourceID:     "dependent",
			ValidityStatus: Valid,
			Reason:         None,
			Updated:        true,
			Dependencies: []ValidityTree{
				{
					ResourceID:     "dependee",
					ValidityStatus: Valid,
					Reason:         None,
					Updated:        true,
					Dependencies:   []ValidityTree{},
				}}},
		resourceValidity)
}

func TestSimpleDepUncorrectablyInvalid(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("dependee", &testResource{ID: "dependee", StatesShouldNotMatch: true, ApplyShouldNotFix: true})
	b.AddResource("dependent", &testResource{ID: "dependent"}, DependOn("dependee"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	resourceValidity := p.EnsureResourceValid("dependent", r)
	assert.Equal(
		t,
		ValidityTree{
			ResourceID:     "dependent",
			ValidityStatus: Invalid,
			Reason:         DependencyInvalid,
			Dependencies: []ValidityTree{
				{
					ResourceID:     "dependee",
					ValidityStatus: Invalid,
					Reason:         ApplyError,
					ObservedError:  "Apply failed",
					Dependencies:   []ValidityTree{},
				}}},
		resourceValidity)
}

func TestMultiLevelMultiDepOneInvalid(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("bottom-1", &testResource{ID: "bottom-1", StatesShouldNotMatch: true, ApplyShouldNotFix: true})
	b.AddResource("bottom-2", &testResource{ID: "bottom-2"})
	b.AddResource("middle-1", &testResource{ID: "middle-1"}, DependOn("bottom-1"))
	b.AddResource("middle-2", &testResource{ID: "middle-2"}, DependOn("bottom-1", "bottom-2"))
	b.AddResource("top", &testResource{ID: "top"}, DependOn("bottom-2", "middle-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	resourceValidity := p.EnsureResourceValid("top", r)
	bot1 := ValidityTree{
		ResourceID:     "bottom-1",
		ValidityStatus: Invalid,
		Reason:         ApplyError,
		ObservedError:  "Apply failed",
		Dependencies:   []ValidityTree{}}
	bot2 := ValidityTree{
		ResourceID:     "bottom-2",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{}}
	mid1 := ValidityTree{
		ResourceID:     "middle-1",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot1}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot1, bot2}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot2, mid1, mid2}}
	assert.Equal(t, top, resourceValidity)
}

func TestMultiLevelMultiDepOneInconclusive(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("bottom-1", &testResource{ID: "bottom-1", QueryShouldError: true})
	b.AddResource("bottom-2", &testResource{ID: "bottom-2"})
	b.AddResource("middle-1", &testResource{ID: "middle-1"}, DependOn("bottom-1"))
	b.AddResource("middle-2", &testResource{ID: "middle-2"}, DependOn("bottom-1", "bottom-2"))
	b.AddResource("top", &testResource{ID: "top"}, DependOn("bottom-2", "middle-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	resourceValidity := p.EnsureResourceValid("top", r)
	bot1 := ValidityTree{
		ResourceID:     "bottom-1",
		ValidityStatus: Inconclusive,
		Reason:         QueryError,
		ObservedError:  "Could not query state",
		Dependencies:   []ValidityTree{}}
	bot2 := ValidityTree{
		ResourceID:     "bottom-2",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{}}
	mid1 := ValidityTree{
		ResourceID:     "middle-1",
		ValidityStatus: Inconclusive,
		Reason:         DependencyInconclusive,
		Dependencies:   []ValidityTree{bot1}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Inconclusive,
		Reason:         DependencyInconclusive,
		Dependencies:   []ValidityTree{bot1, bot2}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Inconclusive,
		Reason:         DependencyInconclusive,
		Dependencies:   []ValidityTree{bot2, mid1, mid2}}
	assert.Equal(t, top, resourceValidity)
}

func TestMultiLevelMultiDepOneInconclusiveOneInvalid(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("bottom-1", &testResource{ID: "bottom-1", QueryShouldError: true})
	b.AddResource("bottom-2", &testResource{ID: "bottom-2", StatesShouldNotMatch: true, ApplyShouldNotFix: true})
	b.AddResource("middle-1", &testResource{ID: "middle-1"}, DependOn("bottom-1"))
	b.AddResource("middle-2", &testResource{ID: "middle-2"}, DependOn("bottom-1", "bottom-2"))
	b.AddResource("top", &testResource{ID: "top"}, DependOn("bottom-2", "middle-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	resourceValidity := p.EnsureResourceValid("top", r)
	bot1 := ValidityTree{
		ResourceID:     "bottom-1",
		ValidityStatus: Inconclusive,
		Reason:         QueryError,
		ObservedError:  "Could not query state",
		Dependencies:   []ValidityTree{}}
	bot2 := ValidityTree{
		ResourceID:     "bottom-2",
		ValidityStatus: Invalid,
		Reason:         ApplyError,
		ObservedError:  "Apply failed",
		Dependencies:   []ValidityTree{}}
	mid1 := ValidityTree{
		ResourceID:     "middle-1",
		ValidityStatus: Inconclusive,
		Reason:         DependencyInconclusive,
		Dependencies:   []ValidityTree{bot1}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot1, bot2}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot2, mid1, mid2}}
	assert.Equal(t, top, resourceValidity)
}

func TestMultiLevelMultiDepOneInconclusiveOneInvalidFixedOnError(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("bottom-1", &testResource{ID: "bottom-1", QueryShouldError: true})
	b.AddResource("bottom-2", &testResource{ID: "bottom-2", StatesShouldNotMatch: true})
	b.AddResource("middle-1", &testResource{ID: "middle-1"}, DependOn("bottom-1"))
	b.AddResource("middle-2", &testResource{ID: "middle-2"}, DependOn("bottom-1", "bottom-2"))
	b.AddResource("top", &testResource{ID: "top"}, DependOn("bottom-2", "middle-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	resourceValidity := p.EnsureResourceValid("top", r)
	bot1 := ValidityTree{
		ResourceID:     "bottom-1",
		ValidityStatus: Inconclusive,
		Reason:         QueryError,
		ObservedError:  "Could not query state",
		Dependencies:   []ValidityTree{}}
	bot2 := ValidityTree{
		ResourceID:     "bottom-2",
		ValidityStatus: Valid,
		Reason:         None,
		Updated:        true,
		Dependencies:   []ValidityTree{}}
	mid1 := ValidityTree{
		ResourceID:     "middle-1",
		ValidityStatus: Inconclusive,
		Reason:         DependencyInconclusive,
		Dependencies:   []ValidityTree{bot1}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Inconclusive,
		Reason:         DependencyInconclusive,
		Dependencies:   []ValidityTree{bot1, bot2}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Inconclusive,
		Reason:         DependencyInconclusive,
		Dependencies:   []ValidityTree{bot2, mid1, mid2}}
	assert.Equal(t, top, resourceValidity)
}

// All resources already at their desired states.
// Nothing gets applied but everything considered valid.
func TestStraightLinePlanApplyAllUpToDate(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("resource-1", &testResource{ID: "resource-1"})
	b.AddResource("resource-2", &testResource{ID: "resource-2"}, DependOn("resource-1"))
	b.AddResource("resource-3", &testResource{ID: "resource-3"}, DependOn("resource-2"))
	b.AddResource("resource-4", &testResource{ID: "resource-4"}, DependOn("resource-3"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	diff := Diff{
		CurrentState:    EmptyState,
		InvalidatedDeps: []Resource{}}
	propagate, err := p.Apply(r, diff)
	assert.NoError(t, err)
	assert.False(t, propagate)
}

// No resources already at their desired states.
// Everything gets applied and updates its state.
func TestStraightLinePlanApplyAllValid(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder()
	b.AddResource("resource-1",
		&testResource{
			ID:                   "resource-1",
			StatesShouldNotMatch: true})
	b.AddResource("resource-2",
		&testResource{
			ID:                   "resource-2",
			StatesShouldNotMatch: true}, DependOn("resource-1"))
	b.AddResource("resource-3",
		&testResource{
			ID:                   "resource-3",
			StatesShouldNotMatch: true}, DependOn("resource-2"))
	b.AddResource("resource-4",
		&testResource{
			ID:                   "resource-4",
			StatesShouldNotMatch: true}, DependOn("resource-3"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	diff := Diff{
		CurrentState:    EmptyState,
		InvalidatedDeps: []Resource{}}
	propagate, err := p.Apply(r, diff)
	assert.True(t, propagate)
	assert.NoError(t, err)
}

// Last resource fails to apply; generate an error and note the invalid state
func TestStraightLinePlanLastApplyError(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder("plan-1")
	b.AddResource("resource-1", &testResource{ID: "resource-1"})
	b.AddResource("resource-2", &testResource{ID: "resource-2"}, DependOn("resource-1"))
	b.AddResource("resource-3",
		&testResource{
			ID:                   "resource-3",
			StatesShouldNotMatch: true,
			ApplyShouldError:     true}, DependOn("resource-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	diff := Diff{
		CurrentState:    EmptyState,
		InvalidatedDeps: []Resource{}}
	propagate, err := p.Apply(r, diff)
	assert.Error(t, err)
	assert.False(t, propagate)
	res1 := ValidityTree{
		ResourceID:     "resource-1",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{}}
	res2 := ValidityTree{
		ResourceID:     "resource-2",
		ValidityStatus: Valid,
		Reason:         None,
		Dependencies:   []ValidityTree{res1}}
	res3 := ValidityTree{
		ResourceID:     "resource-3",
		ValidityStatus: Invalid,
		Reason:         ApplyError,
		ObservedError:  "Apply failed",
		Dependencies:   []ValidityTree{res2}}
	plan := ValidityTree{
		ResourceID:     "plan-1",
		ValidityStatus: Invalid,
		Reason:         ChildInvalid,
		ObservedError:  "Apply failed because a child failed",
		Dependencies:   []ValidityTree{},
		Children:       []ValidityTree{res3}}
	assert.Equal(t, plan, err)
}

// Bottom dependency fails
func TestMultiLevelMultiDepApplyOneInvalid(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder("plan-1")
	b.AddResource("bottom-1",
		&testResource{ID: "bottom-1",
			StatesShouldNotMatch: true,
			ApplyShouldError:     true})
	b.AddResource("middle-1", &testResource{ID: "middle-1"}, DependOn("bottom-1"))
	b.AddResource("middle-2", &testResource{ID: "middle-2"}, DependOn("bottom-1"))
	b.AddResource("top", &testResource{ID: "top"}, DependOn("bottom-1", "middle-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	diff := Diff{
		CurrentState:    EmptyState,
		InvalidatedDeps: []Resource{}}
	propagate, err := p.Apply(r, diff)
	assert.False(t, propagate)
	assert.Error(t, err)
	bot1 := ValidityTree{
		ResourceID:     "bottom-1",
		ValidityStatus: Invalid,
		Reason:         ApplyError,
		ObservedError:  "Apply failed",
		Dependencies:   []ValidityTree{}}
	mid1 := ValidityTree{
		ResourceID:     "middle-1",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot1}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot1}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot1, mid1, mid2}}
	plan := ValidityTree{
		ResourceID:     "plan-1",
		ValidityStatus: Invalid,
		Reason:         ChildInvalid,
		ObservedError:  "Apply failed because a child failed",
		Dependencies:   []ValidityTree{},
		Children:       []ValidityTree{top}}
	assert.Equal(t, plan, err)
}

// Cached state passed in, no failure
func TestMultiLevelMultiDepApplyCachedState(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder("plan-1")
	b.AddResource("bottom-1",
		&testResource{
			ID:                   "bottom-1",
			StatesShouldNotMatch: true})
	b.AddResource("middle-1",
		&testResource{
			ID:                   "middle-1",
			StatesShouldNotMatch: true}, DependOn("bottom-1"))
	b.AddResource("middle-2",
		&testResource{
			ID:                   "middle-2",
			StatesShouldNotMatch: true}, DependOn("bottom-1"))
	b.AddResource("top",
		&testResource{
			ID:                   "top",
			StatesShouldNotMatch: true}, DependOn("bottom-1", "middle-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	diff := Diff{
		CurrentState: State(map[string]interface{}{
			"bottom-1": State(map[string]interface{}{"bstate": 2}),
			"middle-1": State(map[string]interface{}{"m1state": 3}),
			"middle-2": State(map[string]interface{}{"m2state": 4}),
			"top":      State(map[string]interface{}{"tstate": 5})}),
		InvalidatedDeps: []Resource{}}
	propagate, err := p.Apply(r, diff)
	assert.NoError(t, err)
	assert.True(t, propagate)
}

// Cached state passed in, one failure
func TestMultiLevelMultiDepApplyCachedStateWithFailure(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder("plan-1")
	b.AddResource("bottom-1",
		&testResource{
			ID:                   "bottom-1",
			StatesShouldNotMatch: true})
	b.AddResource("middle-1",
		&testResource{
			ID:                   "middle-1",
			StatesShouldNotMatch: true}, DependOn("bottom-1"))
	b.AddResource("middle-2",
		&testResource{
			ID:                   "middle-2",
			StatesShouldNotMatch: true,
			ApplyShouldError:     true}, DependOn("bottom-1"))
	b.AddResource("top",
		&testResource{
			ID:                   "top",
			StatesShouldNotMatch: true}, DependOn("bottom-1", "middle-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	diff := Diff{
		CurrentState: State(map[string]interface{}{
			"bottom-1": State(map[string]interface{}{"bstate": 2}),
			"middle-1": State(map[string]interface{}{"m1state": 3}),
			"middle-2": State(map[string]interface{}{"m2state": 4}),
			"top":      State(map[string]interface{}{"tstate": 5})}),
		InvalidatedDeps: []Resource{}}
	propagate, err := p.Apply(r, diff)
	assert.Error(t, err)
	assert.False(t, propagate)
	bot1 := ValidityTree{
		ResourceID:     "bottom-1",
		ValidityStatus: Valid,
		Reason:         None,
		Updated:        true,
		Dependencies:   []ValidityTree{}}
	mid1 := ValidityTree{
		ResourceID:     "middle-1",
		ValidityStatus: Valid,
		Reason:         None,
		Updated:        true,
		Dependencies:   []ValidityTree{bot1}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Invalid,
		Reason:         ApplyError,
		ObservedError:  "Apply failed",
		Dependencies:   []ValidityTree{bot1}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{bot1, mid1, mid2}}
	plan := ValidityTree{
		ResourceID:     "plan-1",
		ValidityStatus: Invalid,
		Reason:         ChildInvalid,
		ObservedError:  "Apply failed because a child failed",
		Dependencies:   []ValidityTree{},
		Children:       []ValidityTree{top}}
	assert.Equal(t, plan, err)
}

// Nested plan (plan as resource) that is valid
func TestNestedPlan(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder("plan-1")
	b2 := NewBuilder("plan-1-1")
	b2.AddResource("bottom-1",
		&testResource{
			ID:                   "bottom-1",
			StatesShouldNotMatch: true})
	b2.AddResource("middle-1",
		&testResource{
			ID:                   "middle-1",
			StatesShouldNotMatch: true}, DependOn("bottom-1"))
	p11, err := b2.Plan()
	assert.NoError(t, err)
	b.AddResource("plan-1-1", &p11)
	b.AddResource("middle-2",
		&testResource{
			ID:                   "middle-2",
			StatesShouldNotMatch: true,
			ApplyShouldError:     true}, DependOn("plan-1-1"))
	b.AddResource("top",
		&testResource{
			ID:                   "top",
			StatesShouldNotMatch: true}, DependOn("plan-1-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	diff := Diff{
		CurrentState: State(map[string]interface{}{
			"plan-1-1": State(map[string]interface{}{
				"bottom-1": State(map[string]interface{}{"bstate": 2}),
				"middle-1": State(map[string]interface{}{"m1state": 3})}),
			"middle-2": State(map[string]interface{}{"m2state": 4}),
			"top":      State(map[string]interface{}{"tstate": 5})}),
		InvalidatedDeps: []Resource{}}
	propagate, err := p.Apply(r, diff)
	assert.Error(t, err)
	assert.False(t, propagate)
	plan11 := ValidityTree{
		ResourceID:     "plan-1-1",
		ValidityStatus: Valid,
		Reason:         None,
		Updated:        true,
		Dependencies:   []ValidityTree{}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Invalid,
		Reason:         ApplyError,
		ObservedError:  "Apply failed",
		Dependencies:   []ValidityTree{plan11}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{mid2, plan11}}
	plan := ValidityTree{
		ResourceID:     "plan-1",
		ValidityStatus: Invalid,
		Reason:         ChildInvalid,
		ObservedError:  "Apply failed because a child failed",
		Dependencies:   []ValidityTree{},
		Children:       []ValidityTree{top}}
	assert.Equal(t, plan, err)
	fmt.Printf("ERR: %v, ETREE: %s\n", err, err.(ValidityTree).ToJSON())
}

// Nested plan (plan as resource) that is invalid
func TestInvalidNestedPlan(t *testing.T) {
	r := &testutils.MockRunner{Output: "", Err: nil}
	b := NewBuilder("plan-1")
	b2 := NewBuilder("plan-1-1")
	b2.AddResource("bottom-1",
		&testResource{
			ID:                   "bottom-1",
			StatesShouldNotMatch: true})
	b2.AddResource("middle-1",
		&testResource{
			ID:                   "middle-1",
			StatesShouldNotMatch: true,
			ApplyShouldError:     true}, DependOn("bottom-1"))
	p11, err := b2.Plan()
	assert.NoError(t, err)
	b.AddResource("plan-1-1", &p11)
	b.AddResource("middle-2",
		&testResource{
			ID:                   "middle-2",
			StatesShouldNotMatch: true,
			ApplyShouldError:     true}, DependOn("plan-1-1"))
	b.AddResource("top",
		&testResource{
			ID:                   "top",
			StatesShouldNotMatch: true}, DependOn("plan-1-1", "middle-2"))
	assert.Equal(t, 0, len(b.Errors()))
	p, err := b.Plan()
	assert.NoError(t, err)
	diff := Diff{
		CurrentState: State(map[string]interface{}{
			"plan-1-1": State(map[string]interface{}{
				"bottom-1": State(map[string]interface{}{"bstate": 2}),
				"middle-1": State(map[string]interface{}{"m1state": 3})}),
			"middle-2": State(map[string]interface{}{"m2state": 4}),
			"top":      State(map[string]interface{}{"tstate": 5})}),
		InvalidatedDeps: []Resource{}}
	propagate, err := p.Apply(r, diff)
	assert.Error(t, err)
	assert.False(t, propagate)
	bot1 := ValidityTree{
		ResourceID:     "bottom-1",
		ValidityStatus: Valid,
		Reason:         None,
		Updated:        true,
		Dependencies:   []ValidityTree{}}
	mid1 := ValidityTree{
		ResourceID:     "middle-1",
		ValidityStatus: Invalid,
		Reason:         ApplyError,
		ObservedError:  "Apply failed",
		Dependencies:   []ValidityTree{bot1}}
	plan11 := ValidityTree{
		ResourceID:     "plan-1-1",
		ValidityStatus: Invalid,
		Reason:         ApplyError,
		ObservedError:  "Apply failed because a child failed",
		Dependencies:   []ValidityTree{},
		Children:       []ValidityTree{mid1}}
	mid2 := ValidityTree{
		ResourceID:     "middle-2",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{plan11}}
	top := ValidityTree{
		ResourceID:     "top",
		ValidityStatus: Invalid,
		Reason:         DependencyInvalid,
		Dependencies:   []ValidityTree{mid2, plan11}}
	plan := ValidityTree{
		ResourceID:     "plan-1",
		ValidityStatus: Invalid,
		Reason:         ChildInvalid,
		ObservedError:  "Apply failed because a child failed",
		Dependencies:   []ValidityTree{},
		Children:       []ValidityTree{top}}
	assert.Equal(t, plan, err)
	fmt.Printf("PLAN: %v\n", plan.ToJSON())
	fmt.Printf("ERR: %v\nETREE: %s\n", err, err.(ValidityTree).ToJSON())
}
