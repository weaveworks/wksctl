package nodes_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/wksctl/pkg/cluster/nodes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var master = corev1.Node{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{
			"node-role.kubernetes.io/master": "unused-value",
		},
	},
}

var worker = corev1.Node{
	ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{},
	},
}

var nodeList = corev1.NodeList{
	Items: []corev1.Node{
		master,
		worker,
	},
}

func TestMasters(t *testing.T) {
	masters := nodes.Masters(nodeList)
	assert.Len(t, masters.Items, 1)
	assert.Equal(t, master, masters.Items[0])
}

func TestWorkers(t *testing.T) {
	workers := nodes.Workers(nodeList)
	assert.Len(t, workers.Items, 1)
	assert.Equal(t, worker, workers.Items[0])
}
