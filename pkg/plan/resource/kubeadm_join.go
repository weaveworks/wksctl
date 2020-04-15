package resource

import (
	"fmt"
	"github.com/weaveworks/wksctl/pkg/utilities/version"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/plan"
)

// KubeadmJoin represents an attempt to join a Kubernetes node via kubeadm.
type KubeadmJoin struct {
	base

	// IsMaster should be true if this node should join as a master, or false otherwise.
	IsMaster bool `structs:"isMaster"`
	// NodeIP is the IP of the node trying to join the cluster.
	NodeIP string `structs:"nodeIP"`
	// NodeName, if non-empty, will override the default node name guessed by kubeadm.
	NodeName string
	// MasterIP is the IP of the master node to connect to in order to join the cluster --
	// hidden because the value can change in multi-master configurations but should not make the node plan
	// appear to have changed.
	MasterIP string `structs:"masterIP" plan:"hide"`
	// MasterPort is the port of the master node to connect to in order to join the cluster.
	MasterPort int `structs:"masterPort"`
	// Token is used to authenticate with the Kubernetes API server.
	Token string `structs:"token" plan:"hide"`
	// DiscoveryTokenCaCertHash is used to validate that the root CA public key of the cluster we are trying to join matches.
	DiscoveryTokenCaCertHash string `structs:"discoveryTokenCaCertHash" plan:"hide"`
	// CertificateKey is used to add master nodes to the cluster.
	CertificateKey string `structs:"certificateKey" plan:"hide"`
	// IgnorePreflightErrors is optionally used to skip kubeadm's preflight checks.
	IgnorePreflightErrors []string `structs:"ignorePreflightErrors"`
	// External Load Balancer name or IP address to be used instead of the master's IP
	ExternalLoadBalancer string `structs:"externalLoadBalancer"`
	// Kubernetes Version is used to prepare different parameters
	Version string `structs:"version"`
}

var _ plan.Resource = plan.RegisterResource(&KubeadmJoin{})

// State implements plan.Resource.
func (kj *KubeadmJoin) State() plan.State {
	return toState(kj)
}

// Apply implements plan.Resource.
// TODO: find a way to make this idempotent.
// TODO: should such a resource be splitted in smaller resources?
func (kj *KubeadmJoin) Apply(runner plan.Runner, diff plan.Diff) (bool, error) {
	log.Info("joining Kubernetes cluster")
	apiServerEndpoint := fmt.Sprintf("%s:%d", kj.MasterIP, kj.MasterPort)
	if kj.ExternalLoadBalancer != "" {
		apiServerEndpoint = fmt.Sprintf("%s:%d", kj.ExternalLoadBalancer, kj.MasterPort)
	}
	kubeadmJoinCmd := kj.kubeadmJoinCmd(apiServerEndpoint)
	if stdouterr, err := runner.RunCommand(withoutProxy(kubeadmJoinCmd), nil); err != nil {
		log.WithField("stdouterr", stdouterr).Debug("failed to join cluster")
		return false, errors.Wrap(err, "failed to join cluster")
	}
	return true, nil
}

func (kj *KubeadmJoin) kubeadmJoinCmd(apiServerEndpoint string) string {
	var kubeJoinCmd strings.Builder
	kubeJoinCmd.WriteString("kubeadm join")
	if len(kj.IgnorePreflightErrors) > 0 {
		kubeJoinCmd.WriteString(" --ignore-preflight-errors=")
		kubeJoinCmd.WriteString(strings.Join(kj.IgnorePreflightErrors, ","))
	}

	if kj.IsMaster {

		if lt, err := version.LessThan(kj.Version, "1.16.0"); err == nil && lt {
			kubeJoinCmd.WriteString(" --experimental-control-plane --certificate-key ")
		} else {
			kubeJoinCmd.WriteString(" --control-plane --certificate-key ")
		}

		kubeJoinCmd.WriteString(kj.CertificateKey)
	}
	kubeJoinCmd.WriteString(" --node-name=")
	kubeJoinCmd.WriteString(kj.NodeName)
	kubeJoinCmd.WriteString(" --token ")
	kubeJoinCmd.WriteString(kj.Token)
	kubeJoinCmd.WriteString(" --discovery-token-ca-cert-hash ")
	kubeJoinCmd.WriteString(kj.DiscoveryTokenCaCertHash)
	kubeJoinCmd.WriteString(" ")
	kubeJoinCmd.WriteString(apiServerEndpoint)
	return kubeJoinCmd.String()
}

// Undo implements plan.Resource.
func (kj *KubeadmJoin) Undo(runner plan.Runner, current plan.State) error {
	return errors.New("not implemented")
}
