package path

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWKSHome(t *testing.T) {
	// explicit path unchanged
	assert.Equal(t, WKSHome("/aPath"), "/aPath")

	// '~' path expanded
	homeDir, err := os.UserHomeDir()
	assert.NoError(t, err)
	assert.Equal(t, WKSHome("~/aPath"), homeDir+"/aPath")

	// empty path replaced with homedir/.wks
	assert.Equal(t, WKSHome(""), homeDir+"/.wks")
}
