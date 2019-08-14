package quay_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/quay"
	"github.com/weaveworks/wksctl/pkg/registry"
	"github.com/weaveworks/wksctl/pkg/utilities/version"
)

const numImages = 103

func TestListImages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test against quay.io, as these may take a long time")
	}
	images, err := quay.ListImages("wks", version.AnyRange)
	assert.NoError(t, err)
	assert.Truef(t, len(images) >= numImages, "At least %v images should be returned, but got %v: %v", numImages, len(images), images)
	assert.Contains(t, images, registry.Image{
		Registry: "quay.io",
		User:     "wks",
		Name:     "kube-proxy-amd64",
		Tag:      "v1.11.1",
	})
	assert.NotContains(t, images, registry.Image{
		Registry: "quay.io",
		User:     "wks",
		Name:     "wks",
		Tag:      "master-1e5b367",
	})
}
