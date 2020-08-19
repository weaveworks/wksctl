package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/wksctl/test/container/images"
	"github.com/weaveworks/wksctl/test/container/testutils"
)

func TestDeb(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.Ubuntu1804)
	defer closer()

	AssertNotInstalled(t, "nonexistent", r)

	AssertInstalled(t, "libc6", r)

	AssertNotInstalled(t, "busybox", r)
	InstallAndAssertSuccess(t, "busybox", "", r)
	AssertInstalled(t, "busybox", r)
	PurgeAndAssertSuccess(t, "busybox", r)
	AssertNotInstalled(t, "busybox", r)
}

func AssertNotInstalled(t *testing.T, name string, r plan.Runner) {
	testutils.AssertEmptyState(t, &resource.Deb{Name: name}, r)
}

func AssertInstalled(t *testing.T, name string, r plan.Runner) {
	res := resource.Deb{Name: name}
	installedState, err := res.QueryState(r)
	assert.NoError(t, err)
	assert.Equal(t, name, installedState["name"])
	assert.NotZero(t, installedState["suffix"])
}

func InstallAndAssertSuccess(t *testing.T, name, suffix string, r plan.Runner) {
	res := resource.Deb{Name: name, Suffix: suffix}
	prop, err := res.Apply(r, plan.EmptyDiff())
	assert.NoError(t, err)
	assert.True(t, prop)
}

func PurgeAndAssertSuccess(t *testing.T, name string, r plan.Runner) {
	res := resource.Deb{Name: name}
	err := res.Undo(r, res.State())
	assert.NoError(t, err)
}
