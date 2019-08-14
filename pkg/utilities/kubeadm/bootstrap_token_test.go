package kubeadm_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/utilities/kubeadm"
)

func TestGenerateBootstrapToken(t *testing.T) {
	token, err := kubeadm.GenerateBootstrapToken()
	assert.NoError(t, err)
	assert.NotNil(t, token)
	assert.Regexp(t, regexp.MustCompile("^[a-z0-9]{6}$"), token.ID)
	assert.Regexp(t, regexp.MustCompile("^[a-z0-9]{16}$"), token.Secret)
}
