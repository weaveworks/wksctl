package ssh

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/weaveworks/wksctl/pkg/baremetalproviderspec/v1alpha1"
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

func NewClientForMachine(m *v1alpha1.BareMetalMachineProviderSpec, user, keyPath string, printOutputs bool) (*Client, error) {
	var ip string
	var port uint16
	if m.Public.Address != "" {
		ip = m.Public.Address
		port = m.Public.Port
	} else {
		// Fall back to the address at the root
		ip = m.Address
		port = m.Port
	}
	return NewClient(ClientParams{
		User:           user,
		Host:           ip,
		Port:           port,
		PrivateKeyPath: keyPath,
		PrintOutputs:   printOutputs,
	})
}

// NewClient instantiates a new SSH Client object.
// N.B.: provide either the key (privateKey) or its path (privateKeyPath).
func NewClient(params ClientParams) (*Client, error) {
	log.WithFields(log.Fields{"user": params.User, "host": params.Host, "port": params.Port, "privateKeyPath": params.PrivateKeyPath, "printOutputs": params.PrintOutputs}).Debugf("creating SSH client")
	signer, err := sshutil.SignerFromPrivateKey(params.PrivateKeyPath, params.PrivateKey)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read private key from \"%s\"", params.PrivateKeyPath)
	}
	hostPublicKey, err := sshutil.HostPublicKey(params.Host)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read host %s's public key", params.Host)
	}
	config := &ssh.ClientConfig{
		User: params.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: sshutil.HostKeyCallback(hostPublicKey),
	}
	hostPort := fmt.Sprintf("%s:%d", params.Host, params.Port)
	client, err := ssh.Dial(tcp, hostPort, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to %s", hostPort)
	}
	return &Client{
		client:       client,
		printOutputs: params.PrintOutputs,
	}, nil
}

// RunCommand executes the provided command on the remote machine configured in
// this Client object. A new Session is created for each call to RunCommand.
// A Client supports multiple interactive sessions.
func (c *Client) RunCommand(command string, stdin io.Reader) (string, error) {
	log.Debugf("running command: %s", command)
	return c.handleSessionIO(func(session *ssh.Session) error {
		session.Stdin = stdin
		return session.Start(command)
	})
}

// Handle output and command completion for a remote shell
func (c *Client) handleSessionIO(action func(*ssh.Session) error) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", errors.Wrap(err, "failed to create new SSH session")
	}
	defer session.Close()
	// Write stdout and stderr to both this process' stdout and stderr, and
	// buffers, for later re-use.
	stdOutPipe, err := session.StdoutPipe()
	if err != nil {
		return "", errors.Wrap(err, "failed to get pipe to standard output")
	}
	stdErrPipe, err := session.StderrPipe()
	if err != nil {
		return "", errors.Wrap(err, "failed to get pipe to standard error")
	}
	var stdOutErr bytes.Buffer
	outWriters := []io.Writer{&stdOutErr}
	errWriters := []io.Writer{&stdOutErr}
	if c.printOutputs {
		outWriters = append(outWriters, os.Stdout)
		errWriters = append(errWriters, os.Stderr)
	}
	stdOutWriter := io.MultiWriter(outWriters...)
	stdErrWriter := io.MultiWriter(errWriters...)

	err = action(session)

	// Don't respond to err until output complete
	var errStdOut, errStdErr error
	syncChan := make(chan bool)
	go func() {
		_, errStdOut = io.Copy(stdOutWriter, stdOutPipe)
		syncChan <- true
	}()
	go func() {
		_, errStdErr = io.Copy(stdErrWriter, stdErrPipe)
		syncChan <- true
	}()

	// Make sure copying is finished
	<-syncChan
	<-syncChan

	// Now we can return the error
	if err != nil {
		return stdOutErr.String(), errors.Wrap(err, "failed while remote executing")
	}

	if err := session.Wait(); err != nil {
		if err, ok := err.(*ssh.ExitError); ok {
			return stdOutErr.String(), &plan.RunError{ExitCode: err.ExitStatus()}
		}
		return stdOutErr.String(), errors.Wrap(err, "failed while waiting for end of remote execution")
	}

	if errStdOut != nil {
		return stdOutErr.String(), errors.Wrap(errStdOut, "failed while capturing stdout")
	}
	if errStdErr != nil {
		return stdOutErr.String(), errors.Wrap(errStdErr, "failed while capturing stderr")
	}
	return stdOutErr.String(), nil
}

// RemoteAddr returns the remote address of this SSH client.
func (c *Client) RemoteAddr() net.Addr {
	return c.client.RemoteAddr()
}

// Close closes this high-level Client's underlying SSH connection.
func (c *Client) Close() error {
	return c.client.Close()
}
