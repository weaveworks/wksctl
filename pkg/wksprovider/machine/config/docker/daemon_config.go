package docker

// CommonConfig defines the bits of configuration for a docker daemon which is
// needed by WKS.
// See also: https://github.com/docker/engine/blob/v18.09.2/daemon/config/config.go#L111-L115
// There were attempts to directly vendor the above, but this ended up in
// dependency hell, and someone almost being sent to the mental hospital,
// hence this partial copy.
type CommonConfig struct {
	LogConfig
	ExecOptions  []string `json:"exec-opts,omitempty"`
	GraphDriver  string   `json:"storage-driver,omitempty"`
	GraphOptions []string `json:"storage-opts,omitempty"`
}

// LogConfig represents the default log configuration.
// It includes json tags to deserialize configuration from a file
// using the same names that the flags in the command line use.
type LogConfig struct {
	Type   string            `json:"log-driver,omitempty"`
	Config map[string]string `json:"log-opts,omitempty"`
}
