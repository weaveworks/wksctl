package utilities

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndent(t *testing.T) {
	for _, tt := range []struct {
		name      string
		s, indent string
		want      string
	}{
		{
			name: "empty",
		},
		{
			name:   "one line",
			s:      "hello world",
			indent: "  ",
			want:   "  hello world",
		},
		{
			name:   "one line, trimmed",
			s:      "hello world\n",
			indent: "  ",
			want:   "  hello world",
		},
		{
			name:   "many lines",
			s:      "hello world\npotato\ncarrot\n",
			indent: "  ",
			want:   "  hello world\n  potato\n  carrot",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Indent(tt.s, tt.indent)
			assert.Equal(t, got, tt.want)
		})
	}
}
