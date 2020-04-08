package path

import (
	"path/filepath"
	"runtime"
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

func TestPrettify(t *testing.T) {
	homeDir, err := homedir.Dir()
	assert.NoError(t, err)
	t.Run("replace home prefix", func(t *testing.T) {
		assert.Equal(t, Prettify(filepath.Join(homeDir, ".abc"), true), "~/.abc")
	})
	t.Run("return same path", func(t *testing.T) {
		assert.Equal(t, Prettify("/abc", true), "/abc")
	})
	if runtime.GOOS == "windows" {
		t.Run("windows: enabled", func(t *testing.T) {
			assert.Equal(t, Prettify(filepath.Join(homeDir, ".abc"), true), "~/.abc")
		})
		t.Run("windows: disabled", func(t *testing.T) {
			assert.Equal(t, Prettify(filepath.Join(homeDir, ".abc"), false), "~/.abc")
		})
	}
}
