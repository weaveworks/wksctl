package drain

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/drain"
)

// DefaultTimeOut is the default duration Drain will wait for pods to be
// evicted before erroring. This value will be used if Params.TimeOut is not
// provided.
var DefaultTimeOut = 1 * time.Minute

// Params groups required inputs to drain a Kubernetes node.
type Params struct {
	Force               bool
	DeleteLocalData     bool
	IgnoreAllDaemonSets bool
	TimeOut             time.Duration
}

// Drain drains the provided node.
func Drain(node *corev1.Node, clientSet kubernetes.Interface, params Params) error {
	drainer := &drain.Helper{
		Client:              clientSet,
		Force:               params.Force,
		DeleteLocalData:     params.DeleteLocalData,
		IgnoreAllDaemonSets: params.IgnoreAllDaemonSets,
	}
	policyGroupVersion, err := drain.CheckEvictionSupport(clientSet)
	if err != nil {
		return errors.Wrapf(err, "eviction not supported")
	} else if len(policyGroupVersion) == 0 {
		return fmt.Errorf("policy group version not found in the API server; eviction is not supported")
	}

	if err := cordon(node, clientSet); err != nil {
		return err
	}
	return evictPods(node, getOrDefault(params.TimeOut), drainer)
}

func cordon(node *corev1.Node, clientSet kubernetes.Interface) error {
	cordonHelper := drain.NewCordonHelper(node)
	// If the desired state (that the node should be unschedulable) doesn't match the actual state,
	// then do the patch and replace logic
	if cordonHelper.UpdateIfRequired(true) {
		// false means no server dry run logic shall take place
		err, patchErr := cordonHelper.PatchOrReplace(clientSet, false)
		if patchErr != nil {
			log.Warn(patchErr.Error())
		}
		if err != nil {
			log.Error(err.Error())
		}
		log.Infof("cordoning node %q", node.Name)
	} else {
		log.Debugf("no need to cordon node %q", node.Name)
	}
	return nil
}

func getOrDefault(timeOut time.Duration) time.Duration {
	if timeOut == 0 {
		return DefaultTimeOut
	}
	return timeOut
}

func evictPods(node *corev1.Node, timeOut time.Duration, drainer *drain.Helper) error {
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

func evictPodsOn(node *corev1.Node, drainer *drain.Helper) (int, error) {
	podsForDeletion, errs := drainer.GetPodsForDeletion(node.Name)
	if len(errs) > 0 {
		return -1, fmt.Errorf("errors: %v", errs) // TODO: improve formatting
	}
	if w := podsForDeletion.Warnings(); w != "" {
		log.Warn(w)
	}
	pods := podsForDeletion.Pods()
	numPendingPods := len(pods)

	err := drainer.DeleteOrEvictPods(pods)
	return numPendingPods, err
}

func wait(timeOut time.Duration, start time.Time) {
	elapsed := time.Since(start)
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
