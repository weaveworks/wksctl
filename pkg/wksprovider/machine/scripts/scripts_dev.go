// +build dev

package scripts

import "net/http"

// Scripts contains all operating systems' scripts.
var Scripts http.FileSystem = http.Dir("all")
