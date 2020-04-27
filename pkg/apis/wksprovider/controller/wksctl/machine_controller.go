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
	"regexp"
	"time"

	"github.com/chanwit/plandiff"
	gerrors "github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/footloose/pkg/cluster"
	fconfig "github.com/weaveworks/footloose/pkg/config"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/config"
	"github.com/weaveworks/wksctl/pkg/apis/wksprovider/machine/os"
	baremetalspecv1 "github.com/weaveworks/wksctl/pkg/baremetal/v1alpha3"
	machineutil "github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/kubernetes/drain"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/resource"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/specs"
	bootstraputils "github.com/weaveworks/wksctl/pkg/utilities/kubeadm"
	"github.com/weaveworks/wksctl/pkg/utilities/object"
	"github.com/weaveworks/wksctl/pkg/utilities/version"
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
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	planKey             string = "wks.weave.works/node-plan"
	masterLabel         string = "node-role.kubernetes.io/master"
	originalMasterLabel string = "wks.weave.works/original-master"
	controllerName      string = "wks-controller"
	controllerSecret    string = "wks-controller-secrets"
	bootstrapTokenID    string = "bootstrapTokenID"
	clusterName         string = "wks-firekube"
)

type nodeType int

const (
	originalMaster nodeType = iota
	secondaryMaster
	worker
)

var (
	footlooseAddr    = "<unknown>"
	footlooseBackend = "docker"
	machineIPs       = map[string]string{}
	hostAddrRegexp   = regexp.MustCompile(`(?m)controlPlaneEndpoint[:]\s*([^:\s]+)`)
)

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

// TODO: should this be renamed 'reconciler' to match other CAPI providers ?

// MachineController is responsible for managing this cluster's machines, and
// ensuring their state converge towards their definitions.
type MachineController struct {
	client              client.Client
	clientSet           *kubernetes.Clientset
	controllerNamespace string
	eventRecorder       record.EventRecorder
	verbose             bool
}

func (r *MachineController) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.TODO() // upstream will add this eventually
	contextLog := log.WithField("name", req.NamespacedName)

	// request only contains the name of the object, so fetch it from the api-server
	bmm := &baremetalspecv1.BareMetalMachine{}
	err := r.client.Get(ctx, req.NamespacedName, bmm)
	if err != nil {
		if apierrs.IsNotFound(err) { // isn't there; give in
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Get Machine via OwnerReferences
	machine, err := util.GetOwnerMachine(ctx, r.client, bmm.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		contextLog.Info("Machine Controller has not yet set ownerReferences")
		return ctrl.Result{}, nil
	}
	contextLog = contextLog.WithField("machine", machine.Name)

	// Get Cluster via label "cluster.x-k8s.io/cluster-name"
	cluster, err := util.GetClusterFromMetadata(ctx, r.client, machine.ObjectMeta)
	if err != nil {
		contextLog.Info("Machine is missing cluster label or cluster does not exist")
		return ctrl.Result{}, nil
	}

	if util.IsPaused(cluster, bmm) {
		contextLog.Info("BareMetalMachine or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}
	contextLog = contextLog.WithField("cluster", cluster.Name)

	// Now go from the Cluster to the BareMetalCluster
	bmc := &baremetalspecv1.BareMetalCluster{}
	if err := r.client.Get(ctx, client.ObjectKey{
		Namespace: bmm.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}, bmc); err != nil {
		contextLog.Info("BareMetalCluster is not available yet")
		return ctrl.Result{}, nil
	}

	// Object still there but with deletion timestamp => run our finalizer
	if !bmm.ObjectMeta.DeletionTimestamp.IsZero() {
		err := r.delete(ctx, bmc, machine, bmm)
		if err != nil {
			contextLog.Errorf("failed to delete machine: %v", err)
		}
		return ctrl.Result{}, err
	}

	// FIXME!  assuming everything else is create
	{
		err := r.create(ctx, bmc, machine, bmm)
		if err != nil {
			contextLog.Errorf("failed to create machine: %v", err)
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (a *MachineController) create(ctx context.Context, c *baremetalspecv1.BareMetalCluster, machine *clusterv1.Machine, bmm *baremetalspecv1.BareMetalMachine) error {
	contextLog := log.WithFields(log.Fields{"context": ctx, "cluster": *c, "machine": *machine})
	contextLog.Info("creating machine...")

	installer, closer, err := a.connectTo(c, bmm)
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
	nodePlan, err := a.getNodePlan(c, machine, a.getMachineAddress(bmm), installer)
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
func (a *MachineController) initializeMasterPlanIfNecessary(installer *os.OS) error {

	// we also use this method to mark the first master as the "originalMaster"
	originalMasterNode, err := a.getOriginalMasterNode()
	if err != nil {
		return err
	}

	if originalMasterNode.Annotations[planKey] == "" {
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
		if err = a.setNodeAnnotation(originalMasterNode, planKey, seedNodeStandardNodePlan.ToJSON()); err != nil {
			return err
		}
	}
	return nil
}

func (a *MachineController) connectTo(c *baremetalspecv1.BareMetalCluster, m *baremetalspecv1.BareMetalMachine) (*os.OS, io.Closer, error) {
	sshKey, err := a.sshKey()
	if err != nil {
		return nil, nil, gerrors.Wrap(err, "failed to read SSH key")
	}
	sshClient, err := ssh.NewClient(ssh.ClientParams{
		User:         c.User,
		Host:         a.getMachineAddress(m),
		Port:         m.Private.Port,
		PrivateKey:   sshKey,
		PrintOutputs: a.verbose,
	})
	if err != nil {
		return nil, nil, gerrors.Wrapf(err, "failed to create SSH client using %v", m.Private)
	}
	os, err := os.Identify(sshClient)
	if err != nil {
		return nil, nil, gerrors.Wrapf(err, "failed to identify machine %s's operating system", a.getMachineAddress(m))
	}
	return os, sshClient, nil
}

func (a *MachineController) sshKey() ([]byte, error) {
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

func (a *MachineController) kubeadmJoinSecrets() (*kubeadmJoinSecrets, error) {
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

func (a *MachineController) updateKubeadmJoinSecrets(ID string) error {
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

func (a *MachineController) token(ID string) (string, error) {
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
func (a *MachineController) installNewBootstrapToken(ns string) (*corev1.Secret, error) {
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
func (a *MachineController) delete(ctx context.Context, c *baremetalspecv1.BareMetalCluster, machine *clusterv1.Machine, bmm *baremetalspecv1.BareMetalMachine) error {
	contextLog := log.WithFields(log.Fields{"machine": machine.Name, "cluster": c.Name})
	contextLog.Info("deleting machine ...")

	os, closer, err := a.connectTo(c, bmm)
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
func (a *MachineController) update(ctx context.Context, c *baremetalspecv1.BareMetalCluster, machine *clusterv1.Machine, bmm *baremetalspecv1.BareMetalMachine) error {
	contextLog := log.WithFields(log.Fields{"machine": machine.Name, "cluster": c.Name})
	contextLog.Info("updating machine...")
	installer, closer, err := a.connectTo(c, bmm)
	if err != nil {
		return gerrors.Wrapf(err, "failed to establish connection to machine %s", machine.Name)
	}
	defer closer.Close()

	// Bootstrap - set plan on seed node if not present before any updates can occur
	if err := a.initializeMasterPlanIfNecessary(installer); err != nil {
		return err
	}
	ids, err := installer.IDs()
	if err != nil {
		return gerrors.Wrapf(err, "failed to read machine %s's IDs", machine.Name)
	}
	node, err := a.findNodeByID(ids.MachineID, ids.SystemUUID)
	if err != nil {
		return gerrors.Wrapf(err, "failed to find node by id: %s/%s", ids.MachineID, ids.SystemUUID)
	}
	contextLog = contextLog.WithFields(log.Fields{"node": node.Name})
	nodePlan, err := a.getNodePlan(c, machine, a.getMachineAddress(bmm), installer)
	if err != nil {
		return gerrors.Wrapf(err, "Failed to get node plan for machine %s", machine.Name)
	}
	planJSON := nodePlan.ToJSON()
	currentPlan := node.Annotations[planKey]
	if currentPlan == planJSON {
		contextLog.Info("Machine and node have matching plans; nothing to do")
		return nil
	}

	if diffedPlan, err := plandiff.GetUnifiedDiff(currentPlan, planJSON); err == nil {
		contextLog.Info("........................ DIFF PLAN ........................")
		fmt.Print(diffedPlan)
	} else {
		contextLog.Errorf("DIFF PLAN Error: %v", err)
	}

	contextLog.Infof("........................NEW UPDATE FOR: %s...........................", machine.Name)
	isMaster := isMaster(node)
	if isMaster {
		if err := a.prepareForMasterUpdate(); err != nil {
			return err
		}
	}
	upOrDowngrade := isUpOrDowngrade(machine, node)
	contextLog.Infof("Is master: %t, is up or downgrade: %t", isMaster, upOrDowngrade)
	if upOrDowngrade {
		if err := checkForVersionJump(machine, node); err != nil {
			return err
		}
		version := machineutil.GetKubernetesVersion(machine)
		nodeStyleVersion := "v" + version
		originalNeedsUpdate, err := a.checkIfOriginalMasterNotAtVersion(nodeStyleVersion)
		if err != nil {
			return err
		}
		contextLog.Infof("Original needs update: %t", originalNeedsUpdate)
		masterNeedsUpdate, err := a.checkIfMasterNotAtVersion(nodeStyleVersion)
		if err != nil {
			return err
		}
		contextLog.Infof("Master needs update: %t", masterNeedsUpdate)
		isOriginal, err := a.isOriginalMaster(node)
		if err != nil {
			return err
		}
		contextLog.Infof("Is original: %t", isOriginal)
		if (!isOriginal && originalNeedsUpdate) || (!isMaster && masterNeedsUpdate) {
			return errors.New("Master nodes must be upgraded before worker nodes")
		}
		isController, err := a.isControllerNode(node)
		if err != nil {
			return err
		}
		contextLog.Infof("Is controller: %t", isController)
		if isMaster {
			if isController {
				// If there is no error, this will end the run of this reconciliation since the controller will be migrated
				if err := drain.Drain(node, a.clientSet, drain.Params{
					Force:               true,
					DeleteLocalData:     true,
					IgnoreAllDaemonSets: true,
				}); err != nil {
					return err
				}
			} else if isOriginal {
				return a.kubeadmUpOrDowngrade(machine, node, installer, version, planKey, planJSON, originalMaster)
			} else {
				return a.kubeadmUpOrDowngrade(machine, node, installer, version, planKey, planJSON, secondaryMaster)
			}
		}
		return a.kubeadmUpOrDowngrade(machine, node, installer, version, planKey, planJSON, worker)
	}

	if err = a.performActualUpdate(installer, machine, node, nodePlan, c); err != nil {
		return err
	}

	if err = a.setNodeAnnotation(node, planKey, planJSON); err != nil {
		return err
	}
	a.recordEvent(machine, corev1.EventTypeNormal, "Update", "updated machine %s", machine.Name)
	return nil
}

// kubeadmUpOrDowngrade does upgrade or downgrade a machine.
// Parameter k8sversion specified here represents the version of both Kubernetes and Kubeadm.
func (a *MachineController) kubeadmUpOrDowngrade(machine *clusterv1.Machine, node *corev1.Node, installer *os.OS,
	k8sVersion, planKey, planJSON string, ntype nodeType) error {
	b := plan.NewBuilder()
	b.AddResource(
		"upgrade:node-unlock-kubernetes",
		&resource.Run{Script: object.String("yum versionlock delete 'kube*' || true")})
	b.AddResource(
		"upgrade:node-install-kubeadm",
		&resource.RPM{Name: "kubeadm", Version: k8sVersion, DisableExcludes: "kubernetes"},
		plan.DependOn("upgrade:node-unlock-kubernetes"))

	//
	// For secondary masters
	// version >= 1.16.0 uses: kubeadm upgrade node
	// version >= 1.14.0 && < 1.16.0 uses: kubeadm upgrade node experimental-control-plane
	//
	secondaryMasterUpgradeControlPlaneFlag := ""
	if lt, err := version.LessThan(k8sVersion, "v1.16.0"); err == nil && lt {
		secondaryMasterUpgradeControlPlaneFlag = "experimental-control-plane"
	}

	switch ntype {
	case originalMaster:
		b.AddResource(
			"upgrade:node-kubeadm-upgrade",
			&resource.Run{Script: object.String(fmt.Sprintf("kubeadm upgrade plan && kubeadm upgrade apply -y %s", k8sVersion))},
			plan.DependOn("upgrade:node-install-kubeadm"))
	case secondaryMaster:
		b.AddResource(
			"upgrade:node-kubeadm-upgrade",
			&resource.Run{Script: object.String(fmt.Sprintf("kubeadm upgrade node %s", secondaryMasterUpgradeControlPlaneFlag))},
			plan.DependOn("upgrade:node-install-kubeadm"))
	case worker:
		b.AddResource(
			"upgrade:node-kubeadm-upgrade",
			&resource.Run{Script: object.String(fmt.Sprintf("kubeadm upgrade node config --kubelet-version %s", k8sVersion))},
			plan.DependOn("upgrade:node-install-kubeadm"))
	}
	b.AddResource(
		"upgrade:node-kubelet",
		&resource.RPM{Name: "kubelet", Version: k8sVersion, DisableExcludes: "kubernetes"},
		plan.DependOn("upgrade:node-kubeadm-upgrade"))
	b.AddResource(
		"upgrade:node-restart-kubelet",
		&resource.Run{Script: object.String("systemctl restart kubelet")},
		plan.DependOn("upgrade:node-kubelet"))
	b.AddResource(
		"upgrade:node-kubectl",
		&resource.RPM{Name: "kubectl", Version: k8sVersion, DisableExcludes: "kubernetes"},
		plan.DependOn("upgrade:node-restart-kubelet"))
	b.AddResource(
		"upgrade:node-lock-kubernetes",
		&resource.Run{Script: object.String("yum versionlock add 'kube*' || true")},
		plan.DependOn("upgrade:node-kubectl"))

	p, err := b.Plan()
	if err != nil {
		return err
	}
	if err := installer.SetupNode(&p); err != nil {
		log.Infof("Failed to upgrade node %s: %v", node.Name, err)
		return err
	}
	log.Infof("About to uncordon node %s...", node.Name)
	if err := a.uncordon(node); err != nil {
		log.Info("Failed to uncordon...")
		return err
	}
	log.Info("Finished with uncordon...")
	if err = a.setNodeAnnotation(node, planKey, planJSON); err != nil {
		return err
	}
	a.recordEvent(machine, corev1.EventTypeNormal, "Update", "updated machine %s", machine.Name)
	return nil
}

func (a *MachineController) prepareForMasterUpdate() error {
	// Check if it's safe to update a master
	if err := a.checkMasterHAConstraint(); err != nil {
		return gerrors.Wrap(err, "Not enough available master nodes to allow master update")
	}
	return nil
}

func (a *MachineController) performActualUpdate(
	installer *os.OS,
	machine *clusterv1.Machine,
	node *corev1.Node,
	nodePlan *plan.Plan,
	cluster *baremetalspecv1.BareMetalCluster) error {
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

func (a *MachineController) getNodePlan(providerSpec *baremetalspecv1.BareMetalCluster, machine *clusterv1.Machine, machineAddress string, installer *os.OS) (*plan.Plan, error) {
	namespace := a.controllerNamespace
	secrets, err := a.kubeadmJoinSecrets()
	if err != nil {
		return nil, err
	}
	token, err := a.token(secrets.BootstrapTokenID)
	if err != nil {
		return nil, err
	}
	master, err := a.getControllerNode()
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
			NodeIP:         machineAddress,
			CloudProvider:  providerSpec.CloudProvider,
			ExtraArguments: specs.TranslateServerArgumentsToStringMap(providerSpec.KubeletArguments),
		},
		KubernetesVersion:    machineutil.GetKubernetesVersion(machine),
		CRI:                  providerSpec.CRI,
		ConfigFileSpecs:      providerSpec.OS.Files,
		ProviderConfigMaps:   configMaps,
		AuthConfigMap:        authConfigMap,
		Namespace:            namespace,
		ExternalLoadBalancer: providerSpec.APIServer.ExternalLoadBalancer,
	})
	if err != nil {
		return nil, gerrors.Wrapf(err, "failed to create machine plan for %s", machine.Name)
	}
	return plan, nil
}

func (a *MachineController) getAuthConfigMap() (*v1.ConfigMap, error) {
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

func (a *MachineController) getProviderConfigMaps(providerSpec *baremetalspecv1.BareMetalCluster) (map[string]*v1.ConfigMap, error) {
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

func checkForVersionJump(machine *clusterv1.Machine, node *corev1.Node) error {
	mVersion := machineVersion(machine)
	nVersion := nodeVersion(node)
	lt, err := version.LessThan(mVersion, nVersion)
	if err != nil {
		return err
	}
	if lt {
		return fmt.Errorf("Downgrade not supported. Machine version: %s is less than node version: %s", mVersion, nVersion)
	}
	isVersionJump, err := version.Jump(nVersion, mVersion)
	if err != nil {
		return err
	}
	if isVersionJump {
		return fmt.Errorf("Upgrades can only be performed between patch versions of a single minor version or between "+
			"minor versions differing by no more than 1 - machine version: %s, node version: %s", mVersion, nVersion)
	}
	return nil
}

func (a *MachineController) checkIfMasterNotAtVersion(kubernetesVersion string) (bool, error) {
	nodes, err := a.getMasterNodes()
	if err != nil {
		// If we can't read the nodes, return the error so we don't
		// accidentally flush the sole master
		return false, err
	}
	for _, master := range nodes {
		if nodeVersion(master) != kubernetesVersion {
			return true, nil
		}
	}
	return false, nil
}

func (a *MachineController) checkIfOriginalMasterNotAtVersion(kubernetesVersion string) (bool, error) {
	node, err := a.getOriginalMasterNode()
	if err != nil {
		// If we can't read the nodes, return the error so we don't
		// accidentally flush the sole master
		return false, err
	}
	return nodeVersion(node) != kubernetesVersion, nil
}

func (a *MachineController) getOriginalMasterNode() (*corev1.Node, error) {
	nodes, err := a.getMasterNodes()
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		_, isOriginalMaster := node.Labels[originalMasterLabel]
		if isOriginalMaster {
			return node, nil
		}
	}

	if len(nodes) == 0 {
		return nil, errors.New("No master found")
	}

	// There is no master node which is labeled with originalMasterLabel
	// So we just pick nodes[0] of the list, then label it.
	originalMasterNode := nodes[0]
	if _, exist := originalMasterNode.Labels[originalMasterLabel]; !exist {
		if err := a.setNodeLabel(originalMasterNode, originalMasterLabel, ""); err != nil {
			return nil, err
		}
	}

	return originalMasterNode, nil
}

func (a *MachineController) isOriginalMaster(node *corev1.Node) (bool, error) {
	masterNode, err := a.getOriginalMasterNode()
	if err != nil {
		return false, err
	}
	return masterNode.Name == node.Name, nil
}

func extractEndpointAddress(urlstr string) (string, error) {
	u, err := url.Parse(urlstr)
	if err != nil {
		return "", err
	}
	return u.Hostname(), nil
}

func machineVersion(machine *clusterv1.Machine) string {
	return "v" + machineutil.GetKubernetesVersion(machine)
}

func nodeVersion(node *corev1.Node) string {
	return node.Status.NodeInfo.KubeletVersion
}

func (a *MachineController) uncordon(node *corev1.Node) error {
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

func (a *MachineController) setNodeAnnotation(node *corev1.Node, key, value string) error {
	err := a.modifyNode(node, func(node *corev1.Node) {
		node.Annotations[key] = value
	})
	if err != nil {
		return gerrors.Wrapf(err, "Failed to set node annotation: %s for node: %s", key, node.Name)
	}
	return nil
}

func (a *MachineController) setNodeLabel(node *corev1.Node, label, value string) error {
	err := a.modifyNode(node, func(node *corev1.Node) {
		node.Labels[label] = value
	})
	if err != nil {
		return gerrors.Wrapf(err, "Failed to set node label: %s for node: %s", label, node.Name)
	}
	return nil
}

func (a *MachineController) removeNodeLabel(node *corev1.Node, label string) error {
	err := a.modifyNode(node, func(node *corev1.Node) {
		delete(node.Labels, label)
	})
	if err != nil {
		return gerrors.Wrapf(err, "Failed to remove node label: %s for node: %s", label, node.Name)
	}
	return nil
}

func (a *MachineController) modifyNode(node *corev1.Node, updater func(node *corev1.Node)) error {
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

func (a *MachineController) checkMasterHAConstraint() error {
	nodes, err := a.getMasterNodes()
	if err != nil {
		// If we can't read the nodes, return the error so we don't
		// accidentally flush the sole master
		return err
	}
	avail := 0
	for _, node := range nodes {
		if hasConditionTrue(node, corev1.NodeReady) && !hasTaint(node, "NoSchedule") {
			avail++
			if avail >= 2 {
				return nil
			}
		}
	}
	return errors.New("Fewer than two master nodes available")
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

func (a *MachineController) findNodeByID(machineID, systemUUID string) (*corev1.Node, error) {
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

var staticRand = rand.New(rand.NewSource(time.Now().Unix()))

func (a *MachineController) getMasterNode() (*corev1.Node, error) {
	masters, err := a.getMasterNodes()
	if err != nil {
		return nil, err
	}
	if len(masters) == 0 {
		return nil, errors.New("no master node found")
	}
	// Randomise to limit chances of always hitting the same master node:
	index := staticRand.Intn(len(masters))
	return masters[index], nil
}

func (a *MachineController) getMasterNodes() ([]*corev1.Node, error) {
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

func (a *MachineController) getControllerNode() (*corev1.Node, error) {
	name, err := a.getControllerNodeName()
	if err != nil {
		return nil, err
	}
	nodes, err := a.getMasterNodes()
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if node.Name == name {
			return node, nil
		}
	}
	return nil, errors.New("Could not find controller node")
}

func (a *MachineController) isControllerNode(node *corev1.Node) (bool, error) {
	name, err := a.getControllerNodeName()
	if err != nil {
		return false, err
	}
	return node.Name == name, nil
}

func (a *MachineController) getControllerNodeName() (string, error) {
	pods, err := a.clientSet.CoreV1().Pods(a.controllerNamespace).List(metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	for _, pod := range pods.Items {
		if pod.Labels["name"] == controllerName {
			return pod.Spec.NodeName, nil
		}
	}
	return "", err
}

func (a *MachineController) updateMachine(machine *baremetalspecv1.BareMetalMachine, ip string) {
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
	_, isMaster := node.Labels[masterLabel]
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

func (a *MachineController) recordEvent(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
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

func getMachineID(machine *baremetalspecv1.BareMetalMachine) string {
	return machine.Namespace + ":" + machine.Name
}

func (a *MachineController) getMachineAddress(m *baremetalspecv1.BareMetalMachine) string {
	if m.Private.Address != "" {
		return m.Private.Address
	}
	return machineIPs[getMachineID(m)]
}

func (a *MachineController) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	controller, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&baremetalspecv1.BareMetalMachine{}).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: util.MachineToInfrastructureMapFunc(baremetalspecv1.SchemeGroupVersion.WithKind("BareMetalMachine")),
			},
		).
		// TODO: add watch to reconcile all machines that need it
		WithEventFilter(pausedPredicates()).
		Build(a)

	if err != nil {
		return err
	}
	_ = controller // not currently using it here, but it will run in the background
	return nil
}

// MachineControllerParams groups required inputs to create a machine actuator.
type MachineControllerParams struct {
	Client              client.Client
	ClientSet           *kubernetes.Clientset
	ControllerNamespace string
	EventRecorder       record.EventRecorder
	Verbose             bool
}

// NewMachineController creates a new baremetal machine reconciler.
func NewMachineController(params MachineControllerParams) (*MachineController, error) {
	return &MachineController{
		client:              params.Client,
		clientSet:           params.ClientSet,
		controllerNamespace: params.ControllerNamespace,
		eventRecorder:       params.EventRecorder,
		verbose:             params.Verbose,
	}, nil
}
