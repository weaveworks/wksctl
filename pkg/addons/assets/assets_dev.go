// +build dev

package assets

import (
	"net/http"

	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/fixeddate"
)

// Assets is the content of the addons directory.
var Assets http.FileSystem = fixeddate.Dir("../../../addons")
