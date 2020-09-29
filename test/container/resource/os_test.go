package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/wksctl/test/container/images"
)

const (
	uuidRegexp      = `[a-fA-F0-9]{6}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`
	machineidRegexp = `[a-f0-9]{32}`
)

func TestOS(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	os, err := os.Identify(context.Background(), r.Runner)
	assert.NoError(t, err)

	resOs := &resource.OS{}
	// os.apply shoud force a query and update of state.
	emptyDiff := plan.EmptyDiff()
	_, err = resOs.Apply(context.Background(), r, emptyDiff)

	assert.NoError(t, err)
	assert.Equal(t, "centos", os.Name)
	assert.Regexp(t, machineidRegexp, resOs.MachineID)
	assert.Regexp(t, uuidRegexp, resOs.SystemUUID)
}
