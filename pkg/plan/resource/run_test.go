package resource

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/utilities/object"
)

func TestRunAndUndo(t *testing.T) {
	ctx := context.Background()
	dir, err := ioutil.TempDir("", "run-test")
	assert.NoError(t, err)
	filename := filepath.Join(dir, "foo")
	res := &Run{
		Script:     object.String("touch " + filename),
		UndoScript: object.String("rm " + filename),
	}

	runner := &plan.LocalRunner{}
	_, err = os.Stat(filename)
	assert.True(t, os.IsNotExist(err))

	val, err := res.Apply(ctx, runner, plan.EmptyDiff())
	assert.True(t, val)
	assert.NoError(t, err)
	assert.FileExists(t, filename)

	err = res.Undo(ctx, runner, plan.EmptyState)
	assert.NoError(t, err)
	_, err = os.Stat(filename)
	assert.True(t, os.IsNotExist(err))
}
