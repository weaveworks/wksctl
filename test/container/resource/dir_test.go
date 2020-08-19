package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/object"
	"github.com/weaveworks/wksctl/test/container/images"
)

func TestDirRecursive(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	tmpDir := &resource.Dir{
		Path:            object.String("/tmp/dir-test"),
		RecursiveDelete: true,
	}

	emptyDiff := plan.EmptyDiff()

	// Check that the directory does not exist.
	runCmdOrFail(t, r, "[ ! -e /tmp/dir-test ]")

	// Create a directory.
	_, err := tmpDir.Apply(r, emptyDiff)
	assert.NoError(t, err, "tmpDir.Apply")

	// Check that the directory exists.
	runCmdOrFail(t, r, "[ -d /tmp/dir-test ]")

	// Create a file in that directory.
	runCmdOrFail(t, r, "echo potato > /tmp/dir-test/somefile")

	// Check that the directory exists.
	runCmdOrFail(t, r, "[ -f /tmp/dir-test/somefile ]")

	// Delete the directory.
	assert.NoError(t, tmpDir.Undo(r, tmpDir.State()))

	// Check that the directory does not exist.
	runCmdOrFail(t, r, "[ ! -e /tmp/dir-test ]")
}

func TestDirNotRecursive(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	tmpDir := &resource.Dir{
		Path: object.String("/tmp/dir-test"),
	}

	emptyDiff := plan.EmptyDiff()

	// Check that the directory does not exist.
	runCmdOrFail(t, r, "[ ! -e /tmp/dir-test ]")

	// Create a directory.
	_, err := tmpDir.Apply(r, emptyDiff)
	assert.NoError(t, err, "tmpDir.Apply")

	// Check that the directory exists.
	runCmdOrFail(t, r, "[ -d /tmp/dir-test ]")

	// Create a file in that directory.
	runCmdOrFail(t, r, "echo potato > /tmp/dir-test/somefile")

	// Check that the directory exists.
	runCmdOrFail(t, r, "[ -f /tmp/dir-test/somefile ]")

	// Delete the directory.
	assert.NoError(t, tmpDir.Undo(r, tmpDir.State()))

	// Check that the directory and the file both exist.
	runCmdOrFail(t, r, "[ -d /tmp/dir-test ]")
	runCmdOrFail(t, r, "[ -f /tmp/dir-test/somefile ]")
}

func runCmdOrFail(t *testing.T, r plan.Runner, cmd string) {
	_, err := r.RunCommand(cmd, nil)
	assert.NoError(t, err, cmd)
}
