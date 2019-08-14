package node_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/cluster/node"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var master = corev1.Node{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			"node-role.kubernetes.io/master": "",
		},
	},
}

var worker = corev1.Node{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{},
	},
}

func TestIsMaster(t *testing.T) {
	assert.True(t, node.IsMaster(master))
	assert.False(t, node.IsMaster(worker))
}

func TestIsWorker(t *testing.T) {
	assert.True(t, node.IsWorker(worker))
	assert.False(t, node.IsWorker(master))
}
