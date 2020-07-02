package scheme

import (
	ssv1alpha1 "github.com/bitnami-labs/sealed-secrets/pkg/apis/sealed-secrets/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1alpha3 "sigs.k8s.io/cluster-api/api/v1alpha3"

	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/wksctl/pkg/baremetal/v1alpha3"
	"github.com/weaveworks/wksctl/pkg/baremetalproviderspec/v1alpha1"
)

var (
	// Scheme contains information about all known types, API versions, and defaulting & conversion methods
	Scheme = runtime.NewScheme()

	// Codecs provides k8s API machinery low-level codec functionality
	Codecs = k8sserializer.NewCodecFactory(Scheme)

	// Serializer provides powerful high-level encoding/decoding functionality
	Serializer = serializer.NewSerializer(Scheme, &Codecs)
)

func init() {
	utilruntime.Must(AddToScheme(Scheme))
}

// AddToScheme builds the scheme using all known versions of the api.
func AddToScheme(scheme *runtime.Scheme) error {
	// This returns an error if and only if any of the following function calls return an error
	// If many errors are returned, they are all concatenated after each other
	return errors.NewAggregate([]error{
		clientgoscheme.AddToScheme(scheme),                     // Register all known Kubernetes types
		ssv1alpha1.AddToScheme(scheme),                         // Register Bitnami's Sealed Secrets types
		v1alpha1.AddToScheme(scheme),                           // Register our old v1alpha1 types
		v1alpha3.AddToScheme(scheme),                           // Register our new v1alpha3 types
		clusterv1alpha3.AddToScheme(scheme),                    // Register the upstream CAPI v1alpha3 types
		scheme.SetVersionPriority(v1alpha3.SchemeGroupVersion), // Always prefer v1alpha3 when encoding our types
	})
}
