package kubeadm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/config"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config/kubeadm"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	"sigs.k8s.io/yaml"
)

func TestSerializeKubeadmClusterConfiguration(t *testing.T) {
	cfg := kubeadm.NewClusterConfiguration(kubeadm.ClusterConfigurationParams{
		KubernetesVersion: "x.y.z",
		NodeIPs:           []string{"127.0.0.1", "1.2.3.4"},
	})
	assert.NotNil(t, cfg)
	bytes, err := yaml.Marshal(cfg)
	assert.NoError(t, err)
	yamlCfg := string(bytes)
	assert.Contains(t, yamlCfg, "kubernetesVersion: x.y.z")
	assert.Contains(t, yamlCfg, "apiVersion: kubeadm.k8s.io/v1beta1")
	assert.Contains(t, yamlCfg, "kind: ClusterConfiguration")
	assert.Contains(t, yamlCfg, `apiServer:
  certSANs:
  - localhost
  - 127.0.0.1
  - 1.2.3.4`)
	assert.Contains(t, yamlCfg, "controlPlaneEndpoint: localhost:6443")
}

func TestSerializeKubeadmInitConfiguration(t *testing.T) {
	cfg := kubeadm.NewInitConfiguration(kubeadm.InitConfigurationParams{
		KubeletConfig: config.KubeletConfig{NodeIP: "127.0.0.1"},
		BootstrapToken: &kubeadmapi.BootstrapTokenString{
			ID:     "abcdef",
			Secret: "abcdefghijklmnop",
		},
	})
	assert.NotNil(t, cfg)
	bytes, err := yaml.Marshal(cfg)
	assert.NoError(t, err)
	assert.Equal(t, `apiVersion: kubeadm.k8s.io/v1beta1
bootstrapTokens:
- token: abcdef.abcdefghijklmnop
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: 127.0.0.1
  bindPort: 0
nodeRegistration:
  kubeletExtraArgs:
    node-ip: 127.0.0.1
`, string(bytes))
}

func TestSerializeKubeadmJoinConfiguration(t *testing.T) {
	cfg := kubeadm.NewJoinConfiguration(kubeadm.JoinConfigurationParams{
		NodeIP:            "127.0.0.1",
		APIServerEndpoint: "127.0.0.2:6443",
		Token:             "t0k3n",
		CACertHash:        "sha256:c3rth4sh",
	})
	assert.NotNil(t, cfg)
	bytes, err := yaml.Marshal(cfg)
	assert.NoError(t, err)
	assert.Equal(t, `apiVersion: kubeadm.k8s.io/v1beta1
caCertPath: ""
discovery:
  bootstrapToken:
    apiServerEndpoint: 127.0.0.2:6443
    caCertHashes:
    - sha256:c3rth4sh
    token: t0k3n
    unsafeSkipCAVerification: false
  tlsBootstrapToken: ""
kind: JoinConfiguration
nodeRegistration:
  kubeletExtraArgs:
    node-ip: 127.0.0.1
`, string(bytes))
}

func TestAWSCloudProviderClusterConfig(t *testing.T) {
	cfg := kubeadm.NewClusterConfiguration(kubeadm.ClusterConfigurationParams{
		KubernetesVersion: "x.y.z",
		NodeIPs:           []string{"127.0.0.1", "1.2.3.4"},
		CloudProvider:     "aws",
	})
	assert.NotNil(t, cfg)
	bytes, err := yaml.Marshal(cfg)
	assert.NoError(t, err)
	yamlCfg := string(bytes)
	assert.Contains(t, yamlCfg, "cloud-provider: aws")
}

func TestAWSKubletConfig(t *testing.T) {
	cfg := kubeadm.NewInitConfiguration(kubeadm.InitConfigurationParams{
		KubeletConfig: config.KubeletConfig{NodeIP: "127.0.0.1", CloudProvider: "aws"},
		BootstrapToken: &kubeadmapi.BootstrapTokenString{
			ID:     "abcdef",
			Secret: "abcdefghijklmnop",
		},
	})
	assert.NotNil(t, cfg)
	bytes, err := yaml.Marshal(cfg)
	assert.NoError(t, err)
	assert.Equal(t, `apiVersion: kubeadm.k8s.io/v1beta1
bootstrapTokens:
- token: abcdef.abcdefghijklmnop
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: 127.0.0.1
  bindPort: 0
nodeRegistration:
  kubeletExtraArgs:
    cloud-provider: aws
    node-ip: 127.0.0.1
`, string(bytes))
}
