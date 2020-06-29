package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
	"github.com/weaveworks/wksctl/test/container/images"
)

const (
	uuid_regexp      = `[a-fA-F0-9]{6}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`
	machineid_regexp = `[a-f0-9]{32}`
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
	assert.Regexp(t, machineid_regexp, os.MachineID)
	assert.Regexp(t, uuid_regexp, os.SystemUUID)
}
