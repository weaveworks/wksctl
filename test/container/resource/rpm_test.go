package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
	"github.com/weaveworks/wksctl/test/container/images"
	"github.com/weaveworks/wksctl/test/container/testutils"
)

func TestRPM(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()
	emptyDiff := plan.EmptyDiff()

	// First, make isn't installed.
	p := &resource.RPM{
		Name: "make",
	}

	testutils.AssertEmptyState(t, p, r)

	// Install make.
	_, err := p.Apply(r, emptyDiff)
	assert.NoError(t, err)

	// Verify make is installed.
	installedState, err := p.QueryState(r)
	assert.NoError(t, err)
	assert.Equal(t, "make", installedState["name"])
	assert.NotZero(t, installedState["version"])
	assert.NotZero(t, installedState["release"])

	// Verify that, if we apply again, no command will actually be issued.
	r.ResetOperations()
	installedDiff := plan.Diff{
		CurrentState:    installedState,
		InvalidatedDeps: []plan.Resource{}}
	_, err = p.Apply(r, installedDiff)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(r.Operations()))

	// Undo the install.
	err = p.Undo(r, installedState)
	assert.NoError(t, err)
	testutils.AssertEmptyState(t, p, r)
}
