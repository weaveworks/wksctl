package resource

import (
	"testing"

	"github.com/weaveworks/wksctl/test/container/testutils"
)

var sshPort = testutils.PortAllocator{Next: 2222}

func NewRunnerForTest(t *testing.T, image string) (*testutils.TestRunner, func()) {
	return testutils.MakeFootlooseTestRunner(t, image, sshPort.Allocate())
}
