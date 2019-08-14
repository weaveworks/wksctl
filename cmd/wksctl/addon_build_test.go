package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseParam(t *testing.T) {
	tests := []struct {
		intput   string
		valid    bool
		expected []string
	}{
		{"key", false, nil},
		{"key=value=bar", true, []string{"key", "value=bar"}},
		{"key=value", true, []string{"key", "value"}},
		{"key=value=bar", true, []string{"key", "value=bar"}},
	}

	for _, test := range tests {
		key, value, err := parseParam(test.intput)
		if !test.valid {
			assert.Error(t, err)
			assert.Equal(t, "", key)
			assert.Equal(t, "", value)
			continue
		}
		assert.NoError(t, err)
		assert.Equal(t, test.expected[0], key)
		assert.Equal(t, test.expected[1], value)
	}
}
