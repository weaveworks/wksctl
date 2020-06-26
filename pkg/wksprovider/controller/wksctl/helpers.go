package wks

import (
	"strings"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterutil "sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func pausedPredicates() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return processIfUnpaused(log.WithField("predicate", "updateEvent"), e.ObjectNew, e.MetaNew)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return processIfUnpaused(log.WithField("predicate", "createEvent"), e.Object, e.Meta)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return processIfUnpaused(log.WithField("predicate", "deleteEvent"), e.Object, e.Meta)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return processIfUnpaused(log.WithField("predicate", "genericEvent"), e.Object, e.Meta)
		},
	}
}

func processIfUnpaused(logger *log.Entry, obj runtime.Object, meta metav1.Object) bool {
	kind := strings.ToLower(obj.GetObjectKind().GroupVersionKind().Kind)
	log := logger.WithFields(log.Fields{"namespace": meta.GetNamespace(), kind: meta.GetName()})
	if clusterutil.HasPausedAnnotation(meta) {
		log.Info("Resource is paused, will not attempt to map resource")
		return false
	}
	log.Info("Resource is not paused, will attempt to map resource")
	return true
}
