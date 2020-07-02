package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const GroupName = "cluster.weave.works"

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha3"}

var (
	SchemeBuilder      runtime.SchemeBuilder
	localSchemeBuilder = &SchemeBuilder
	AddToScheme        = localSchemeBuilder.AddToScheme
)

func init() {
	localSchemeBuilder.Register(addKnownTypes)
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&ExistingInfraMachine{},
		&ExistingInfraMachineList{},
		&ExistingInfraCluster{},
		&ExistingInfraClusterList{},
	)
	// TODO: Do we really need this?
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
