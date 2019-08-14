package nodes

import (
	"github.com/weaveworks/wksctl/pkg/cluster/node"
	corev1 "k8s.io/api/core/v1"
)

// Masters selects master nodes among the provided nodes.
func Masters(nodes corev1.NodeList) corev1.NodeList {
	out := corev1.NodeList{}
	for _, n := range nodes.Items {
		if node.IsMaster(n) {
			out.Items = append(out.Items, n)
		}
	}
	return out
}

// Workers selects master nodes among the provided nodes.
func Workers(nodes corev1.NodeList) corev1.NodeList {
	out := corev1.NodeList{}
	for _, n := range nodes.Items {
		if node.IsWorker(n) {
			out.Items = append(out.Items, n)
		}
	}
	return out
}
