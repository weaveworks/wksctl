package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLine(t *testing.T) {
	tests := []struct {
		output, expected string
	}{
		{"foo", "foo"},
		{"foo\n", "foo"},
		{"foo\nbar\n", "foo"},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, line(test.output))
	}
}

const testSystemdShow = `
Delegate=no
CPUAccounting=no
CPUShares=18446744073709551615
StartupCPUShares=18446744073709551615
CPUQuotaPerSecUSec=infinity
BlockIOAccounting=no
BlockIOWeight=18446744073709551615
StartupBlockIOWeight=18446744073709551615
MemoryAccounting=no
MemoryLimit=18446744073709551615
DevicePolicy=auto
`

func TestKeyval(t *testing.T) {
	tests := []struct {
		output, key, expected string
	}{
		{"foo=bar", "foo", "bar"},
		{"foo=\"bar\"", "foo", "bar"},
		{"foo=bar", "meh", ""},
		{testSystemdShow, "MemoryAccounting", "no"},
	}

	for _, test := range tests {
		v := keyval(test.output, test.key)
		assert.Equal(t, test.expected, v)
	}
}
