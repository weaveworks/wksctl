// +build dev

package manifests

import "net/http"

// Manifests contains wksctl's manifests.
var Manifests http.FileSystem = http.Dir("yaml")
