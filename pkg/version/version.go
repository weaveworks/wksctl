package version

var (
	// The constants below are to be set by flags passed to `go build`.
	// Examples: -X version.Version=xxxxx -X version.ImageTag=yyyyy

	Version  = "undefined"
	ImageTag = "latest"
)
