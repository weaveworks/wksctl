package wks

import (
	"errors"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterActuator is responsible for managing this cluster, and ensuring its
// state converges towards its definition.
type ClusterActuator struct {
	eventRecorder record.EventRecorder
}

// Reconcile creates or updates the cluster.
func (a *ClusterActuator) Reconcile(cluster *clusterv1.Cluster) error {
	a.recordEvent(cluster, corev1.EventTypeNormal, "Reconcile", "Reconciled cluster %v", cluster.Name)
	return nil
}

// Delete the cluster.
func (a *ClusterActuator) Delete(cluster *clusterv1.Cluster) error {
	a.recordEvent(cluster, corev1.EventTypeNormal, "Delete", "Deleted cluster %v", cluster.Name)
	return errors.New("ClusterActuator#Delete not implemented")
}

func (a *ClusterActuator) recordEvent(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
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

// ClusterActuatorParams groups required inputs to create a cluster actuator.
type ClusterActuatorParams struct {
	Client        client.Client
	ClientSet     *kubernetes.Clientset
	EventRecorder record.EventRecorder
	Scheme        *runtime.Scheme
}

// NewClusterActuator creates a new cluster actuator.
func NewClusterActuator(params ClusterActuatorParams) (*ClusterActuator, error) {
	return &ClusterActuator{
		eventRecorder: params.EventRecorder,
	}, nil
}
