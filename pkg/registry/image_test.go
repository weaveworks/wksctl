package registry_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/registry"
)

func TestImageWithName(t *testing.T) {
	image, err := registry.NewImage("alpine")
	assert.NoError(t, err)
	assert.Equal(t, &registry.Image{
		Registry: "",
		User:     "",
		Name:     "alpine",
		Tag:      "",
	}, image)
	assert.Equal(t, "alpine", image.String())
}

func TestImageWithNameTag(t *testing.T) {
	image, err := registry.NewImage("golang:1.10")
	assert.NoError(t, err)
	assert.Equal(t, &registry.Image{
		Registry: "",
		User:     "",
		Name:     "golang",
		Tag:      "1.10",
	}, image)
	assert.Equal(t, "golang:1.10", image.String())
}

func TestImageWithUserName(t *testing.T) {
	image, err := registry.NewImage("foo/bar")
	assert.NoError(t, err)
	assert.Equal(t, &registry.Image{
		Registry: "",
		User:     "foo",
		Name:     "bar",
		Tag:      "",
	}, image)
	assert.Equal(t, "foo/bar", image.String())
}

func TestImageWithRegistryUserName(t *testing.T) {
	image, err := registry.NewImage("foo/bar/baz")
	assert.NoError(t, err)
	assert.Equal(t, &registry.Image{
		Registry: "foo",
		User:     "bar",
		Name:     "baz",
		Tag:      "",
	}, image)
	assert.Equal(t, "foo/bar/baz", image.String())
}

func TestImageWithHostUserNameTag(t *testing.T) {
	image, err := registry.NewImage("quay.io/weaveworks/wks:latest")
	assert.NoError(t, err)
	assert.Equal(t, &registry.Image{
		Registry: "quay.io",
		User:     "weaveworks",
		Name:     "wks",
		Tag:      "latest",
	}, image)
	assert.Equal(t, "quay.io/weaveworks/wks:latest", image.String())
}

func TestImageWithHostPortUserNameTag(t *testing.T) {
	image, err := registry.NewImage("localhost:5000/test/busybox:v1.2.3")
	assert.NoError(t, err)
	assert.Equal(t, &registry.Image{
		Registry: "localhost:5000",
		User:     "test",
		Name:     "busybox",
		Tag:      "v1.2.3",
	}, image)
	assert.Equal(t, "localhost:5000/test/busybox:v1.2.3", image.String())
}

func TestImageWithHostPortUserName(t *testing.T) {
	image, err := registry.NewImage("localhost:5000/test/busybox")
	assert.NoError(t, err)
	assert.Equal(t, &registry.Image{
		Registry: "localhost:5000",
		User:     "test",
		Name:     "busybox",
		Tag:      "",
	}, image)
	assert.Equal(t, "localhost:5000/test/busybox", image.String())
}

func TestInvalidImages(t *testing.T) {
	image, err := registry.NewImage("")
	assert.Nil(t, image)
	assert.Equal(t, errors.New("invalid image: ''"), err)

	image, err = registry.NewImage("    ")
	assert.Nil(t, image)
	assert.Equal(t, errors.New("invalid image: '    '"), err)

	image, err = registry.NewImage("a/b/c/d:e")
	assert.Nil(t, image)
	assert.Equal(t, errors.New("invalid image: 'a/b/c/d:e'"), err)

	image, err = registry.NewImage("a/b/c:d:e")
	assert.Nil(t, image)
	assert.Equal(t, errors.New("invalid image: 'a/b/c:d:e'"), err)
}

func TestCommandsToRetagAs(t *testing.T) {
	source := registry.Image{
		Registry: "quay.io",
		User:     "weaveworks",
		Name:     "wks",
		Tag:      "latest",
	}

	// Make a copy of the source struct:
	dest := source

	// Change the container image's "coordinates" as much or as little as required:

	dest.Registry = "acme.com:8443"
	assert.Equal(t, []string{
		"docker pull quay.io/weaveworks/wks:latest",
		"docker tag quay.io/weaveworks/wks:latest acme.com:8443/weaveworks/wks:latest",
		"docker push acme.com:8443/weaveworks/wks:latest",
	}, source.CommandsToRetagAs(dest))

	dest.User = "mono-repo"
	assert.Equal(t, []string{
		"docker pull quay.io/weaveworks/wks:latest",
		"docker tag quay.io/weaveworks/wks:latest acme.com:8443/mono-repo/wks:latest",
		"docker push acme.com:8443/mono-repo/wks:latest",
	}, source.CommandsToRetagAs(dest))

	dest.Name = "my-custom-wks"
	assert.Equal(t, []string{
		"docker pull quay.io/weaveworks/wks:latest",
		"docker tag quay.io/weaveworks/wks:latest acme.com:8443/mono-repo/my-custom-wks:latest",
		"docker push acme.com:8443/mono-repo/my-custom-wks:latest",
	}, source.CommandsToRetagAs(dest))

	dest.Tag = "v1.2.3"
	assert.Equal(t, []string{
		"docker pull quay.io/weaveworks/wks:latest",
		"docker tag quay.io/weaveworks/wks:latest acme.com:8443/mono-repo/my-custom-wks:v1.2.3",
		"docker push acme.com:8443/mono-repo/my-custom-wks:v1.2.3",
	}, source.CommandsToRetagAs(dest))

	// Blanking a "coordinate" is also possible:
	dest.Tag = ""
	assert.Equal(t, []string{
		"docker pull quay.io/weaveworks/wks:latest",
		"docker tag quay.io/weaveworks/wks:latest acme.com:8443/mono-repo/my-custom-wks",
		"docker push acme.com:8443/mono-repo/my-custom-wks",
	}, source.CommandsToRetagAs(dest))
}

func TestByCoordinate(t *testing.T) {
	assert.False(t, registry.ByCoordinate([]registry.Image{
		{Registry: "a", User: "b", Name: "c", Tag: "d"},
		{Registry: "a", User: "b", Name: "c", Tag: "d"},
	}).Less(0, 1))

	assert.True(t, registry.ByCoordinate([]registry.Image{
		{Registry: "a", User: "c", Name: "d", Tag: "e"},
		{Registry: "b", User: "b", Name: "c", Tag: "d"},
	}).Less(0, 1))

	assert.True(t, registry.ByCoordinate([]registry.Image{
		{Registry: "a", User: "b", Name: "d", Tag: "e"},
		{Registry: "a", User: "c", Name: "c", Tag: "d"},
	}).Less(0, 1))

	assert.True(t, registry.ByCoordinate([]registry.Image{
		{Registry: "a", User: "b", Name: "c", Tag: "e"},
		{Registry: "a", User: "b", Name: "d", Tag: "d"},
	}).Less(0, 1))

	assert.True(t, registry.ByCoordinate([]registry.Image{
		{Registry: "a", User: "b", Name: "c", Tag: "d"},
		{Registry: "a", User: "b", Name: "c", Tag: "e"},
	}).Less(0, 1))
}
