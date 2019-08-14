// +build dev

package assets

import "net/http"

// Assets is the content of the addons directory.
var Assets http.FileSystem = http.Dir("../../../addons")
