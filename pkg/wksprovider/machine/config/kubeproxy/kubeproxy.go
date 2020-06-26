package kubeproxy

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeproxycfg "k8s.io/kube-proxy/config/v1alpha1"
	"k8s.io/utils/pointer"
)

const kubeProxyConfiguration = "KubeProxyConfiguration"

// NewConfig returns an KubeProxyConfiguration with appropriate
// defaults set for WKS.
func NewConfig(conntrackMax int32) *kubeproxycfg.KubeProxyConfiguration {
	return &kubeproxycfg.KubeProxyConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       kubeProxyConfiguration,
			APIVersion: kubeproxycfg.SchemeGroupVersion.String(),
		},
		Conntrack: kubeproxycfg.KubeProxyConntrackConfiguration{
			MaxPerCore: pointer.Int32Ptr(conntrackMax),
		},
	}
}
