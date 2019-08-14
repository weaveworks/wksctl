package node

import (
	corev1 "k8s.io/api/core/v1"
)

// IsMaster returns true if the provided node is a master or false otherwise.
func IsMaster(n corev1.Node) bool {
	_, ok := n.Labels["node-role.kubernetes.io/master"]
	return ok
}

// IsWorker returns true if the provided node is a worker or false otherwise.
func IsWorker(n corev1.Node) bool {
	return !IsMaster(n)
}
