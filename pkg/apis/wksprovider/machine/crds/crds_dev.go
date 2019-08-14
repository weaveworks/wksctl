// +build dev

package crds

import "net/http"

// CRDs contains wksctl's crds.
var CRDs http.FileSystem = http.Dir("../../../cluster-api/config/crds")
