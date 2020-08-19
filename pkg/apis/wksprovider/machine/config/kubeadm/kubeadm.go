package kubeadm

import (
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	"k8s.io/kubernetes/cmd/kubeadm/app/constants"
)

// ClusterConfigurationParams groups the values to provide to
// NewClusterConfiguration to create a new ClusterConfiguration object.
type ClusterConfigurationParams struct {
	KubernetesVersion string
	NodeIPs           []string
	// ControlPlaneEndpoint is the IP:port of the control plane load balancer.
	// Default: localhost:6443
	// See also: https://kubernetes.io/docs/setup/independent/high-availability/#stacked-control-plane-and-etcd-nodes
	ControlPlaneEndpoint string
	// Used to configure kubeadm and kubelet with a cloud provider
	CloudProvider   string
	ImageRepository string
	// AdditionalSANs can hold additional SANs to add to the API server certificate.
	AdditionalSANs []string
	// Additional arguments for auth, etc.
	ExtraArgs map[string]string
	// The IP range for services
	ServiceCIDRBlock string
	// PodCIDRBlock is the subnet used by pods.
	PodCIDRBlock string
}

// NewClusterConfiguration returns an ClusterConfiguration with appropriate
// defaults set for WKS.
func NewClusterConfiguration(params ClusterConfigurationParams) *kubeadmapi.ClusterConfiguration {
	SANs := []string{}
	SANs = append(SANs, params.NodeIPs...)
	SANs = append(SANs, params.AdditionalSANs...)

	cc := &kubeadmapi.ClusterConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       constants.ClusterConfigurationKind,
			APIVersion: kubeadmapi.SchemeGroupVersion.String(),
		},
		Networking: kubeadmapi.Networking{
			ServiceSubnet: params.ServiceCIDRBlock,
			PodSubnet:     params.PodCIDRBlock,
		},
		APIServer: kubeadmapi.APIServer{
			CertSANs:              certSANs(SANs...),
			ControlPlaneComponent: kubeadmapi.ControlPlaneComponent{ExtraArgs: params.ExtraArgs},
		},
		KubernetesVersion:    params.KubernetesVersion,
		ControlPlaneEndpoint: getOrDefaultControlPlaneEndpoint(params.ControlPlaneEndpoint),
		ImageRepository:      params.ImageRepository,
	}
	if params.CloudProvider != "" {
		cpc := kubeadmapi.ControlPlaneComponent{
			ExtraArgs: map[string]string{
				"cloud-provider": params.CloudProvider,
			},
		}
		cc.APIServer.ControlPlaneComponent = cpc
		cc.ControllerManager = cpc

	}
	return cc
}

func getOrDefaultControlPlaneEndpoint(controlPlaneEndpoint string) string {
	if len(controlPlaneEndpoint) > 0 {
		return controlPlaneEndpoint
	}
	return "localhost:6443"
}

func certSANs(names ...string) []string {
	certSANs := make([]string, 0, len(names)+1)
	certSANs = append(certSANs, "localhost")
	for _, name := range names {
		if name != "" {
			certSANs = append(certSANs, name)
		}
	}
	return certSANs
}

// InitConfigurationParams groups the values to provide to NewInitConfiguration
// to create a new InitConfiguration object.
type InitConfigurationParams struct {
	NodeName string
	// BootstrapToken is the token used by kubeadm init and kubeadm join to
	// safely form new clusters.
	BootstrapToken *kubeadmapi.BootstrapTokenString
	KubeletConfig  config.KubeletConfig
}

// NewInitConfiguration returns an InitConfiguration with appropriate defaults
// set for WKS.
func NewInitConfiguration(params InitConfigurationParams) *kubeadmapi.InitConfiguration {
	ic := &kubeadmapi.InitConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       constants.InitConfigurationKind,
			APIVersion: kubeadmapi.SchemeGroupVersion.String(),
		},
		LocalAPIEndpoint: kubeadmapi.APIEndpoint{
			AdvertiseAddress: params.KubeletConfig.NodeIP,
		},
		BootstrapTokens: []kubeadmapi.BootstrapToken{
			{
				Token: params.BootstrapToken,
			},
		},
		NodeRegistration: kubeadmapi.NodeRegistrationOptions{
			Name: params.NodeName,
			KubeletExtraArgs: map[string]string{
				"node-ip": params.KubeletConfig.NodeIP,
			},
		},
	}
	if params.KubeletConfig.CloudProvider != "" {
		ic.NodeRegistration.KubeletExtraArgs["cloud-provider"] = params.KubeletConfig.CloudProvider
	}
	return ic
}

// JoinConfigurationParams groups the values to provide to NewJoinConfiguration
// to create a new JoinConfiguration object.
type JoinConfigurationParams struct {
	// IsMaster should be true if this node should join as a master, or false otherwise.
	IsMaster bool
	// LocalMasterAdvertiseAddress is this master node's address. Default: localhost.
	LocalMasterAdvertiseAddress string
	// LocalMasterBindPort is this master node's port. Default: 6443.
	LocalMasterBindPort int32
	// NodeIP is the IP that kubelet will use for this node.
	NodeIP string
	// APIServerEndpoint is the <IP/host>:<port> to use to connect to the API
	// server in order to join this node.
	APIServerEndpoint string
	// Token is used to ensure this node can safely join the Kubernetes cluster.
	Token string
	// CACertHash is used to ensure this node can safely join the Kubernetes cluster.
	CACertHash string
}

// NewJoinConfiguration returns an JoinConfiguration with appropriate defaults
// set for WKS.
func NewJoinConfiguration(params JoinConfigurationParams) *kubeadmapi.JoinConfiguration {
	cfg := &kubeadmapi.JoinConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       constants.JoinConfigurationKind,
			APIVersion: kubeadmapi.SchemeGroupVersion.String(),
		},
		NodeRegistration: kubeadmapi.NodeRegistrationOptions{
			KubeletExtraArgs: map[string]string{
				"node-ip": params.NodeIP,
			},
		},
		Discovery: kubeadmapi.Discovery{
			BootstrapToken: &kubeadmapi.BootstrapTokenDiscovery{
				Token:             params.Token,
				APIServerEndpoint: params.APIServerEndpoint,
				CACertHashes: []string{
					params.CACertHash,
				},
			},
		},
	}
	if params.IsMaster {
		cfg.ControlPlane = &kubeadmapi.JoinControlPlane{
			LocalAPIEndpoint: kubeadmapi.APIEndpoint{
				AdvertiseAddress: params.LocalMasterAdvertiseAddress,
				BindPort:         getOrDefaultLocalMasterBindPort(params.LocalMasterBindPort),
			},
		}
	}
	return cfg
}

func getOrDefaultLocalMasterBindPort(localMasterBindPort int32) int32 {
	if localMasterBindPort > 0 {
		return localMasterBindPort
	}
	return 6443
}
