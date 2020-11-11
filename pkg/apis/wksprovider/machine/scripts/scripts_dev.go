// +build dev

package scripts

import (
	"net/http"

	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/fixeddate"
)

// Scripts contains all operating systems' scripts.
var Scripts http.FileSystem = fixeddate.Dir("all")
