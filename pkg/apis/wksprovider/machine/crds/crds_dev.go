// +build dev

package crds

import (
	"net/http"

	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/fixeddate"
)

// CRDs contains wksctl's crds.
var CRDs http.FileSystem = fixeddate.Dir("../../../../../config/crd")
