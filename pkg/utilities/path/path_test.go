package path

import (
	"testing"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/stretchr/testify/assert"
)

func TestWKSHome(t *testing.T) {
	t.Run("explicit path unchanged", func(t *testing.T) {
		assert.Equal(t, WKSHome("/aPath"), "/aPath")
	})
	homeDir, err := homedir.Dir()
	assert.NoError(t, err)
	t.Run("'~' path expanded", func(t *testing.T) {
		assert.Equal(t, WKSHome("~/aPath"), homeDir+"/aPath")
	})
	t.Run("empty path replaced with homedir/.wks", func(t *testing.T) {
		assert.Equal(t, WKSHome(""), homeDir+"/.wks")
	})
}
