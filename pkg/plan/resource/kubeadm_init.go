package resource

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/controller/manifests"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config/kubeadm"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config/kubeproxy"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/scripts"
	"github.com/weaveworks/wksctl/pkg/plan"
	kubeadmutil "github.com/weaveworks/wksctl/pkg/utilities/kubeadm"
	"github.com/weaveworks/wksctl/pkg/utilities/manifest"
	"github.com/weaveworks/wksctl/pkg/utilities/object"
	"github.com/weaveworks/wksctl/pkg/utilities/ssh"
	yml "github.com/weaveworks/wksctl/pkg/utilities/yaml"
	corev1 "k8s.io/api/core/v1"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	"sigs.k8s.io/yaml"
)

// KubeadmInit represents an attempt to init a Kubernetes node via kubeadm.
type KubeadmInit struct {
	base

	// PublicIP is public IP of the master node we are trying to setup here.
	PublicIP string `structs:"publicIP"`
	// PrivateIP is private IP of the master node we are trying to setup here.
	PrivateIP string `structs:"privateIP"`
	// NodeName, if non-empty, will override the default node name guessed by kubeadm.
	NodeName string
	// KubeletConfig groups all options & flags which need to be passed to kubelet.
	KubeletConfig *config.KubeletConfig `structs:"kubeletConfig"`
	// ConntrackMax is the maximum number of NAT connections for kubeproxy to track (0 to leave as-is).
	ConntrackMax int32 `structs:"conntrackMax"`
	// UseIPTables controls whether the following command is called or not:
	//   sysctl net.bridge.bridge-nf-call-iptables=1
	// prior to running kubeadm init.
	UseIPTables bool `structs:"useIPTables"`
	// kubeadmInitScriptPath is the path to the "kubeadm init" script to use.
	KubeadmInitScriptPath string `structs:"kubeadmInitScriptPath"`
	// IgnorePreflightErrors is optionally used to skip kubeadm's preflight checks.
	IgnorePreflightErrors []string `structs:"ignorePreflightErrors"`
	// SSHKeyPath is the path to the private SSH key used by WKS to SSH into
	// nodes to add/remove them to/from the Kubernetes cluster.
	SSHKeyPath string `structs:"sshKeyPath"`
	// BootstrapToken is the token used by kubeadm init and kubeadm join to
	// safely form new clusters.
	BootstrapToken *kubeadmapi.BootstrapTokenString `structs:"bootstrapToken"`
	// The version of Kubernetes to install
	KubernetesVersion string `structs:"kubernetesVersion"`
	// ControlPlaneEndpoint is the IP:port of the control plane load balancer.
	// Default: localhost:6443
	// See also: https://kubernetes.io/docs/setup/independent/high-availability/#stacked-control-plane-and-etcd-nodes
	ControlPlaneEndpoint string `structs:"controlPlaneEndpoint"`
	// Cloud provider setting which is needed for kubeadm and kubelet
	CloudProvider string `structs:"cloudProvider"`
	// ImageRepository sets the container registry to pull images from. If empty,
	// `k8s.gcr.io` will be used by default.
	ImageRepository string `structs:"imageRepository"`
	// ExternalLoadBalancer is the name or IP of the external load balancer setup
	// in from the the API master nodes.
	ExternalLoadBalancer string
	// AdditionalSANs can hold additional SANs to add to the API server certificate.
	AdditionalSANs []string
	// The namespace in which to init kubeadm
	Namespace fmt.Stringer
	// Extra arguments to pass to the APIServer
	ExtraAPIServerArgs map[string]string
}

var _ plan.Resource = plan.RegisterResource(&KubeadmInit{})

// State implements plan.Resource.
func (ki *KubeadmInit) State() plan.State {
	return toState(ki)
}

// Apply implements plan.Resource.
// TODO: find a way to make this idempotent.
// TODO: should such a resource be split into smaller resources?
func (ki *KubeadmInit) Apply(runner plan.Runner, diff plan.Diff) (bool, error) {
	log.Info("initializing Kubernetes cluster")

	sshKey, err := ssh.ReadPrivateKey(ki.SSHKeyPath)
	if err != nil {
		return false, err
	}
	namespace := ki.Namespace.String()
	if namespace == "" {
		namespace = manifest.DefaultNamespace
	}
	clusterConfig, err := yaml.Marshal(kubeadm.NewClusterConfiguration(kubeadm.ClusterConfigurationParams{
		KubernetesVersion:    ki.KubernetesVersion,
		NodeIPs:              []string{ki.PublicIP, ki.PrivateIP},
		ControlPlaneEndpoint: ki.ControlPlaneEndpoint,
		CloudProvider:        ki.CloudProvider,
		ImageRepository:      ki.ImageRepository,
		ExternalLoadBalancer: ki.ExternalLoadBalancer,
		AdditionalSANs:       ki.AdditionalSANs,
		ExtraArgs:            ki.ExtraAPIServerArgs,
	}))
	if err != nil {
		return false, errors.Wrap(err, "failed to serialize kubeadm's ClusterConfiguration object")
	}
	kubeadmConfig, err := yaml.Marshal(kubeadm.NewInitConfiguration(kubeadm.InitConfigurationParams{
		NodeName:       ki.NodeName,
		BootstrapToken: ki.BootstrapToken,
		KubeletConfig:  *ki.KubeletConfig,
	}))
	if err != nil {
		return false, errors.Wrap(err, "failed to serialize kubeadm's InitConfiguration object")
	}
	kubeproxyConfig, err := yaml.Marshal(kubeproxy.NewConfig(ki.ConntrackMax))
	if err != nil {
		return false, errors.Wrap(err, "failed to serialize kube-proxy's KubeProxyConfiguration object")
	}

	config := yml.Concat(clusterConfig, kubeadmConfig, kubeproxyConfig)
	remotePath := "/tmp/wks_kubeadm_init_config.yaml"
	if err = scripts.WriteFile(config, remotePath, 0660, runner); err != nil {
		return false, errors.Wrap(err, "failed to upload kubeadm's configuration")
	}
	log.WithField("yaml", string(config)).Debug("uploaded kubeadm's configuration")
	defer removeFile(remotePath, runner)

	var stdOutErr string
	p := buildKubeadmInitPlan(remotePath, strings.Join(ki.IgnorePreflightErrors, ","),
		ki.UseIPTables, &stdOutErr)
	_, err = p.Apply(runner, plan.EmptyDiff())
	if err != nil {
		return false, errors.Wrap(err, "failed to initialize Kubernetes cluster with kubeadm")
	}

	// TODO: switch to cluster-info.yaml approach.
	kubeadmJoinCmd, err := kubeadmutil.ExtractJoinCmd(stdOutErr)
	if err != nil {
		return false, err
	}
	log.Debug(kubeadmJoinCmd)
	caCertHash, err := kubeadmutil.ExtractDiscoveryTokenCaCertHash(kubeadmJoinCmd)
	if err != nil {
		return false, err
	}
	certKey, err := kubeadmutil.ExtractCertificateKey(kubeadmJoinCmd)
	if err != nil {
		return false, err
	}

	if err := ki.kubectlApply("01_namespace.yaml", namespace, runner); err != nil {
		return false, err
	}

	if err := ki.kubectlApply("02_rbac.yaml", namespace, runner); err != nil {
		return false, err
	}
	return true, ki.applySecretWith(sshKey, caCertHash, certKey, namespace, runner)
}

func (ki *KubeadmInit) updateManifestNamespace(fileName, namespace string) ([]byte, error) {
	content, err := ki.manifestContent(fileName)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to open manifest")
	}
	c, err := manifest.WithNamespace(string(content), namespace)
	if err != nil {
		return nil, err
	}
	return []byte(c), nil
}

func (ki *KubeadmInit) kubectlApply(fileName, namespace string, runner plan.Runner) error {
	content, err := ki.updateManifestNamespace(fileName, namespace)
	if err != nil {
		return errors.Wrap(err, "Failed to upate manifest namespace")
	}
	return kubectlApply(runner, kubectlApplyArgs{Content: content}, fileName)
}

func (ki *KubeadmInit) manifestContent(fileName string) ([]byte, error) {
	file, err := manifests.Manifests.Open(fileName)
	if err != nil {
		return nil, err
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func (ki *KubeadmInit) applySecretWith(sshKey []byte, discoveryTokenCaCertHash, certKey, namespace string, runner plan.Runner) error {
	log.Info("adding SSH key to WKS secret and applying its manifest")
	fileName := "03_secrets.yaml"
	secret, err := ki.deserializeSecret(fileName, namespace)
	if err != nil {
		return err
	}
	secret.Data["sshKey"] = sshKey
	secret.Data["discoveryTokenCaCertHash"] = []byte(discoveryTokenCaCertHash)
	secret.Data["certificateKey"] = []byte(certKey)
	// We only store the ID as a Secret object containing the bootstrap token's
	// secret is already created by kubeadm init under:
	//   kube-system/bootstrap-token-$ID
	secret.Data["bootstrapTokenID"] = []byte(ki.BootstrapToken.ID)
	bytes, err := yaml.Marshal(secret)
	if err != nil {
		return errors.Wrap(err, "failed to serialize manifest")
	}
	return kubectlApply(runner, kubectlApplyArgs{Content: bytes}, fileName)
}

func (ki *KubeadmInit) deserializeSecret(fileName, namespace string) (*corev1.Secret, error) {
	content, err := ki.updateManifestNamespace(fileName, namespace)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to upate manifest namespace")
	}
	secret := &corev1.Secret{}
	if err = yaml.Unmarshal(content, secret); err != nil {
		return nil, errors.Wrap(err, "failed to deserialize manifest")
	}
	return secret, nil
}

// Undo implements plan.Resource.
func (ki *KubeadmInit) Undo(runner plan.Runner, current plan.State) error {
	remotePath := "/tmp/wks_kubeadm_init_config.yaml"
	var ignored string
	return buildKubeadmInitPlan(
		remotePath,
		strings.Join(ki.IgnorePreflightErrors, ","),
		ki.UseIPTables, &ignored).Undo(runner, plan.EmptyState)
}

func buildKubeadmInitPlan(path string, ignorePreflightErrors string, useIPTables bool, output *string) plan.Resource {
	b := plan.NewBuilder()
	if useIPTables {
		b.AddResource(
			"configure:iptables",
			&Run{Script: object.String("sysctl net.bridge.bridge-nf-call-iptables=1")}) // TODO: undo?
	}

	b.AddResource(
		"kubeadm:reset",
		&Run{Script: object.String("kubeadm reset --force")},
	).AddResource(
		"kubeadm:config:images",
		&Run{Script: plan.ParamString("kubeadm config images pull --config=%s", &path)},
		plan.DependOn("kubeadm:reset"),
	).AddResource(
		"kubeadm:run-init",
		// N.B.: --experimental-upload-certs encrypts & uploads
		// certificates of the primary control plane in the kubeadm-certs
		// Secret, and prints the value for --certificate-key to STDOUT.
		&Run{Script: plan.ParamString(
			withoutProxy("kubeadm init --config=%s --ignore-preflight-errors=%s --experimental-upload-certs"),
			&path, &ignorePreflightErrors),
			UndoResource: buildKubeadmRunInitUndoPlan(),
			Output:       output,
		},
		plan.DependOn("kubeadm:config:images"),
	)

	var homedir string

	b.AddResource(
		"kubeadm:get-homedir",
		&Run{Script: object.String("echo -n $HOME"), Output: &homedir},
	).AddResource(
		"kubeadm:config:kubectl-dir",
		&Dir{Path: plan.ParamString("%s/.kube", &homedir)},
		plan.DependOn("kubeadm:get-homedir"),
	).AddResource(
		"kubeadm:config:copy",
		&Run{Script: plan.ParamString("cp /etc/kubernetes/admin.conf %s/.kube/config", &homedir)},
		plan.DependOn("kubeadm:run-init", "kubeadm:config:kubectl-dir"),
	).AddResource(
		"kubeadm:config:set-ownership",
		&Run{Script: plan.ParamString("chown -R $(id -u):$(id -g) %s/.kube", &homedir)},
		plan.DependOn("kubeadm:config:copy"),
	)

	p, err := b.Plan()
	if err != nil {
		log.Fatalf("%v", err)
	}
	return &p
}

func buildKubeadmRunInitUndoPlan() plan.Resource {
	b := plan.NewBuilder()
	b.AddResource(
		"file:kube-apiserver.yaml",
		&File{Destination: "/etc/kubernetes/manifests/kube-apiserver.yaml"},
	).AddResource(
		"file:kube-controller-manager.yaml",
		&File{Destination: "/etc/kubernetes/manifests/kube-controller-manager.yaml"},
	).AddResource(
		"file:kube-scheduler.yaml",
		&File{Destination: "/etc/kubernetes/manifests/kube-scheduler.yaml"},
	).AddResource(
		"file:etcd.yaml",
		&File{Destination: "/etc/kubernetes/manifests/etcd.yaml"},
	).AddResource(
		"dir:etcd",
		&Dir{Path: object.String("/var/lib/etcd"), RecursiveDelete: true},
	)
	p, err := b.Plan()
	if err != nil {
		log.Fatalf("%v", err)
	}
	return &p
}
