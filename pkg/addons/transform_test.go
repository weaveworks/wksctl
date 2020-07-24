package addons_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/addons"
)

func TestUpdateImage(t *testing.T) {
	tests := []struct {
		image         string
		repository    string
		expectedImage string
		expectedError error
	}{
		// CNI addon's images should just have their repository updated:
		{
			image:         "docker.io/weaveworks/weave-kube:2.6.5",
			repository:    "172.17.0.2:5000",
			expectedImage: "172.17.0.2:5000/weaveworks/weave-kube:2.6.5",
			expectedError: nil,
		},
		// WKS controller's image should just have its repository updated:
		{
			image:         "quay.io/wksctl/controller:master",
			repository:    "172.17.0.2:5000",
			expectedImage: "172.17.0.2:5000/wksctl/controller:master",
			expectedError: nil,
		},
		// Override the namespace a.k.a. organisation by what is provided in
		// the repository URL, as this is what we've recommended to customers
		// in the past, even though it diverges from upstream's naming:
		{
			image:         "quay.io/wksctl/controller:master",
			repository:    "registry.weave.works/wkp",
			expectedImage: "registry.weave.works/wkp/controller:master",
			expectedError: nil,
		},
		// Override the namespace a.k.a. organisation by what is provided in
		// the repository URL, as this is what we've recommended to customers
		// in the past, even though it diverges from upstream's naming:
		{
			image:         "grafana/grafana:x.y.z",
			repository:    "registry.weave.works/wkp",
			expectedImage: "registry.weave.works/wkp/grafana:x.y.z",
			expectedError: nil,
		},
		// WKS controller's image shouldn't change if no repository is specified:
		{
			image:         "quay.io/wksctl/controller:master",
			repository:    "",
			expectedImage: "quay.io/wksctl/controller:master",
			expectedError: nil,
		},
	}
	for _, test := range tests {
		updatedImage, err := addons.UpdateImage(test.image, test.repository)
		if test.expectedError != nil {
			assert.Equal(t, test.expectedError, err)
		} else {
			assert.Equal(t, test.expectedImage, updatedImage)
		}
	}
}
