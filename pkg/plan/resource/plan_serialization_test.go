package resource

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/plan"
)

// Tests real Resources in the main 'resource' package
func TestPlanToJSON(t *testing.T) {
	b := plan.NewBuilder()
	b.AddResource("rpm:foo", &RPM{Name: "foo", Version: "2"})
	b.AddResource("service:bar", &Service{Name: "bar", Status: "OK", Enabled: true}, plan.DependOn("rpm:foo"))
	b.AddResource("file:baz", &File{Source: "/tmp/x", Destination: "/etc/y"}, plan.DependOn("service:bar", "rpm:foo"))

	pin, err := b.Plan()
	assert.NoError(t, err)
	pout, err := plan.NewPlanFromJSON(strings.NewReader(pin.ToJSON()))
	assert.NoError(t, err)
	assert.True(t, plan.EqualPlans(pin, pout))
}
