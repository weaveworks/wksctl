package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BareMetalClusterProviderSpec struct {
	metav1.TypeMeta `json:",inline"`

	User                 string `json:"user"`
	DeprecatedSSHKeyPath string `json:"sshKeyPath"`
	HTTPProxy            string `json:"httpProxy,omitempty"`

	Authentication *AuthenticationWebhook `json:"authenticationWebhook,omitempty"`
	Authorization  *AuthorizationWebhook  `json:"authorizationWebhook,omitempty"`

	OS              OSConfig         `json:"os,omitempty"`
	CRI             ContainerRuntime `json:"cri"`
	ImageRepository string           `json:"imageRepository,omitempty"`

	APIServer APIServer `json:"apiServer,omitempty"`

	Addons []Addon `json:"addons,omitempty"`

	CloudProvider string `json:"cloudProvider,omitempty"`
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
	ExternalLoadBalancer string   `json:"externalLoadBalancer"`
	AdditionalSANs       []string `json:"additionalSANs,omitempty"`
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

type BareMetalMachineProviderSpec struct {
	metav1.TypeMeta `json:",inline"`

	Address          string   `json:"address"`
	Port             uint16   `json:"port,omitempty"`
	PrivateAddress   string   `json:"privateAddress,omitempty"`
	PrivateInterface string   `json:"privateInterface,omitempty"`
	Private          EndPoint `json:"private,omitempty"`
	Public           EndPoint `json:"public,omitempty"`
}

// EndPoint groups the details required to establish a connection.
type EndPoint struct {
	Address string `json:"address"`
	Port    uint16 `json:"port"`
}
