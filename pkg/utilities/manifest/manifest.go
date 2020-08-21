package manifest

const (
	DefaultNamespace = `weavek8sops`
)

var DefaultAddonNamespaces = map[string]string{"weave-net": "kube-system"}
