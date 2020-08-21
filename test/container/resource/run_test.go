package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/object"
	"github.com/weaveworks/wksctl/test/container/images"
)

const runScript = `
for i in s u c c e s s; do
  echo -n $i
done
echo
`

func TestRun(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	run := &resource.Run{
		Script: object.String(runScript),
	}

	emptyDiff := plan.EmptyDiff()
	_, err := run.Apply(r, emptyDiff)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(r.Operations()))
	assert.Equal(t, "success\n", r.Operation(-1).Output)
}
