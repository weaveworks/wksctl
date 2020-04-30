package baremetalproviderspec

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

	ImageRepository string `json:"imageRepository,omitempty"`

	APIServer APIServer `json:"apiServer,omitempty"`

	KubeletArguments []ServerArgument `json:"kubeletArguments,omitempty"`

	Addons []Addon `json:"addons,omitempty"`
}

type AuthenticationWebhook struct {
	CacheTTL      string        `json:"cacheTTL,omitempty"`
	WebhookClient WebhookClient `json:"client"`
	WebhookServer WebhookServer `json:"server"`
}

type AuthorizationWebhook struct {
	CacheAuthorizedTTL   string        `json:"cacheAuthorizedTTL,omitempty"`
	CacheUnauthorizedTTL string        `json:"cacheUnauthorizedTTL,omitempty"`
	WebhookClient        WebhookClient `json:"client"`
	WebhookServer        WebhookServer `json:"server"`
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

type WebhookClient struct {
	KeyData         []byte `json:"keyData,omitempty"`
	CertificateData []byte `json:"certificateData,omitempty"`
}

type WebhookServer struct {
	URL                      string `json:"url"`
	CertificateAuthorityData []byte `json:"certificateAuthorityData,omitempty"`
}

// Addon describes an addon to install on the cluster.
type Addon struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params,omitempty"`
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
