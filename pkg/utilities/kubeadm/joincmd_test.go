package kubeadm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/kubeadm"
)

const (
	kubeadmInitPartialStdout = `Your Kubernetes master has initialized successfully!

To start using your cluster, you need to run the following as a regular user:

  mkdir -p $HOME/.kube
  sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
  sudo chown $(id -u):$(id -g) $HOME/.kube/config

You should now deploy a pod network to the cluster.
Run "kubectl apply -f [podnetwork].yaml" with one of the options listed at:
  https://kubernetes.io/docs/concepts/cluster-administration/addons/

You can now join any number of machines by running the following on each node
as root:

  kubeadm join 172.17.0.2:6443 --token hpr1w5.ec293ztzcstptgz6 --discovery-token-ca-cert-hash sha256:ddddf4513d9dd1641b6da46cf5a83f18f269a72e1e7145d55c4cbc03fd4f7309

`
	kubeadmJoinCmd = "kubeadm join 172.17.0.2:6443 --token hpr1w5.ec293ztzcstptgz6 --discovery-token-ca-cert-hash sha256:ddddf4513d9dd1641b6da46cf5a83f18f269a72e1e7145d55c4cbc03fd4f7309"
)

func TestExtractJoinCmd(t *testing.T) {
	extractedKubeadmJoinCmd, err := kubeadm.ExtractJoinCmd(kubeadmInitPartialStdout)
	assert.NoError(t, err)
	assert.Equal(t, kubeadmJoinCmd, extractedKubeadmJoinCmd)
}

func TestExtractDiscoveryTokenCaCertHash(t *testing.T) {
	hash, err := kubeadm.ExtractDiscoveryTokenCaCertHash(kubeadmJoinCmd)
	assert.NoError(t, err)
	assert.Equal(t, "sha256:ddddf4513d9dd1641b6da46cf5a83f18f269a72e1e7145d55c4cbc03fd4f7309", hash)
}

const (
	kubeadmInitPartialStdoutWithMultilineJoin = `Your Kubernetes control-plane has initialized successfully!

To start using your cluster, you need to run the following as a regular user:

  mkdir -p $HOME/.kube
  sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
  sudo chown $(id -u):$(id -g) $HOME/.kube/config

You should now deploy a pod network to the cluster.
Run "kubectl apply -f [podnetwork].yaml" with one of the options listed at:
  https://kubernetes.io/docs/concepts/cluster-administration/addons/

Then you can join any number of worker nodes by running the following on each as root:

kubeadm join 172.17.0.2:6443 --token 2wbvbd.bib9jtbupd9vu7ke \
    --discovery-token-ca-cert-hash sha256:c70a0fcbbce8e1b579b6af359daef91e3dd1a37ce1359c4ee726c441253503b2
`
	kubeadmJoinCmdOneLiner = "kubeadm join 172.17.0.2:6443 --token 2wbvbd.bib9jtbupd9vu7ke --discovery-token-ca-cert-hash sha256:c70a0fcbbce8e1b579b6af359daef91e3dd1a37ce1359c4ee726c441253503b2"
)

func TestExtractJoinCmdLineContinuation(t *testing.T) {
	extractedKubeadmJoinCmd, err := kubeadm.ExtractJoinCmd(kubeadmInitPartialStdoutWithMultilineJoin)
	assert.NoError(t, err)
	assert.Equal(t, kubeadmJoinCmdOneLiner, extractedKubeadmJoinCmd)
}

func TestExtractCertificateKey(t *testing.T) {
	cmd := "kubeadm join 192.168.0.200:6443 --token 9vr73a.a8uxyaju799qwdjv --discovery-token-ca-cert-hash sha256:7c2e69131a36ae2a042a339b33381c6d0d43887e2de83720eff5359e26aec866 --experimental-control-plane"
	expectedKey := "f8902e114ef118304e561c3ecd4d0b543adc226b7a07f675f56564185ffe0c07"
	// case 1: --certificate-key <value>
	key, err := kubeadm.ExtractCertificateKey(cmd + " --certificate-key " + expectedKey)
	assert.NoError(t, err)
	assert.Equal(t, expectedKey, key)
	// case 2: --certificate-key=<value>
	key, err = kubeadm.ExtractCertificateKey(cmd + " --certificate-key=" + expectedKey)
	assert.NoError(t, err)
	assert.Equal(t, expectedKey, key)
}
