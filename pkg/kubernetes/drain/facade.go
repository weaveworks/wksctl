package drain

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DefaultIgnoredDaemonSets represents the default daemonsets ignored by WKS,
// when draining a node.
var DefaultIgnoredDaemonSets = []metav1.ObjectMeta{
	{
		Namespace: "kube-system",
		Name:      "aws-node",
	},
	{
		Namespace: "kube-system",
		Name:      "kube-proxy",
	},
	{
		Name: "node-exporter",
	},
	{
		Name: "prom-node-exporter",
	},
	{
		Name: "weave-scope",
	},
	{
		Name: "weave-scope-agent",
	},
	{
		Name: "weave-net",
	},
}

// DefaultTimeOut is the default duration Drain will wait for pods to be
// evicted before erroring. This value will be used if Params.TimeOut is not
// provided.
var DefaultTimeOut = 1 * time.Minute

// Params groups required inputs to drain a Kubernetes node.
type Params struct {
	Force               bool
	DeleteLocalData     bool
	IgnoreAllDaemonSets bool
	IgnoreDaemonSets    []metav1.ObjectMeta
	TimeOut             time.Duration
}

// Drain drains the provided node.
func Drain(node *corev1.Node, clientSet kubernetes.Interface, params Params) error {
	drainer := &Helper{
		Client:              clientSet,
		Force:               params.Force,
		DeleteLocalData:     params.DeleteLocalData,
		IgnoreAllDaemonSets: params.IgnoreAllDaemonSets,
		IgnoreDaemonSets:    params.IgnoreDaemonSets,
	}
	if err := drainer.CanUseEvictions(); err != nil {
		// TODO: this wrapping should really be done within CanUseEvictions:
		return errors.Wrapf(err, "checking if cluster implements policy API")
	}
	if err := cordon(node, clientSet); err != nil {
		return err
	}
	return evictPods(node, getOrDefault(params.TimeOut), drainer)
}

func cordon(node *corev1.Node, clientSet kubernetes.Interface) error {
	cordonHelper := NewCordonHelper(node, CordonNode)
	if cordonHelper.IsUpdateRequired() {
		err, patchErr := cordonHelper.PatchOrReplace(clientSet)
		if patchErr != nil {
			log.Warn(patchErr.Error())
		}
		if err != nil {
			log.Error(err.Error())
		}
		log.Infof("%s node %q", CordonNode, node.Name)
	} else {
		log.Debugf("no need to %s node %q", CordonNode, node.Name)
	}
	return nil
}

func getOrDefault(timeOut time.Duration) time.Duration {
	if timeOut == 0 {
		return DefaultTimeOut
	}
	return timeOut
}

func evictPods(node *corev1.Node, timeOut time.Duration, drainer *Helper) error {
	timer := time.After(timeOut)
	start := time.Now()
	for {
		select {
		case <-timer:
			return fmt.Errorf("timed out (after %s) waiting for node %q to be drain", timeOut, node.Name)
		default:
			numPendingPods, err := evictPodsOn(node, drainer)
			if err != nil {
				return err
			}
			log.Debugf("%d pod(s) to be evicted from %s", numPendingPods, node.Name)
			if numPendingPods == 0 {
				return nil
			}
			// Wait a bit, to avoid hitting the API server in a tight loop:
			wait(timeOut, start)
		}
	}
}

func evictPodsOn(node *corev1.Node, drainer *Helper) (int, error) {
	podsForDeletion, errs := drainer.GetPodsForDeletion(node.Name)
	if len(errs) > 0 {
		return -1, fmt.Errorf("errors: %v", errs) // TODO: improve formatting
	}
	if w := podsForDeletion.Warnings(); w != "" {
		log.Warn(w)
	}
	pods := podsForDeletion.Pods()
	numPendingPods := len(pods)
	for _, pod := range pods {
		// TODO: handle API rate limitter error
		if err := drainer.EvictOrDeletePod(pod); err != nil {
			return numPendingPods, err
		}
	}
	return numPendingPods, nil
}

func wait(timeOut time.Duration, start time.Time) {
	elapsed := time.Now().Sub(start)
	remainingTime := timeOut - elapsed
	time.Sleep(min(remainingTime, max(5*time.Second, timeOut/10)))
}

func max(x time.Duration, y time.Duration) time.Duration {
	if x < y {
		return y
	}
	return x
}

func min(x time.Duration, y time.Duration) time.Duration {
	if x < y {
		return x
	}
	return y
}
