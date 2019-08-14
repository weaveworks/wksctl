package v1alpha1

import (
	"bytes"
	"fmt"

	"github.com/weaveworks/wksctl/pkg/baremetalproviderspec"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

// +k8s:deepcopy-gen=false
type BareMetalProviderSpecCodec struct {
	encoder runtime.Encoder
	decoder runtime.Decoder
}

const GroupName = "baremetalproviderspec"

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1alpha1"}

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
		&BareMetalMachineProviderSpec{},
	)
	scheme.AddKnownTypes(SchemeGroupVersion,
		&BareMetalClusterProviderSpec{},
	)
	return nil
}

func NewScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := baremetalproviderspec.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func NewCodec() (*BareMetalProviderSpecCodec, error) {
	scheme, err := NewScheme()
	if err != nil {
		return nil, err
	}
	codecFactory := serializer.NewCodecFactory(scheme)
	encoder, err := newEncoder(&codecFactory)
	if err != nil {
		return nil, err
	}
	codec := BareMetalProviderSpecCodec{
		encoder: encoder,
		decoder: codecFactory.UniversalDecoder(SchemeGroupVersion),
	}
	return &codec, nil
}

func (codec *BareMetalProviderSpecCodec) ClusterProviderFromProviderSpec(providerSpec clusterv1.ProviderSpec) (*BareMetalClusterProviderSpec, error) {
	var spec BareMetalClusterProviderSpec
	err := codec.DecodeFromProviderSpec(providerSpec, &spec)
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

func (codec *BareMetalProviderSpecCodec) MachineProviderFromProviderSpec(providerSpec clusterv1.ProviderSpec) (*BareMetalMachineProviderSpec, error) {
	var spec BareMetalMachineProviderSpec
	err := codec.DecodeFromProviderSpec(providerSpec, &spec)
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

func (codec *BareMetalProviderSpecCodec) DecodeFromProviderSpec(providerSpec clusterv1.ProviderSpec, out runtime.Object) error {
	_, _, err := codec.decoder.Decode(providerSpec.Value.Raw, nil, out)
	if err != nil {
		return fmt.Errorf("decoding failure: %v", err)
	}
	return nil
}

func (codec *BareMetalProviderSpecCodec) EncodeToProviderSpec(in runtime.Object) (*clusterv1.ProviderSpec, error) {
	var buf bytes.Buffer
	if err := codec.encoder.Encode(in, &buf); err != nil {
		return nil, fmt.Errorf("encoding failed: %v", err)
	}
	return &clusterv1.ProviderSpec{
		Value: &runtime.RawExtension{Raw: buf.Bytes()},
	}, nil
}

func newEncoder(codecFactory *serializer.CodecFactory) (runtime.Encoder, error) {
	serializerInfos := codecFactory.SupportedMediaTypes()
	if len(serializerInfos) == 0 {
		return nil, fmt.Errorf("unable to find any serlializers")
	}
	encoder := codecFactory.EncoderForVersion(serializerInfos[0].Serializer, SchemeGroupVersion)
	return encoder, nil
}
