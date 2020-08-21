package ssh

import (
	"github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeissh "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/runners/ssh"
)

func NewClientForMachine(m *v1alpha3.MachineSpec, user, keyPath string, printOutputs bool) (*capeissh.Client, error) {
	ip := m.Public.Address
	port := m.Public.Port
	return capeissh.NewClient(capeissh.ClientParams{
		User:           user,
		Host:           ip,
		Port:           port,
		PrivateKeyPath: keyPath,
		PrintOutputs:   printOutputs,
	})
}
