package wks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
)

func TestJoinTokenExpirationHandling(t *testing.T) {
	checks := []struct {
		nowOffset time.Duration
		exp       bool
		msg       string
	}{
		{nowOffset: (time.Hour * 1), exp: false, msg: "Token should be good for another hour"},
		{nowOffset: (time.Second * 1), exp: true, msg: "Token expires in 30 seconds"},
		{nowOffset: (time.Second * 59), exp: true, msg: "Token expires in 59 seconds"},
		{nowOffset: (time.Second * 61), exp: false, msg: "Token good for 61 seconds - expiration limit is 60"},
	}

	s := corev1.Secret{}
	for _, check := range checks {
		now := time.Now().Add(check.nowOffset)
		d := map[string][]byte{}
		d[bootstrapapi.BootstrapTokenExpirationKey] = []byte(now.Format(time.RFC3339))
		s.Data = d
		assert.Equal(t, check.exp, bootstrapTokenHasExpired(&s), check.msg)
	}
}
