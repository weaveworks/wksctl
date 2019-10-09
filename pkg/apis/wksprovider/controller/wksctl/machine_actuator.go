package wks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	goos "os"
	"path/filepath"
	"time"

	gerrors "github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/footloose/pkg/cluster"
	fconfig "github.com/weaveworks/footloose/pkg/config"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/os"
	baremetalspecv1 "github.com/weaveworks/wksctl/pkg/baremetalproviderspec/v1alpha1"
	machineutil "github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/kubernetes/drain"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	bootstraputils "github.com/weaveworks/wksctl/pkg/utilities/kubeadm"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	planKey          string = "wkp.weave.works/node-plan"
	masterLabel      string = "node-role.kubernetes.io/master"
	controllerSecret string = "wks-controller-secrets"
	bootstrapTokenID string = "bootstrapTokenID"
)

const clusterName = "firekube"

var footlooseAddr = "<unknown>"
var footlooseBackend = "docker"
var machineIPs = map[string]string{}

// STOPGAP: copy of machine def from footloose; the footloose version has private fields
type FootlooseMachine struct {
	Spec *fconfig.Machine `json:"spec"`

	// container name.
	Name string `json:"name"`
	// container hostname.
	Hostname string `json:"hostname"`
	// container ip.
	IP string `json:"ip,omitempty"`

	RuntimeNetworks []*cluster.RuntimeNetwork `json:"runtimeNetworks,omitempty"`
	// Fields that are cached from the docker daemon.

	Ports map[int]int `json:"ports,omitempty"`
	// maps containerPort -> hostPort.
}

// MachineActuator is responsible for managing this cluster's machines, and
// ensuring their state converge towards their definitions.
type MachineActuator struct {
	client              client.Client
	clientSet           *kubernetes.Clientset
	codec               *baremetalspecv1.BareMetalProviderSpecCodec
	controllerNamespace string
	eventRecorder       record.EventRecorder
	verbose             bool
}

// Create the machine.
func (a *MachineActuator) Create(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	contextLog := log.WithFields(log.Fields{"context": ctx, "cluster": *cluster, "machine": *machine})
	contextLog.Info("creating machine...")
	if err := a.create(ctx, cluster, machine); err != nil {
		contextLog.Errorf("failed to create machine: %v", err)
		return err
	}
	return nil
}

func (a *MachineActuator) create(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	c, m, err := a.parse(cluster, machine)
	if err != nil {
		return err
	}
	installer, closer, err := a.connectTo(machine, c, m)
	if err != nil {
		return gerrors.Wrapf(err, "failed to establish connection to machine %s", machine.Name)
	}
	defer closer.Close()
	// Bootstrap - set plan on seed node if not present before any updates can occur
	if err = a.initializeMasterPlanIfNecessary(installer); err != nil {
		return err
	}
	// Also, update footloose IP from env
	log.Infof("FETCHING FOOTLOOSE ADDRESS...")
	fip := goos.Getenv("FOOTLOOSE_SERVER_ADDR")
	if fip != "" {
		footlooseAddr = fip
	}
	backend := goos.Getenv("FOOTLOOSE_BACKEND")
	if backend != "" {
		footlooseBackend = backend
	}
	log.Infof("FOOTLOOSE ADDR: %s", footlooseAddr)
	log.Infof("FOOTLOOSE BACKEND: %s", footlooseBackend)
	nodePlan, err := a.getNodePlan(c, machine, a.getMachineAddress(machine), installer)
	if err != nil {
		return err
	}
	if err := installer.SetupNode(nodePlan); err != nil {
		return gerrors.Wrapf(err, "failed to set up machine %s", machine.Name)
	}
	ids, err := installer.IDs()
	if err != nil {
		return gerrors.Wrapf(err, "failed to read machine %s's IDs", machine.Name)
	}
	node, err := a.findNodeByID(ids.MachineID, ids.SystemUUID)
	if err != nil {
		return err
	}
	if err = a.setNodeAnnotation(node, planKey, nodePlan.ToJSON()); err != nil {
		return err
	}
	a.recordEvent(machine, corev1.EventTypeNormal, "Create", "created machine %s", machine.Name)
	return nil
}

// We set the plan annotation for a seed node at the first create of another mode so we
// don't miss any updates. The plan is derived from the original seed node plan and stored in a config map
// for use by the actuator.
func (a *MachineActuator) initializeMasterPlanIfNecessary(installer *os.OS) error {
	master, err := a.getMasterNode()
	if err != nil {
		return err
	}
	if master.Annotations[planKey] == "" {
		client := a.clientSet.CoreV1().ConfigMaps(a.controllerNamespace)
		configMap, err := client.Get(os.SeedNodePlanName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		seedNodePlanParams := configMap.BinaryData["plan"]
		var params os.NodeParams
		err = gob.NewDecoder(bytes.NewReader(seedNodePlanParams)).Decode(&params)
		if err != nil {
			return err
		}
		seedNodeStandardNodePlan, err := installer.CreateNodeSetupPlan(params)
		if err != nil {
			return err
		}
		if err = a.setNodeAnnotation(master, planKey, seedNodeStandardNodePlan.ToJSON()); err != nil {
			return err
		}
	}
	return nil
}

func (a *MachineActuator) parse(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (*baremetalspecv1.BareMetalClusterProviderSpec, *baremetalspecv1.BareMetalMachineProviderSpec, error) {
	c, err := a.codec.ClusterProviderFromProviderSpec(cluster.Spec.ProviderSpec)
	if err != nil {
		return nil, nil, gerrors.Wrapf(err, "failed to parse cluster %v", cluster.Spec.ProviderSpec)
	}
	m, err := a.codec.MachineProviderFromProviderSpec(machine.Spec.ProviderSpec)
	if err != nil {
		return nil, nil, gerrors.Wrapf(err, "failed to parse machine %v", machine.Spec.ProviderSpec)
	}
	return c, m, nil
}

func (a *MachineActuator) connectTo(machine *clusterv1.Machine, c *baremetalspecv1.BareMetalClusterProviderSpec, m *baremetalspecv1.BareMetalMachineProviderSpec) (*os.OS, io.Closer, error) {
	sshKey, err := a.sshKey()
	if err != nil {
		return nil, nil, gerrors.Wrap(err, "failed to read SSH key")
	}
	sshClient, err := ssh.NewClient(ssh.ClientParams{
		User:         c.User,
		Host:         a.getMachineAddress(machine),
		Port:         m.Private.Port,
		PrivateKey:   sshKey,
		PrintOutputs: a.verbose,
	})
	if err != nil {
		return nil, nil, gerrors.Wrapf(err, "failed to create SSH client using %v", m.Private)
	}
	os, err := os.Identify(sshClient)
	if err != nil {
		return nil, nil, gerrors.Wrapf(err, "failed to identify machine %s's operating system", a.getMachineAddress(machine))
	}
	return os, sshClient, nil
}

func (a *MachineActuator) sshKey() ([]byte, error) {
	secret, err := a.clientSet.CoreV1().Secrets(a.controllerNamespace).Get(controllerSecret, metav1.GetOptions{})
	if err != nil {
		return nil, gerrors.Wrap(err, "failed to get WKS' secret")
	}
	return secret.Data["sshKey"], nil
}

// kubeadmJoinSecrets groups the values available in the wks-controller-secrets
// Secret to provide to kubeadm join commands.
type kubeadmJoinSecrets struct {
	// DiscoveryTokenCaCertHash is used to validate that the root CA public key
	// of the cluster we are trying to join matches.
	DiscoveryTokenCaCertHash string
	// BootstrapTokenID is the ID of the token used by kubeadm init and kubeadm
	// join to safely form new clusters.
	BootstrapTokenID string
	// CertificateKey is used by kubeadm --certificate-key to have other master
	// nodes safely join the cluster.
	CertificateKey string
}

func (a *MachineActuator) kubeadmJoinSecrets() (*kubeadmJoinSecrets, error) {
	secret, err := a.clientSet.CoreV1().Secrets(a.controllerNamespace).Get(controllerSecret, metav1.GetOptions{})
	if err != nil {
		return nil, gerrors.Wrap(err, "failed to get WKS' secret")
	}
	return &kubeadmJoinSecrets{
		DiscoveryTokenCaCertHash: string(secret.Data["discoveryTokenCaCertHash"]),
		BootstrapTokenID:         string(secret.Data[bootstrapTokenID]),
		CertificateKey:           string(secret.Data["certificateKey"]),
	}, nil
}

func (a *MachineActuator) updateKubeadmJoinSecrets(ID string) error {
	len := base64.StdEncoding.EncodedLen(len(ID))
	enc := make([]byte, len)
	base64.StdEncoding.Encode(enc, []byte(ID))
	patch := []byte(fmt.Sprintf("{\"data\":{\"%s\":\"%s\"}}", bootstrapTokenID, enc))
	_, err := a.clientSet.CoreV1().Secrets(a.controllerNamespace).Patch(controllerSecret, types.StrategicMergePatchType, patch)
	if err != nil {
		log.Debugf("failed to patch wks secret %s %v", patch, err)
	}
	return err
}

func (a *MachineActuator) token(ID string) (string, error) {
	ns := "kube-system"
	name := fmt.Sprintf("%s%s", bootstrapapi.BootstrapTokenSecretPrefix, ID)
	secret, err := a.clientSet.CoreV1().Secrets(ns).Get(name, metav1.GetOptions{})
	if err != nil {
		// The secret may have been removed if it expired so we will generate a new one
		log.Debugf("failed to find original bootstrap token %s/%s, generating a new one", ns, name)
		newSecret, err := a.installNewBootstrapToken(ns)
		if err != nil {
			return "", gerrors.Wrapf(err, "failed to find old secret %s/%s or generate a new one", ns, name)
		}
		secret = newSecret
	} else {
		if bootstrapTokenHasExpired(secret) {
			newSecret, err := a.installNewBootstrapToken(ns)
			if err != nil {
				return "", gerrors.Wrapf(err, "failed to replace expired secret %s/%s with a new one", ns, name)
			}
			secret = newSecret
		}
	}
	tokenID, ok := secret.Data[bootstrapapi.BootstrapTokenIDKey]
	if !ok {
		return "", gerrors.Errorf("token-id not found %s/%s", ns, name)
	}
	tokenSecret, ok := secret.Data[bootstrapapi.BootstrapTokenSecretKey]
	if !ok {
		return "", gerrors.Errorf("token-secret not found %s/%s", ns, name)
	}
	return fmt.Sprintf("%s.%s", tokenID, tokenSecret), nil
}

func bootstrapTokenHasExpired(secret *corev1.Secret) bool {
	// verify that the token hasn't expired
	expiration, ok := secret.Data[bootstrapapi.BootstrapTokenExpirationKey]
	if !ok {
		log.Debugf("expiration not found for secret %s/%s", secret.ObjectMeta.Namespace, secret.ObjectMeta.Name)
		return true
	}
	expirationTime, err := time.Parse(time.RFC3339, string(expiration))
	if err != nil {
		log.Debugf("failed to parse token expiration %s for secret %s/%s error %v", expiration, secret.ObjectMeta.Namespace, secret.ObjectMeta.Name, err)
		return true
	}
	// if the token expires within 60 seconds, we need to generate a new one
	return time.Until(expirationTime).Seconds() < 60
}
func (a *MachineActuator) installNewBootstrapToken(ns string) (*corev1.Secret, error) {
	secret, err := bootstraputils.GenerateBootstrapSecret(ns)
	if err != nil {
		return nil, gerrors.Errorf("failed to create new bootstrap token %s/%s", ns, secret.ObjectMeta.Name)
	}
	s, err := a.clientSet.CoreV1().Secrets(ns).Create(secret)
	if err != nil {
		return nil, gerrors.Errorf("failed to install new bootstrap token %s/%s", ns, secret.ObjectMeta.Name)
	}
	tokenID, ok := s.Data[bootstrapapi.BootstrapTokenIDKey]
	if !ok {
		return nil, gerrors.Errorf("token-id not found %s/%s", s.ObjectMeta.Namespace, s.ObjectMeta.Name)
	}
	if err := a.updateKubeadmJoinSecrets(string(tokenID)); err != nil {
		return nil, gerrors.Errorf("Failed to update wks join token %s/%s", s.ObjectMeta.Namespace, s.ObjectMeta.Name)
	}
	return s, nil
}

// Delete the machine. If no error is returned, it is assumed that all dependent resources have been cleaned up.
func (a *MachineActuator) Delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	contextLog := log.WithFields(log.Fields{"machine": machine.Name, "cluster": cluster.Name})
	contextLog.Info("deleting machine ...")
	if err := a.delete(ctx, cluster, machine); err != nil {
		contextLog.Errorf("failed to delete machine: %v", err)
		return err
	}
	return nil
}

func (a *MachineActuator) delete(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	c, m, err := a.parse(cluster, machine)
	if err != nil {
		return err
	}
	os, closer, err := a.connectTo(machine, c, m)
	if err != nil {
		return gerrors.Wrapf(err, "failed to establish connection to machine %s", machine.Name)
	}
	defer closer.Close()
	ids, err := os.IDs()
	if err != nil {
		return gerrors.Wrapf(err, "failed to read machine %s's IDs", machine.Name)
	}
	node, err := a.findNodeByID(ids.MachineID, ids.SystemUUID)
	if err != nil {
		return err
	}
	if err := drain.Drain(node, a.clientSet, drain.Params{
		Force:               true,
		DeleteLocalData:     true,
		IgnoreAllDaemonSets: true,
	}); err != nil {
		return err
	}
	if err = a.clientSet.CoreV1().Nodes().Delete(node.Name, &metav1.DeleteOptions{}); err != nil {
		return err
	}
	a.recordEvent(machine, corev1.EventTypeNormal, "Delete", "deleted machine %s", machine.Name)
	return nil
}

// Update the machine to the provided definition.
func (a *MachineActuator) Update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	contextLog := log.WithFields(log.Fields{"machine": machine.Name, "cluster": cluster.Name})
	contextLog.Info("updating machine...")
	if err := a.update(ctx, cluster, machine); err != nil {
		contextLog.Errorf("failed to update machine: %v", err)
		return err
	}
	return nil
}
func (a *MachineActuator) update(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) error {
	c, m, err := a.parse(cluster, machine)
	if err != nil {
		return err
	}
	installer, closer, err := a.connectTo(machine, c, m)
	if err != nil {
		return gerrors.Wrapf(err, "failed to establish connection to machine %s", machine.Name)
	}
	defer closer.Close()
	ids, err := installer.IDs()
	if err != nil {
		return gerrors.Wrapf(err, "failed to read machine %s's IDs", machine.Name)
	}
	node, err := a.findNodeByID(ids.MachineID, ids.SystemUUID)
	if err != nil {
		return err
	}
	contextLog := log.WithFields(log.Fields{"machine": machine.Name, "cluster": cluster.Name, "node": node.Name})
	nodePlan, err := a.getNodePlan(c, machine, a.getMachineAddress(machine), installer)
	if err != nil {
		return gerrors.Wrapf(err, "Failed to get node plan for machine %s", machine.Name)
	}
	planJSON := nodePlan.ToJSON()
	if node.Annotations[planKey] == planJSON {
		contextLog.Info("Machine and node have matching plans; nothing to do")
		return nil
	}
	nodeIsMaster := isMaster(node)
	if nodeIsMaster {
		if err := a.prepareForMasterUpdate(machine, node); err != nil {
			return err
		}
	} else if isUpOrDowngrade(machine, node) {
		if err = a.checkAnyMasterNotAtVersion(machineVersion(machine)); err != nil {
			return gerrors.Wrap(err, "Master nodes must be upgraded before worker nodes")
		}
	}
	if err = a.performActualUpdate(installer, machine, node, nodePlan, c); err != nil {
		// If the update of a master failed, reset the master label
		if nodeIsMaster {
			if seterr := a.setNodeLabel(node, masterLabel, ""); seterr != nil {
				return gerrors.Wrapf(err, "Could not reset master label after failed update: %v", seterr)
			}
		}
		return err
	}
	if err = a.setNodeAnnotation(node, planKey, planJSON); err != nil {
		return err
	}
	a.recordEvent(machine, corev1.EventTypeNormal, "Update", "updated machine %s", machine.Name)
	return nil
}

func (a *MachineActuator) prepareForMasterUpdate(machine *clusterv1.Machine, node *corev1.Node) error {
	// Check if it's safe to update a master
	if err := a.checkMasterHAConstraint(); err != nil {
		return gerrors.Wrap(err, "Not enough available master nodes to allow master update")
	}
	// If we update to a non-master, we need to remove the master label
	if err := a.removeNodeLabel(node, masterLabel); err != nil {
		return err
	}
	return nil
}

func (a *MachineActuator) performActualUpdate(
	installer *os.OS,
	machine *clusterv1.Machine,
	node *corev1.Node,
	nodePlan *plan.Plan,
	cluster *baremetalspecv1.BareMetalClusterProviderSpec) error {
	if err := drain.Drain(node, a.clientSet, drain.Params{
		Force:               true,
		DeleteLocalData:     true,
		IgnoreAllDaemonSets: true,
	}); err != nil {
		return err
	}
	if err := installer.SetupNode(nodePlan); err != nil {
		return gerrors.Wrapf(err, "failed to set up machine %s", machine.Name)
	}
	if err := a.uncordon(node); err != nil {
		return err
	}
	return nil
}

func (a *MachineActuator) getNodePlan(providerSpec *baremetalspecv1.BareMetalClusterProviderSpec, machine *clusterv1.Machine, machineAddress string, installer *os.OS) (*plan.Plan, error) {
	namespace := a.controllerNamespace
	secrets, err := a.kubeadmJoinSecrets()
	if err != nil {
		return nil, err
	}
	token, err := a.token(secrets.BootstrapTokenID)
	if err != nil {
		return nil, err
	}
	master, err := a.getMasterNode()
	if err != nil {
		return nil, err
	}
	masterIP, err := getInternalAddress(master)
	if err != nil {
		return nil, err
	}
	configMaps, err := a.getProviderConfigMaps(providerSpec)
	if err != nil {
		return nil, err
	}
	authConfigMap, err := a.getAuthConfigMap()
	if err != nil {
		return nil, err
	}
	plan, err := installer.CreateNodeSetupPlan(os.NodeParams{
		IsMaster:                 machine.Labels["set"] == "master",
		MasterIP:                 masterIP,
		MasterPort:               6443, // TODO: read this dynamically, from somewhere.
		Token:                    token,
		DiscoveryTokenCaCertHash: secrets.DiscoveryTokenCaCertHash,
		CertificateKey:           secrets.CertificateKey,
		KubeletConfig: config.KubeletConfig{
			NodeIP:        machineAddress,
			CloudProvider: providerSpec.CloudProvider,
		},
		KubernetesVersion:  machineutil.GetKubernetesVersion(machine),
		CRI:                providerSpec.CRI,
		ConfigFileSpecs:    providerSpec.OS.Files,
		ProviderConfigMaps: configMaps,
		AuthConfigMap:      authConfigMap,
		Namespace:          namespace,
	})
	if err != nil {
		return nil, gerrors.Wrapf(err, "failed to create machine plan for %s", machine.Name)
	}
	return plan, nil
}

func (a *MachineActuator) getAuthConfigMap() (*v1.ConfigMap, error) {
	client := a.clientSet.CoreV1().ConfigMaps(a.controllerNamespace)
	maps, err := client.List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, cmap := range maps.Items {
		if cmap.Name == "auth-config" {
			return &cmap, nil
		}
	}
	return nil, nil
}

func (a *MachineActuator) getProviderConfigMaps(providerSpec *baremetalspecv1.BareMetalClusterProviderSpec) (map[string]*v1.ConfigMap, error) {
	fileSpecs := providerSpec.OS.Files
	client := a.clientSet.CoreV1().ConfigMaps(a.controllerNamespace)
	configMaps := map[string]*v1.ConfigMap{}
	for _, fileSpec := range fileSpecs {
		mapName := fileSpec.Source.ConfigMap
		if _, seen := configMaps[mapName]; !seen {
			configMap, err := client.Get(mapName, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			configMaps[mapName] = configMap
		}
	}
	return configMaps, nil
}

func isUpOrDowngrade(machine *clusterv1.Machine, node *corev1.Node) bool {
	return machineVersion(machine) != nodeVersion(node)
}

func (a *MachineActuator) checkAnyMasterNotAtVersion(kubernetesVersion string) error {
	nodes, err := a.getMasterNodes()
	if err != nil {
		// If we can't read the nodes, return the error so we don't
		// accidentally flush the sole master
		return err
	}

	for _, master := range nodes {
		if nodeVersion(master) != kubernetesVersion {
			return gerrors.Errorf("Master node: %s has not been upgraded", master.Name)
		}
	}
	return nil
}

func machineVersion(machine *clusterv1.Machine) string {
	return "v" + machineutil.GetKubernetesVersion(machine)
}

func nodeVersion(node *corev1.Node) string {
	return node.Status.NodeInfo.KubeletVersion
}

func (a *MachineActuator) uncordon(node *corev1.Node) error {
	contextLog := log.WithFields(log.Fields{"node": node.Name})
	client := a.clientSet.CoreV1().Nodes()
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, getErr := client.Get(node.Name, metav1.GetOptions{})
		if getErr != nil {
			contextLog.Errorf("failed to read node info, can't reschedule: %v", getErr)
			return getErr
		}
		result.Spec.Unschedulable = false
		_, updateErr := client.Update(result)
		if updateErr != nil {
			contextLog.Errorf("failed to reschedule node: %v", updateErr)
			return updateErr
		}
		return nil
	})
	if retryErr != nil {
		contextLog.Errorf("failed to reschedule node: %v", retryErr)
		return retryErr
	}
	return nil
}

func (a *MachineActuator) setNodeAnnotation(node *corev1.Node, key, value string) error {
	err := a.modifyNode(node, func(node *corev1.Node) {
		node.Annotations[key] = value
	})
	if err != nil {
		return gerrors.Wrapf(err, "Failed to set node annotation: %s for node: %s", key, node.Name)
	}
	return nil
}

func (a *MachineActuator) setNodeLabel(node *corev1.Node, label, value string) error {
	err := a.modifyNode(node, func(node *corev1.Node) {
		node.Labels[label] = value
	})
	if err != nil {
		return gerrors.Wrapf(err, "Failed to set node label: %s for node: %s", label, node.Name)
	}
	return nil
}

func (a *MachineActuator) removeNodeLabel(node *corev1.Node, label string) error {
	err := a.modifyNode(node, func(node *corev1.Node) {
		delete(node.Labels, label)
	})
	if err != nil {
		return gerrors.Wrapf(err, "Failed to remove node label: %s for node: %s", label, node.Name)
	}
	return nil
}

func (a *MachineActuator) modifyNode(node *corev1.Node, updater func(node *corev1.Node)) error {
	contextLog := log.WithFields(log.Fields{"node": node.Name})
	client := a.clientSet.CoreV1().Nodes()
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, getErr := client.Get(node.Name, metav1.GetOptions{})
		if getErr != nil {
			contextLog.Errorf("failed to read node info, assuming unsafe to update: %v", getErr)
			return getErr
		}
		updater(result)
		_, updateErr := client.Update(result)
		if updateErr != nil {
			contextLog.Errorf("failed attempt to update node annotation: %v", updateErr)
			return updateErr
		}
		return nil
	})
	if retryErr != nil {
		contextLog.Errorf("failed to update node annotation: %v", retryErr)
		return gerrors.Wrapf(retryErr, "Could not mark node %s as updated", node.Name)
	}
	return nil
}

func (a *MachineActuator) checkMasterHAConstraint() error {
	nodes, err := a.getMasterNodes()
	if err != nil {
		// If we can't read the nodes, return the error so we don't
		// accidentally flush the sole master
		return err
	}
	availableMasters := 0
	for _, node := range nodes {
		if hasConditionTrue(node, corev1.NodeReady) && !hasTaint(node, "NoSchedule") {
			availableMasters++
		}
	}
	if availableMasters < 2 {
		return errors.New("Fewer than two master nodes available")
	}
	return nil
}

func hasConditionTrue(node *corev1.Node, typ corev1.NodeConditionType) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == typ && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func hasTaint(node *corev1.Node, value string) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Value == value {
			return true
		}
	}
	return false
}

// Exists checks if the machine currently exists.
func (a *MachineActuator) Exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	contextLog := log.WithFields(log.Fields{"machine": machine.Name, "cluster": cluster.Name})
	contextLog.Info("checking existence of machine...")
	exists, err := a.exists(ctx, cluster, machine)
	if err != nil {
		contextLog.Errorf("failed to check existence of machine: %s", err)
		return false, err
	}
	return exists, nil
}

func (a *MachineActuator) exists(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) (bool, error) {
	c, m, err := a.parse(cluster, machine)
	if err != nil {
		return false, err
	}
	contextLog := log.WithFields(log.Fields{"machine": machine.Name})
	contextLog.Infof("M: %#v", m)
	// In a managed environment, machine IPs are added by an underlying controller; if no IP is currently present, wait
	// for the IP to be added before operating on the machine
	if a.getMachineAddress(machine) == "" {
		contextLog.Info("Creating underlying machine")
		ip, err := invokeFootlooseCreate(machine)
		if err != nil {
			return false, err
		}
		contextLog.Infof("Created underlying machine: %s", ip)
		a.updateMachine(machine, ip)
		contextLog.Infof("Updated machine: %s", ip)
	}
	os, closer, err := a.connectTo(machine, c, m)
	if err != nil {
		return false, gerrors.Wrapf(err, "failed to establish connection to machine %s", machine.Name)
	}
	defer closer.Close()
	ids, err := os.IDs()
	if err != nil {
		return false, gerrors.Wrapf(err, "failed to read machine %s's IDs", machine.Name)
	}
	node, err := a.findNodeByID(ids.MachineID, ids.SystemUUID)
	if err != nil {
		// If the error is that the machine was not found; record the event. Otherwise, error out
		if apierrs.IsNotFound(err) {
			a.recordEvent(machine, corev1.EventTypeNormal, "Exists", "machine %s (%s ; %s) is not a node", machine.Name, ids.SystemUUID, ids.MachineID)
			return false, nil
		}
		return false, err
	}
	a.recordEvent(machine, corev1.EventTypeNormal, "Exists", "machine %s (%s ; %s) is a node (%s)", machine.Name, ids.SystemUUID, ids.MachineID, node.Name)
	return true, nil
}

func (a *MachineActuator) findNodeByID(machineID, systemUUID string) (*corev1.Node, error) {
	nodes, err := a.clientSet.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, gerrors.Wrap(err, "failed to list nodes")
	}
	for _, node := range nodes.Items {
		if node.Status.NodeInfo.MachineID == machineID && node.Status.NodeInfo.SystemUUID == systemUUID {
			return &node, nil
		}
	}
	return nil, apierrs.NewNotFound(schema.GroupResource{Group: "", Resource: "nodes"}, "")
}

var r = rand.New(rand.NewSource(time.Now().Unix()))

func (a *MachineActuator) getMasterNode() (*corev1.Node, error) {
	masters, err := a.getMasterNodes()
	if err != nil {
		return nil, err
	}
	if len(masters) == 0 {
		return nil, errors.New("no master node found")
	}
	// Randomise to limit chances of always hitting the same master node:
	index := r.Intn(len(masters))
	return masters[index], nil
}

func (a *MachineActuator) getMasterNodes() ([]*corev1.Node, error) {
	nodes, err := a.clientSet.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, gerrors.Wrap(err, "failed to list nodes")
	}
	masters := []*corev1.Node{}
	for _, node := range nodes.Items {
		if isMaster(&node) {
			n := node
			masters = append(masters, &n)
		}
	}
	return masters, nil
}

func (a *MachineActuator) updateMachine(machine *clusterv1.Machine, ip string) {
	machineIPs[getMachineID(machine)] = ip
}

func getMachineName(uri string) string {
	return filepath.Base(uri)
}

func getFootlooseMachineIP(uri string) (string, error) {
	machineName := getMachineName(uri)
	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Opaque: fmt.Sprintf("/api/clusters/%s/machines/%s", clusterName, machineName),
			Scheme: "http",
			Host:   footlooseAddr,
		},
		Close: true,
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Error retrieving footloose machine: %v\n", err)
	}
	defer resp.Body.Close()
	var m FootlooseMachine
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return "", err
	}
	nets := m.RuntimeNetworks
	for _, net := range nets {
		if net.Name == "bridge" {
			return net.IP, nil
		}
	}
	return "", fmt.Errorf("Could not find bridge network for machine: %s", machineName)
}

func invokeFootlooseCreate(machine *clusterv1.Machine) (string, error) {
	params := map[string]interface{}{
		"name":       machine.Name,
		"image":      "quay.io/footloose/centos7:0.6.1",
		"privileged": true,
		"backend":    footlooseBackend,
	}
	postdata, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	resp, err := http.Post(fmt.Sprintf("http://%s/api/clusters/%s/machines", footlooseAddr, clusterName),
		"application/json", bytes.NewReader(postdata))
	if err != nil {
		return "", fmt.Errorf("Error creating footloose machine: %v\n", err)
	} else {
		defer resp.Body.Close()
	}
	m := map[string]interface{}{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return "", err
	}
	uri, ok := m["uri"]
	if !ok {
		uri = []byte(fmt.Sprintf("http://%s/api/clusters/%s/machines/%s", footlooseAddr, clusterName, machine.Name))
	}
	ustr, ok := uri.(string)
	if !ok {
		return "", fmt.Errorf("Invalid uri for: %s", machine.Name)
	}
	return getFootlooseMachineIP(ustr)
}

func isMaster(node *corev1.Node) bool {
	_, isMaster := node.Labels["node-role.kubernetes.io/master"]
	return isMaster
}

func getInternalAddress(node *corev1.Node) (string, error) {
	for _, address := range node.Status.Addresses {
		if address.Type == "InternalIP" {
			return address.Address, nil
		}
	}
	return "", errors.New("no InternalIP address found")
}

func (a *MachineActuator) recordEvent(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
	a.eventRecorder.Eventf(object, eventType, reason, messageFmt, args...)
	switch eventType {
	case corev1.EventTypeWarning:
		log.Warnf(messageFmt, args...)
	case corev1.EventTypeNormal:
		log.Infof(messageFmt, args...)
	default:
		log.Debugf(messageFmt, args...)
	}
}

func getMachineID(machine *clusterv1.Machine) string {
	return machine.Namespace + ":" + machine.Name
}

func (a *MachineActuator) getMachineAddress(machine *clusterv1.Machine) string {
	m, _ := a.codec.MachineProviderFromProviderSpec(machine.Spec.ProviderSpec)

	if m.Private.Address != "" {
		return m.Private.Address
	}
	return machineIPs[getMachineID(machine)]
}

// MachineActuatorParams groups required inputs to create a machine actuator.
type MachineActuatorParams struct {
	Client              client.Client
	ClientSet           *kubernetes.Clientset
	ControllerNamespace string
	EventRecorder       record.EventRecorder
	Scheme              *runtime.Scheme
	Verbose             bool
}

// NewMachineActuator creates a new Machine actuator.
func NewMachineActuator(params MachineActuatorParams) (*MachineActuator, error) {
	codec, err := baremetalspecv1.NewCodec()
	if err != nil {
		return nil, gerrors.Wrap(err, "failed to create codec")
	}
	return &MachineActuator{
		client:              params.Client,
		clientSet:           params.ClientSet,
		codec:               codec,
		controllerNamespace: params.ControllerNamespace,
		eventRecorder:       params.EventRecorder,
		verbose:             params.Verbose,
	}, nil
}
