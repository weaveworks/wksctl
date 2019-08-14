package resource

import (
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
	"github.com/weaveworks/wksctl/test/container/images"
	"github.com/weaveworks/wksctl/test/container/testutils"

	"github.com/stretchr/testify/assert"
)

func TestFile(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	srcPath := "testdata/daemon.json"
	file := &resource.File{
		Source:      srcPath,
		Destination: "/this/dir/does/not/exist/daemon.json",
	}

	emptyDiff := plan.EmptyDiff()

	// File shouldn't exist just yet.
	testutils.AssertEmptyState(t, file, r)

	// Go create that file and check that it exists and has the right content.
	_, err := file.Apply(r, emptyDiff)
	assert.NoError(t, err)

	remoteContent, err := r.RunCommand(fmt.Sprintf("cat %q", file.Destination), nil)
	assert.NoError(t, err)

	localContent, err := ioutil.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("ReadFile %q failed: %v", srcPath, err)
	}

	assert.Equal(t, string(localContent), remoteContent)

	// Query the state and ensures it's consistent.
	realizedState, err := file.QueryState(r)
	assert.NoError(t, err)
	assert.Equal(t, realizedState, file.State())

	// Undo and check the file isn't there.
	err = file.Undo(r, realizedState)
	assert.NoError(t, err)

}
