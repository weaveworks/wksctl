package version

var (
	// The constants below are to be overridden by linker flags passed to `go build`.
	// Examples: -X github.com/weaveworks/wksctl/pkg/version.Version=xxxxx -X github.com/weaveworks/wksctl/pkg/version.ImageTag=yyyyy
	// If wksctl is used as an imported module, the importing program shall override these values using the linker flags above. If not done, the defaults below will be used instead.

	Version  = "undefined"
	ImageTag = "0.8.0-beta.1"
)
