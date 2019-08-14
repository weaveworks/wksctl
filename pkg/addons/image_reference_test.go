package addons

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseImageReference(t *testing.T) {
	tests := []struct {
		input        string
		err          error
		domain       string
		organisation string
		name         string
		tag          string
		digest       string
	}{
		{
			input: "test_com",
			name:  "test_com",
		},
		{
			input: "test.com:tag",
			name:  "test.com",
			tag:   "tag",
		},
		{
			input: "test.com:5000",
			name:  "test.com",
			tag:   "5000",
		},
		{
			input:  "host.com/name:tag",
			domain: "host.com",
			name:   "name",
			tag:    "tag",
		},
		{
			input:  "host:5000/name",
			domain: "host:5000",
			name:   "name",
		},
		{
			input:  "host:5000/name:tag",
			domain: "host:5000",
			name:   "name",
			tag:    "tag",
		},
		{
			input:  "host:5000/name@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			domain: "host:5000",
			name:   "name",
			digest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		{
			input:  "host:5000/name:tag@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			domain: "host:5000",
			name:   "name",
			tag:    "tag",
			digest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		{
			input:        "host:5000/org/name:tag@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			domain:       "host:5000",
			organisation: "org",
			name:         "name",
			tag:          "tag",
			digest:       "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		{
			input:        "host/org/name:tag@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			domain:       "host",
			organisation: "org",
			name:         "name",
			tag:          "tag",
			digest:       "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		{
			input:        "org/name:tag@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			domain:       "",
			organisation: "org",
			name:         "name",
			tag:          "tag",
			digest:       "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		},
		{
			input:  "host:5000/name",
			domain: "host:5000",
			name:   "name",
		},
	}

	for _, test := range tests {
		ref, err := parseImageReference(test.input)
		assert.Equal(t, test.err, err)
		assert.Equal(t, test.domain, ref.Domain)
		assert.Equal(t, test.organisation, ref.Organisation)
		assert.Equal(t, test.name, ref.Name)
		assert.Equal(t, test.tag, ref.Tag)
		assert.Equal(t, test.digest, ref.Digest)
	}
}
