package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/plan"
)

func TestToState(t *testing.T) {
	rpm := &RPM{
		Name:    "make",
		Version: "3.83",
	}
	expected := plan.State(map[string]interface{}{
		"name":    "make",
		"version": "3.83",
	})
	assert.Equal(t, expected, toState(rpm))
	assert.Equal(t, expected, rpm.State())
}
