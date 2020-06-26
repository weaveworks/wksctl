package config

// KubeletConfig groups all options & flags which need to be passed to kubelet.
type KubeletConfig struct {
	NodeIP         string
	CloudProvider  string
	ExtraArguments map[string]string
}
