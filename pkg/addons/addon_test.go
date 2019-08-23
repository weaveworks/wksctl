package addons

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/registry"
)

func TestListImages(t *testing.T) {
	addon, err := Get("flux")
	assert.NoError(t, err, "missing 'flux' addon")

	images, err := addon.ListImages()

	assert.NoError(t, err)
	assert.NotEmpty(t, images)
	assert.Equal(t, 2, len(images))
	matchCount := 0
	image1 := registry.Image{
		Registry: "",
		User:     "fluxcd",
		Name:     "flux",
		Tag:      "1.13.3",
	}
	image2 := registry.Image{
		Registry: "",
		User:     "",
		Name:     "memcached",
		Tag:      "1.4.25",
	}
	for _, image := range images {
		if image == image1 || image == image2 {
			matchCount++
		}
	}
	assert.Equal(t, 2, matchCount)
}

func TestBuildAllAddons(t *testing.T) {
	dir, err := ioutil.TempDir("", t.Name())
	assert.NoError(t, err)
	defer os.RemoveAll(dir)

	for _, addon := range List() {
		t.Run(addon.ShortName, func(t *testing.T) {
			addon.autoBuild(BuildOptions{
				OutputDirectory: dir,
			})
		})
	}
}

func TestListImagesAllAddons(t *testing.T) {
	for _, addon := range List() {
		t.Run(addon.ShortName, func(t *testing.T) {
			images, err := addon.ListImages()
			assert.NoError(t, err)
			assert.True(t, len(images) >= 1)
		})
	}
}
