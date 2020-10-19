package ssh

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/test/container/images"
	"github.com/weaveworks/wksctl/test/container/testutils"
)

// Expect /etc/nologin content not to pollute SSH RunCommand() output.
// This is a regression test for bug #431.
func TestNotLoginShell(t *testing.T) {
	r, closer := NewRunnerForTest(t, images.CentOS7)
	defer closer()

	gotOutSetup, err := r.RunCommand(context.Background(), "echo 'this text should not appear in the output' > /etc/nologin", nil)
	assert.NoError(t, err)
	assert.Equal(t, "", gotOutSetup)

	client := testutils.ConnectSSH(t, r.Runner.(*testutils.FootlooseRunner))

	gotOutHello, err := client.RunCommand(context.Background(), "echo hello", nil)
	assert.NoError(t, err)
	assert.Equal(t, "hello\n", gotOutHello)
}
