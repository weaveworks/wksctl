package apiserver

import (
	"io"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	capeissh "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/kubernetes/config"
	"github.com/weaveworks/wksctl/pkg/specs"
	"k8s.io/client-go/kubernetes"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type ClientParams struct {
}

type Client struct {
	client    *k8s.Clientset
	sshClient *capeissh.Client
	sp        *specs.Specs
}

// NewClient creates an api server client based on the kubeconfig from the
// initial master machine
func NewClient(sp *specs.Specs, sshClient *capeissh.Client) (*Client, error) {
	log.Debug("Creating new apiserver client")
	return &Client{sshClient: sshClient,
		sp: sp}, nil

}
func (c *Client) fetchKubeconfig() error {
	// get the configuration from the master node
	configStr, err := config.GetRemoteKubeconfigSSH(c.sp, c.sshClient, true, true)
	if err != nil {
		return errors.Wrapf(err, "failed to get remote kubeconfig")
	}

	rConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(configStr))
	if err != nil {
		return errors.Wrapf(err, "failed to create REST config from kubeconfig")
	}

	client, err := kubernetes.NewForConfig(rConfig)
	if err != nil {
		return errors.Wrap(err, "Failed to convert client from rest client")
	}
	c.client = client
	return nil
}

// RunCommand executes the provided command using the K8s API server
func (c *Client) RunCommand(command string, stdin io.Reader) (string, error) {
	log.Debugf("running command: %s", command)
	// The first time we run this we need to fetch the kubeconfig
	if c.client == nil {
		if err := c.fetchKubeconfig(); err != nil {
			return "", errors.Wrap(err, "Filed to fetch kubeconfig for apiClient")
		}
	}
	return "", nil
}
