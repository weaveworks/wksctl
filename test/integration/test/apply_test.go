package test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"
	"github.com/weaveworks/wksctl/pkg/specs"

	baremetalspecv1 "github.com/weaveworks/wksctl/pkg/baremetal/v1alpha3"
	spawn "github.com/weaveworks/wksctl/test/integration/spawn"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// Runs a basic set of tests for apply.

type role = string

const (
	master     = "master"
	node       = "node"
	sshKeyPath = "/root/.ssh/wksctl_cit_id_rsa"
)

var (
	srcDir    = os.Getenv("SRCDIR")
	configDir = filepath.Join(srcDir, "test", "integration", "test", "assets")
)

func generateName(role role, i int) string {
	switch role {
	case master:
		return fmt.Sprintf("master-%d", i)
	case node:
		return fmt.Sprintf("node-%d", i)
	default:
		panic(fmt.Errorf("unknown role: %s", role))
	}
}

func setLabel(role role) string {
	switch role {
	case master:
		return "master"
	case node:
		return "node"
	default:
		panic(fmt.Errorf("unknown role: %s", role))
	}
}

func appendMachine(t *testing.T, ordinal int, ml *[]*clusterv1.Machine, bl *[]*baremetalspecv1.BareMetalMachine, clusterName, role role, publicIP, privateIP string) {
	name := generateName(role, ordinal)
	spec := baremetalspecv1.BareMetalMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.weave.works/v1alpha3",
			Kind:       "BareMetalMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: baremetalspecv1.BareMetalMachineSpec{
			Public: baremetalspecv1.EndPoint{
				Address: publicIP,
				Port:    22,
			},
			Private: baremetalspecv1.EndPoint{
				Address: privateIP,
				Port:    22,
			}},
	}
	*bl = append(*bl, &spec)

	// Create a machine.
	machine := clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.x-k8s.io/v1alpha3",
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"set": setLabel(role),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: clusterName,
			InfrastructureRef: v1.ObjectReference{
				APIVersion: "cluster.weave.works/v1alpha3",
				Kind:       spec.TypeMeta.Kind,
				Name:       spec.ObjectMeta.Name,
			},
		},
	}

	*ml = append(*ml, &machine)
}

// makeMachinesFromTerraform creates cluster-api Machine objects from a
// terraform output. The terraform output must have two variables:
//  - "public_ips": list of public IPs
//  - "private_ips": list of private IPs (duh!)
//
// numMachines is the number of machines to use. It can be less than the number
// of provisionned terraform machines. -1 means use all machines setup by
// terraform. The minimum number of machines to use is 2.
func makeMachinesFromTerraform(t *testing.T, clusterName string, terraform *terraformOutput, numMachines int) (ml []*clusterv1.Machine, bl []*baremetalspecv1.BareMetalMachine) {
	publicIPs := terraform.stringArrayVar(keyPublicIPs)
	privateIPs := terraform.stringArrayVar(keyPrivateIPs)
	assert.True(t, len(publicIPs) >= 2) // One master and at least one node
	assert.True(t, len(privateIPs) == len(publicIPs))

	if numMachines < 0 {
		numMachines = len(publicIPs)
	}
	assert.True(t, numMachines >= 2)
	assert.True(t, numMachines <= len(publicIPs))

	// First machine will be master
	const numMasters = 1

	for i := 0; i < numMasters; i++ {
		appendMachine(t, i, &ml, &bl, clusterName, master, publicIPs[i], privateIPs[i])
	}

	// Subsequent machines will be nodes.
	for i := numMasters; i < numMachines; i++ {
		appendMachine(t, i, &ml, &bl, clusterName, node, publicIPs[i], privateIPs[i])
	}

	return ml, bl
}

func writeYamlManifests(t *testing.T, path string, machines []*clusterv1.Machine, bml []*baremetalspecv1.BareMetalMachine) {
	var buf bytes.Buffer
	err := machine.WriteMachines(&buf, machines, bml)
	assert.NoError(t, err)
	err = ioutil.WriteFile(path, buf.Bytes(), 0644)
	assert.NoError(t, err)
}

func numMasters(l []*clusterv1.Machine) int {
	n := 0
	for _, m := range l {
		if machine.IsMaster(m) {
			n++
		}
	}
	return n
}

func numWorkers(l []*clusterv1.Machine) int {
	n := 0
	for _, m := range l {
		if machine.IsNode(m) {
			n++
		}
	}
	return n
}

func setKubernetesVersion(l []*clusterv1.Machine, version string) {
	for i := range l {
		l[i].Spec.Version = &version
	}
}

func parseClusterManifest(t *testing.T, file string) (*clusterv1.Cluster, *baremetalspecv1.BareMetalCluster) {
	f, err := os.Open(file)
	assert.NoError(t, err)
	cluster, bmCluster, err := specs.ParseCluster(f)
	assert.NoError(t, err)
	return cluster, bmCluster
}

// The installer names the kubeconfig file from the cluster namespace and name
// ~/.wks
func wksKubeconfig(t *testing.T) string {
	kubeconfig := clientcmd.RecommendedHomeFile
	_, err := os.Stat(kubeconfig)
	assert.NoError(t, err)

	return kubeconfig
}

func testApplyKubernetesVersion(t *testing.T, versionNumber string) {
	version := "v" + versionNumber
	test := kube.NewTest(t)
	defer test.Close()
	client := kube.KubeClient()
	v, err := client.Discovery().ServerVersion()
	assert.NoError(t, err)
	assert.Equal(t, version, v.GitVersion)
	nodes := test.ListNodes(metav1.ListOptions{})
	for _, n := range nodes.Items {
		assert.Equal(t, version, n.Status.NodeInfo.KubeletVersion)
	}
}

func testKubectl(t *testing.T, kubeconfig string) {
	exe := run.NewExecutor()

	run, err := exe.RunV(kubectl, fmt.Sprintf("--kubeconfig=%s", kubeconfig), "get", "nodes")
	assert.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode())
	assert.True(t, run.Contains("Ready"))
}

func testDebugLogging(t *testing.T, kubeconfig string) {
	exe := run.NewExecutor()

	run, err := exe.RunV(kubectl,
		fmt.Sprintf("--kubeconfig=%s", kubeconfig), "get", "pods", "-l", "name=wks-controller", "--namespace=default", "-o", "jsonpath={.items[].spec.containers[].command}")
	assert.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode())
	verbose := false
	if run.Contains("--verbose") {
		verbose = true
	}

	run, err = exe.RunV(kubectl,
		fmt.Sprintf("--kubeconfig=%s", kubeconfig), "logs", "-l", "name=wks-controller", "--namespace=default")
	assert.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode())
	if verbose {
		assert.True(t, run.Contains("level=debug"))
	} else {
		assert.False(t, run.Contains("level=debug"))
	}
}

func nodeIsMaster(n *v1.Node) bool {
	const masterLabel = "node-role.kubernetes.io/master"
	if _, ok := n.Labels[masterLabel]; ok {
		return true
	}
	return false
}

func nodesNumMasters(l *v1.NodeList) int {
	n := 0
	for i := range l.Items {
		node := &l.Items[i]
		if nodeIsMaster(node) {
			n++
		}
	}
	return n
}

func nodesNumWorkers(l *v1.NodeList) int {
	n := 0
	for i := range l.Items {
		node := &l.Items[i]
		if !nodeIsMaster(node) {
			n++
		}
	}
	return n
}

func testNodes(t *testing.T, numMasters, numWorkers int) {
	test := kube.NewTest(t)
	defer test.Close()
	// Wait for two nodes to be available
	nodes := test.ListNodes(metav1.ListOptions{})
	for {
		if len(nodes.Items) == numMasters+numWorkers {
			break
		}
		log.Println("waiting for nodes - retrying in 10s")
		time.Sleep(10 * time.Second)
		nodes = test.ListNodes(metav1.ListOptions{})
	}
	assert.Equal(t, numMasters+numWorkers, len(nodes.Items))
	assert.Equal(t, numMasters, nodesNumMasters(nodes))
	assert.Equal(t, numWorkers, nodesNumWorkers(nodes))
}

// DOES NOT CURRENTLY WORK - NODES DO NOT POSSESS THESE LABELS
func testLabels(t *testing.T, numMasters, numWorkers int) {
	test := kube.NewTest(t)
	defer test.Close()

	masterNodes := test.ListNodes(metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			"set": setLabel(master),
		}).AsSelector().String(),
	})
	workerNodes := test.ListNodes(metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			"set": setLabel(node),
		}).AsSelector().String(),
	})
	assert.Equal(t, numMasters, len(masterNodes.Items))
	assert.Equal(t, numWorkers, len(workerNodes.Items))
}

func apply(exe *spawn.Executor, extra ...string) (*spawn.Entry, error) {
	args := []string{"apply"}
	args = append(args, extra...)
	return exe.RunV(cmd, args...)
}

func kubeconfig(exe *spawn.Executor, extra ...string) (*spawn.Entry, error) {
	args := []string{"kubeconfig"}
	args = append(args, extra...)
	return exe.RunV(cmd, args...)
}

func krb5Kubeconfig(exe *spawn.Executor, extra ...string) (*spawn.Entry, error) {
	args := []string{"krb5-kubeconfig"}
	args = append(args, extra...)
	return exe.RunV(cmd, args...)
}

func configPath(filename string) string {
	return filepath.Join(configDir, filename)
}

func writeFile(content []byte, dstPath string, perm os.FileMode, runner *ssh.Client) error {
	input := bytes.NewReader(content)
	cmd := fmt.Sprintf("mkdir -pv $(dirname %q) && sed -n 'w %s' && chmod 0%o %q", dstPath, dstPath, perm, dstPath)
	_, err := runner.RunCommand(cmd, input)
	return err
}

func writeTmpFile(runner *ssh.Client, inputFilename, outputFilename string) error {
	contents, err := ioutil.ReadFile(inputFilename)
	if err != nil {
		return err
	}
	return writeFile(contents, filepath.Join("/tmp", outputFilename), 0777, runner)
}

func TestApply(t *testing.T) {
	assert.NoError(t, clusterv1.AddToScheme(scheme.Scheme))
	assert.NoError(t, baremetalspecv1.AddToScheme(scheme.Scheme))

	exe := run.NewExecutor()

	// Prepare the machines manifest from terraform output.
	terraform, err := newTerraformOutputFromFile(options.terraform.outputPath)
	require.NoError(t, err)

	machines, bmMachines := makeMachinesFromTerraform(t, c.Name, terraform, terraform.numMachines()-1)
	setKubernetesVersion(machines, kubernetes.DefaultVersion)
	writeYamlManifests(t, configPath("machines.yaml"), machines, bmMachines)

	// Generate bad version to check failure return codes
	savedAddress := bmMachines[0].Spec.PrivateAddress
	bmMachines[0].Spec.PrivateAddress = "192.168.111.111"
	writeYamlManifests(t, configPath("badmachines.yaml"), machines, bmMachines)
	bmMachines[0].Spec.PrivateAddress = savedAddress

	clusterManifestPath := configPath("cluster.yaml")
	machinesManifestPath := configPath("machines.yaml")
	_, c := parseClusterManifest(t, clusterManifestPath)
	_, m := machine.FirstMaster(machines, bmMachines)
	assert.NotNil(t, m)
	ip := m.Spec.Public.Address
	port := m.Spec.Public.Port
	sshClient, err := ssh.NewClient(ssh.ClientParams{
		User:           c.Spec.User,
		Host:           ip,
		Port:           port,
		PrivateKeyPath: sshKeyPath,
		PrintOutputs:   true,
	})
	require.NoError(t, err)
	err = writeTmpFile(sshClient, "/tmp/workspace/cmd/mock-https-authz-server/server", "authserver")
	assert.NoError(t, err)
	for _, authFile := range []string{"rootCA.pem", "server.crt", "server.key"} {
		err = writeTmpFile(sshClient, configPath(authFile), authFile)
		assert.NoError(t, err)
	}
	go func() {
		_, err := sshClient.RunCommand("/tmp/authserver --pem-dir=/tmp", nil)
		if err != nil {
			fmt.Printf("AUTHZ ERROR: %v", err)
		}
	}()

	// First test that bad apply returns non-zero exit code
	badMachinesManifestPath := configPath("badmachines.yaml")
	// Fail to install the cluster.
	run, _ := apply(exe, "--cluster="+clusterManifestPath, "--machines="+badMachinesManifestPath, "--namespace=default",
		"--config-directory="+configDir, "--sealed-secret-key="+configPath("ss.key"), "--sealed-secret-cert="+configPath("ss.cert"),
		"--verbose=true", "--ssh-key="+sshKeyPath)
	assert.Equal(t, 1, run.ExitCode())

	// Install the Cluster.
	run, err = apply(exe, "--cluster="+clusterManifestPath, "--machines="+machinesManifestPath, "--namespace=default",
		"--config-directory="+configDir, "--sealed-secret-key="+configPath("ss.key"), "--sealed-secret-cert="+configPath("ss.cert"),
		"--verbose=true", "--ssh-key="+sshKeyPath)
	assert.NoError(t, err)
	require.Equal(t, 0, run.ExitCode())

	// Extract the kubeconfig,
	run, err = kubeconfig(exe, "--cluster="+configPath("cluster.yaml"), "--machines="+configPath("machines.yaml"), "--namespace=default", "--ssh-key="+sshKeyPath)
	assert.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode())

	// Tell kube-state-harness about the location of the kubeconfig file.
	kubeconfig := wksKubeconfig(t)
	err = kube.SetKubeconfig(kubeconfig)
	assert.NoError(t, err)

	// Test we have the number of nodes we asked for.
	t.Run("Nodes", func(t *testing.T) {
		testNodes(t, numMasters(machines), numWorkers(machines))
	})

	//Test we have installed the specified version.
	t.Run("KubernetesVersion", func(t *testing.T) {
		testApplyKubernetesVersion(t, "1.14.1")
	})

	// Test we can run kubectl against the cluster.
	t.Run("kubectl", func(t *testing.T) {
		testKubectl(t, kubeconfig)
	})

	// Test the we are getting debug logging messages.
	t.Run("loglevel", func(t *testing.T) {
		testDebugLogging(t, kubeconfig)
	})
}
