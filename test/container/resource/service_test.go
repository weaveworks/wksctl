package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/wksctl/test/container/images"
)

func TestService(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	service := &resource.Service{
		Name:    "httpd",
		Enabled: true,
		Status:  "active",
	}

	// Ensure the service isn't started when this tests begins.
	startingState, err := service.QueryState(r)
	startingDiff := plan.Diff{
		CurrentState:    startingState,
		InvalidatedDeps: []plan.Resource{}}

	assert.NoError(t, err)
	assert.Equal(t, "httpd", startingState.String("name"))
	assert.Equal(t, false, startingState.Bool("enabled"))
	assert.Equal(t, "inactive", startingState.String("status"))

	// Apply the desired state.
	_, err = service.Apply(r, startingDiff)
	assert.NoError(t, err)

	// Verify the state is correctly applied.
	realizedState, err := service.QueryState(r)
	assert.NoError(t, err)
	assert.Equal(t, "httpd", realizedState.String("name"))
	assert.Equal(t, true, realizedState.Bool("enabled"))
	assert.Equal(t, "active", realizedState.String("status"))

	// Verify that, if we apply again, no command will actually be issued.
	realizedDiff := plan.Diff{
		CurrentState:    realizedState,
		InvalidatedDeps: []plan.Resource{}}

	r.ResetOperations()
	_, err = service.Apply(r, realizedDiff)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(r.Operations()))

	// Undo the install.
	err = service.Undo(r, realizedState)
	assert.NoError(t, err)
	undoState, err := service.QueryState(r)
	assert.NoError(t, err)
	assert.Equal(t, startingState, undoState)
}
