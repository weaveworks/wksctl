package kubeadm

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	kubeadmutil "k8s.io/cluster-bootstrap/token/util"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta1"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
)

// GenerateBootstrapToken generates a new kubeadm bootstrap token, used by
// kubeadm init and kubeadm join to safely form clusters.
func GenerateBootstrapToken() (*kubeadmapi.BootstrapTokenString, error) {
	token, err := kubeadmutil.GenerateBootstrapToken()
	if err != nil {
		return nil, err
	}
	idAndSecret := strings.Split(token, ".")
	if len(idAndSecret) != 2 {
		// Never supposed to happen with the current bootstrap token format,
		// but we nevertheless defend ourselves against it.
		return nil, fmt.Errorf("invalid kubeadm bootstrap token: %s", token)
	}
	return &kubeadmapi.BootstrapTokenString{
		ID:     idAndSecret[0],
		Secret: idAndSecret[1],
	}, nil
}

// GenerateBootstrapSecret creates a new bootstrap secret to be used with kubeadm join.  This secret must be
// applied to the kubernetes cluster prior to trying to join a node to the cluster.
func GenerateBootstrapSecret(namespace string) (*corev1.Secret, error) {
	t, err := GenerateBootstrapToken()
	if err != nil {
		return nil, err
	}
	d := map[string]string{
		bootstrapapi.BootstrapTokenExtraGroupsKey:      kubeadmconstants.NodeBootstrapTokenAuthGroup,
		bootstrapapi.BootstrapTokenIDKey:               t.ID,
		bootstrapapi.BootstrapTokenSecretKey:           t.Secret,
		bootstrapapi.BootstrapTokenUsageAuthentication: "true",
		bootstrapapi.BootstrapTokenUsageSigningKey:     "true",
		bootstrapapi.BootstrapTokenExpirationKey:       time.Now().Add(time.Hour * 24).Format(time.RFC3339),
	}
	n := fmt.Sprintf("%s%s", bootstrapapi.BootstrapTokenSecretPrefix, t.ID)
	return &corev1.Secret{
		Type:       corev1.SecretTypeBootstrapToken,
		StringData: d,
		ObjectMeta: metav1.ObjectMeta{
			Name:      n,
			Namespace: namespace,
		},
	}, nil
}
