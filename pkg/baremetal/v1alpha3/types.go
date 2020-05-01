package v1alpha3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:generate=true
// +groupName=cluster.weave.works

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BareMetalCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BareMetalClusterSpec `json:"spec,omitempty"`
}

type BareMetalClusterSpec struct {
	User                 string `json:"user"`
	DeprecatedSSHKeyPath string `json:"sshKeyPath"`
	HTTPProxy            string `json:"httpProxy,omitempty"`

	Authentication *AuthenticationWebhook `json:"authenticationWebhook,omitempty"`
	Authorization  *AuthorizationWebhook  `json:"authorizationWebhook,omitempty"`

	OS              OSConfig         `json:"os,omitempty"`
	CRI             ContainerRuntime `json:"cri"`
	ImageRepository string           `json:"imageRepository,omitempty"`

	APIServer APIServer `json:"apiServer,omitempty"`

	KubeletArguments []ServerArgument `json:"kubeletArguments,omitempty"`

	Addons []Addon `json:"addons,omitempty"`

	CloudProvider string `json:"cloudProvider,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BareMetalClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalCluster `json:"items"`
}

type OSConfig struct {
	Files []FileSpec `json:"files,omitempty"`
}

type FileSpec struct {
	Source      SourceSpec `json:"source"`
	Destination string     `json:"destination"`
	// XXX: maybe later --	Permissions string     `json:"permissions"`
}

type SourceSpec struct {
	ConfigMap string `json:"configmap"`
	Key       string `json:"key"`
}

type ContainerRuntime struct {
	Kind    string `json:"kind"`
	Package string `json:"package"`
	Version string `json:"version"`
}

type APIServer struct {
	ExternalLoadBalancer string           `json:"externalLoadBalancer"`
	AdditionalSANs       []string         `json:"additionalSANs,omitempty"`
	ExtraArguments       []ServerArgument `json:"extraArguments,omitempty"`
}

type ServerArgument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type AuthenticationWebhook struct {
	CacheTTL   string `json:"cacheTTL,omitempty"`
	URL        string `json:"url"`
	SecretFile string `json:"secretFile"`
}

type AuthorizationWebhook struct {
	CacheAuthorizedTTL   string `json:"cacheAuthorizedTTL,omitempty"`
	CacheUnauthorizedTTL string `json:"cacheUnauthorizedTTL,omitempty"`
	URL                  string `json:"url"`
	SecretFile           string `json:"secretFile"`
}

// Addon describes an addon to install on the cluster.
type Addon struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params,omitempty"`
	Deps   []string          `json:"deps,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BareMetalMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BareMetalMachineSpec `json:"spec,omitempty"`
}

type BareMetalMachineSpec struct {
	Address          string   `json:"address"`
	Port             uint16   `json:"port,omitempty"`
	PrivateAddress   string   `json:"privateAddress,omitempty"`
	PrivateInterface string   `json:"privateInterface,omitempty"`
	Private          EndPoint `json:"private,omitempty"`
	Public           EndPoint `json:"public,omitempty"`
}

// BareMetalMachineList contains a list of Machine
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BareMetalMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalMachine `json:"items"`
}

// EndPoint groups the details required to establish a connection.
type EndPoint struct {
	Address string `json:"address"`
	Port    uint16 `json:"port"`
}
