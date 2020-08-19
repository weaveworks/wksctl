package ssh

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/existinginfra/v1alpha3"
	"github.com/weaveworks/wksctl/pkg/plan"
	sshutil "github.com/weaveworks/wksctl/pkg/utilities/ssh"
	"golang.org/x/crypto/ssh"
)

// ClientParams groups inputs to build a client object.
type ClientParams struct {
	User           string
	Host           string
	Port           uint16
	PrivateKeyPath string
	PrivateKey     []byte
	PrintOutputs   bool
}

// Client is a higher-level abstraction around the standard API's SSH
// configuration, client and connection to the remote machine.
type Client struct {
	client       *ssh.Client
	printOutputs bool
}

var _ plan.Runner = &Client{}

const tcp = "tcp"

func NewClientForMachine(m *v1alpha3.ExistingInfraMachineSpec, user, keyPath string, printOutputs bool) (*Client, error) {
	ip := m.Public.Address
	port := m.Public.Port
	return NewClient(ClientParams{
		User:           user,
		Host:           ip,
		Port:           port,
		PrivateKeyPath: keyPath,
		PrintOutputs:   printOutputs,
	})
}
