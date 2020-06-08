package plan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuilder(t *testing.T) {
	b := NewBuilder()
	b.AddResource(
		"rpm:docker",
		&testResource{ID: "rpm:docker"},
	).AddResource(
		"service:docker",
		&testResource{ID: "service:docker"},
		DependOn("rpm:docker", "file:daemon.json"),
	).AddResource(
		"file:daemon.json",
		&testResource{ID: "file:daemon.json"},
		DependOn("rpm:docker"),
	)
	assert.Equal(t, 0, len(b.Errors()))

	plan, err := b.Plan()
	assert.NoError(t, err)
	sorted, ok := plan.graph.Toposort()
	assert.True(t, ok)
	assert.Equal(t, []string{"rpm:docker", "file:daemon.json", "service:docker"}, sorted)
}

func TestAddResourceFrom(t *testing.T) {
	resources := map[string]Resource{
		"rpm:docker":     &testResource{ID: "rpm:docker"},
		"service:docker": &testResource{ID: "service:docker"},
	}

	b := NewBuilder()
	b.AddResourceFrom("rpm:docker", resources)
	b.AddResourceFrom("service:docker", resources, DependOn("rpm:docker", "file:daemon.json"))
	assert.Equal(t, 0, len(b.Errors()))

	// missing resource
	b.AddResourceFrom("rpm:k8s", resources)
	assert.Equal(t, 1, len(b.Errors()))
	errstr := b.Errors()[0].Error()
	assert.Equal(t, "resource id rpm:k8s not found in resources", errstr)
}

func makeInvalidBuilder() *Builder {
	b := NewBuilder()
	b.AddResource(
		"rpm:docker",
		&testResource{ID: "rpm:docker"},
	).AddResource(
		"service:docker",
		&testResource{ID: "service:docker"},
		DependOn("rpm:docker2", "file:daemon2.json"),
	).AddResource(
		"file:daemon.json",
		&testResource{ID: "file:daemon.json"},
	)
	return b
}

func checkInvalidBuilderErrors(t *testing.T, b *Builder) {
	assert.Equal(t, 2, len(b.Errors()))
	errstr1 := b.Errors()[0].Error()
	errstr2 := b.Errors()[1].Error()
	assert.True(t,
		strings.Contains(errstr1, "rpm:docker2") ||
			strings.Contains(errstr1, "file:daemon2.json"))
	assert.True(t,
		strings.Contains(errstr2, "rpm:docker2") ||
			strings.Contains(errstr2, "file:daemon2.json"))
}

func TestInvalidBuilder(t *testing.T) {
	b := makeInvalidBuilder()
	_, _ = b.Plan()
	checkInvalidBuilderErrors(t, b)

	// run it again to make sure we don't create duplicate errors
	b = makeInvalidBuilder()
	_, _ = b.Plan()
	checkInvalidBuilderErrors(t, b)
}
