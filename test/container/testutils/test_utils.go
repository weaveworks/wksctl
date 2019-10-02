package testutils

import (
	"fmt"
	"io"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/footloose/pkg/cluster"
	"github.com/weaveworks/footloose/pkg/config"
	"github.com/weaveworks/wksctl/pkg/plan"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
)

type Operation struct {
	Kind   string
	Arg    string
	Output string // for operations that output something on stdouterr, we keep it there.
}

type TestRunner struct {
	T      *testing.T
	Runner plan.Runner

	ops []Operation
}

var _ plan.Runner = &TestRunner{}

// RunCommand implements plan.Runner.
func (r *TestRunner) RunCommand(cmd string, stdin io.Reader) (stdouterr string, err error) {
	r.T.Log("RunCommand:", cmd)
	stdouterr, err = r.Runner.RunCommand(cmd, stdin)
	r.T.Logf("Output:\n%s", stdouterr)

	r.pushRunCommand(cmd, stdouterr)
	return
}

// Give tests visibility on the operations done by a applying a resource.

func (r *TestRunner) Operations() []Operation {
	return r.ops
}

func (r *TestRunner) ResetOperations() {
	r.ops = nil
}

func (r *TestRunner) pushRunCommand(cmd string, output string) {
	r.ops = append(r.ops, Operation{
		Kind:   "RunCommand",
		Arg:    cmd,
		Output: output,
	})
}

func (r *TestRunner) Operation(i int) Operation {
	if i >= 0 {
		return r.ops[i]
	}
	return r.ops[len(r.ops)+i]
}

var _ plan.Runner = &FootlooseRunner{}

type FootlooseRunner struct {
	Name    string
	SSHPort uint16
	Image   string

	cluster *cluster.Cluster
	ssh     *ssh.Client
}

func (r *FootlooseRunner) clusterName() string {
	return "cluster-" + r.Name
}

func (r *FootlooseRunner) sshPrivateKeyPath() string {
	return ".tmp-key-" + r.Name
}

func (r *FootlooseRunner) makeCluster() (*cluster.Cluster, error) {
	footlooseCfg := config.Config{
		Cluster: config.Cluster{
			Name:       r.clusterName(),
			PrivateKey: r.sshPrivateKeyPath(),
		},
		Machines: []config.MachineReplicas{
			{
				Count: 1,
				Spec: config.Machine{
					Name: r.Name + "-%d", Image: r.Image,
					PortMappings: []config.PortMapping{
						{ContainerPort: 22, HostPort: r.SSHPort},
					},
				},
			},
		},
	}

	c, err := cluster.New(footlooseCfg)
	if err != nil {
		return nil, fmt.Errorf("footloose cluster New(): %v", err)
	}
	if err := c.Create(); err != nil {
		return nil, fmt.Errorf("footloose cluster Create(): %v", err)
	}
	return c, nil
}

func (r *FootlooseRunner) makeSSHClientWithRetries(numRetries int) (*ssh.Client, error) {
	host := "localhost"
	for i := 1; i <= numRetries; i++ {
		client, err := ssh.NewClient(ssh.ClientParams{
			User:           "root",
			Host:           host,
			Port:           r.SSHPort,
			PrivateKeyPath: r.sshPrivateKeyPath(),
			PrintOutputs:   true,
		})

		if err == nil {
			log.Printf("ssh.NewClient: successfully connected to %s:%d", host, r.SSHPort)
			return client, nil
		}

		log.Printf("ssh.NewClient (try %d of %d): %v", i, numRetries, err)

		if i < numRetries {
			time.Sleep(time.Second)
		}
	}
	return nil, fmt.Errorf("failed %d times to connect over ssh to %s:%d", numRetries, host, r.SSHPort)
}

func (r *FootlooseRunner) Close() {
	_ = r.ssh.Close()
	_ = r.cluster.Delete()
}

func (r *FootlooseRunner) Start() error {
	var err error
	r.cluster, err = r.makeCluster()
	if err != nil {
		return err
	}

	r.ssh, err = r.makeSSHClientWithRetries(30)
	if err != nil {
		r.cluster.Delete()
		return err
	}

	return nil
}

func (r *FootlooseRunner) RunCommand(command string, stdin io.Reader) (string, error) {
	return r.ssh.RunCommand(command, stdin)
}

func MakeFootlooseTestRunner(t *testing.T, image string, sshPort uint16) (*TestRunner, func()) {
	f := FootlooseRunner{
		Name:    t.Name(),
		Image:   image,
		SSHPort: sshPort,
	}
	if err := f.Start(); err != nil {
		t.Fatalf("MakeFootlooseTestRunner: %v", err)
	}
	return f.WrapInTestRunner(t), f.Close
}

func ConnectSSH(t *testing.T, r *FootlooseRunner) *ssh.Client {
	c, err := r.makeSSHClientWithRetries(1)
	assert.NoError(t, err)
	return c
}

func (r *FootlooseRunner) WrapInTestRunner(t *testing.T) *TestRunner {
	return &TestRunner{
		T:      t,
		Runner: r,
	}
}

type PortAllocator struct {
	mtx  sync.Mutex
	Next uint16
}

func (pa *PortAllocator) Allocate() uint16 {
	pa.mtx.Lock()
	defer pa.mtx.Unlock()

	result := pa.Next
	pa.Next++
	return result
}

// Other utilities
func AssertEmptyState(t *testing.T, s plan.Resource, r plan.Runner) {
	state, err := s.QueryState(r)
	assert.NoError(t, err)
	assert.Equal(t, plan.EmptyState, state)
}
