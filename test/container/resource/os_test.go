package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
	"github.com/weaveworks/wksctl/test/container/images"
)

const (
	uuidRegexp      = `[a-fA-F0-9]{6}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`
	machineidRegexp = `[a-f0-9]{32}`
)

func TestOS(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	os := &resource.OS{}
	// os.apply shoud force a query and update of state.
	emptyDiff := plan.EmptyDiff()
	_, err := os.Apply(r, emptyDiff)

	assert.NoError(t, err)
	assert.Equal(t, "centos", os.Name)
	assert.Equal(t, "7", os.Version)
	assert.Regexp(t, machineidRegexp, os.MachineID)
	assert.Regexp(t, uuidRegexp, os.SystemUUID)
}
