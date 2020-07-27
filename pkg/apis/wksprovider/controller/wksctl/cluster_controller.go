package wks

import (
	"context"
	"errors"

	log "github.com/sirupsen/logrus"
	existinginfrav1 "github.com/weaveworks/wksctl/pkg/existinginfra/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// ClusterReconciler is responsible for managing this cluster, and ensuring its
// state converges towards its definition.
type ClusterReconciler struct {
	client        client.Client
	eventRecorder record.EventRecorder
}

// Reconcile creates or updates the cluster.
func (a *ClusterReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, reterr error) {
	ctx := context.TODO() // upstream will add this eventually
	contextLog := log.WithField("name", req.NamespacedName)

	// request only contains the name of the object, so fetch it from the api-server
	eic := &existinginfrav1.ExistingInfraCluster{}
	err := a.client.Get(ctx, req.NamespacedName, eic)
	if err != nil {
		if apierrs.IsNotFound(err) { // isn't there; give in
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Get Cluster via OwnerReferences
	cluster, err := util.GetOwnerCluster(ctx, a.client, eic.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		contextLog.Info("Cluster Controller has not yet set ownerReferences")
		return ctrl.Result{}, nil
	}
	contextLog = contextLog.WithField("cluster", cluster.Name)

	if util.IsPaused(cluster, eic) {
		contextLog.Info("ExistingInfraCluster or linked Cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(eic, a.client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Attempt to Patch the ExistingInfraMachine object and status after each reconciliation.
	defer func() {
		if err := patchHelper.Patch(ctx, eic); err != nil {
			contextLog.Errorf("failed to patch ExistingInfraCluster: %v", err)
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// Object still there but with deletion timestamp => run our finalizer
	if !eic.ObjectMeta.DeletionTimestamp.IsZero() {
		a.recordEvent(cluster, corev1.EventTypeNormal, "Delete", "Deleted cluster %v", cluster.Name)
		return ctrl.Result{}, errors.New("ClusterReconciler#Delete not implemented")
	}

	eic.Status.Ready = true // TODO: know whether it is really ready

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options) error {
	controller, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&existinginfrav1.ExistingInfraCluster{}).
		WithEventFilter(pausedPredicates()).
		Build(r)

	if err != nil {
		return err
	}
	_ = controller // not currently using it here, but it will run in the background
	return nil
}

func (a *ClusterReconciler) recordEvent(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
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

// NewClusterReconciler creates a new cluster reconciler.
func NewClusterReconciler(client client.Client, eventRecorder record.EventRecorder) (*ClusterReconciler, error) {
	return &ClusterReconciler{
		client:        client,
		eventRecorder: eventRecorder,
	}, nil
}
