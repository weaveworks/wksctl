package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/utilities/version"
)

func TestMatchesRangeDefaultVersion(t *testing.T) {
	matches, err := version.MatchesRange(kubernetes.DefaultVersion, DefaultVersionsRange)
	assert.NoError(t, err)
	assert.True(t, matches)
}
