package test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	"github.com/weaveworks/wksctl/pkg/kubernetes"
	"github.com/weaveworks/wksctl/pkg/plan/runners/ssh"

	yaml "github.com/ghodss/yaml"
	baremetalspecv1 "github.com/weaveworks/wksctl/pkg/baremetalproviderspec/v1alpha1"
	spawn "github.com/weaveworks/wksctl/test/integration/spawn"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/stretchr/testify/assert"
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

func generateName(role role) string {
	switch role {
	case master:
		return "master-"
	case node:
		return "node-"
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

func appendMachine(t *testing.T, l *clusterv1.MachineList, role role, publicIP, privateIP string) {
	// Create a BareMetalMachineProviderSpec and encode it.
	spec := &baremetalspecv1.BareMetalMachineProviderSpec{
		Public: baremetalspecv1.EndPoint{
			Address: publicIP,
			Port:    22,
		},
		Private: baremetalspecv1.EndPoint{
			Address: privateIP,
			Port:    22,
		},
	}
	codec, err := baremetalspecv1.NewCodec()
	assert.NoError(t, err)
	encodedSpec, err := codec.EncodeToProviderSpec(spec)
	assert.NoError(t, err)

	// Create a machine.
	machine := clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.k8s.io/v1alpha1",
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName(role),
			Labels: map[string]string{
				"set": setLabel(role),
			},
		},
		Spec: clusterv1.MachineSpec{
			ProviderSpec: *encodedSpec,
		},
	}

	l.Items = append(l.Items, machine)
}

// makeMachinesFromTerraform creates cluster-api Machine objects from a
// terraform output. The terraform output must have two variables:
//  - "public_ips": list of public IPs
//  - "private_ips": list of private IPs (duh!)
//
// numMachines is the number of machines to use. It can be less than the number
// of provisionned terraform machines. -1 means use all machines setup by
// terraform. The minimum number of machines to use is 2.
func makeMachinesFromTerraform(t *testing.T, terraform *terraformOutput, numMachines int) *clusterv1.MachineList {
	l := &clusterv1.MachineList{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.k8s.io/v1alpha1",
			Kind:       "MachineList",
		},
	}
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
		appendMachine(t, l, master, publicIPs[i], privateIPs[i])
	}

	// Subsequent machines will be nodes.
	for i := numMasters; i < numMachines; i++ {
		appendMachine(t, l, node, publicIPs[i], privateIPs[i])
	}

	return l
}

func writeYamlManifest(t *testing.T, o interface{}, path string) {
	data, err := yaml.Marshal(o)
	assert.NoError(t, err)
	err = ioutil.WriteFile(path, data, 0644)
	assert.NoError(t, err)
}

func firstMaster(l *clusterv1.MachineList) *clusterv1.Machine {
	for i := range l.Items {
		m := &l.Items[i]
		if machine.IsMaster(m) {
			return m
		}
	}
	return nil
}

func numMasters(l *clusterv1.MachineList) int {
	n := 0
	for i := range l.Items {
		m := &l.Items[i]
		if machine.IsMaster(m) {
			n++
		}
	}
	return n
}

func numWorkers(l *clusterv1.MachineList) int {
	n := 0
	for i := range l.Items {
		m := &l.Items[i]
		if machine.IsNode(m) {
			n++
		}
	}
	return n
}

func setKubernetesVersion(l *clusterv1.MachineList, version string) {
	for i := range l.Items {
		m := &l.Items[i]
		m.Spec.Versions.Kubelet = version
		if machine.IsMaster(m) {
			m.Spec.Versions.ControlPlane = version
		}
	}
}

func machineSpec(t *testing.T, machine *clusterv1.Machine) *baremetalspecv1.BareMetalMachineProviderSpec {
	codec, err := baremetalspecv1.NewCodec()
	assert.NoError(t, err)
	spec, err := codec.MachineProviderFromProviderSpec(machine.Spec.ProviderSpec)
	assert.NoError(t, err)
	return spec
}

func clusterSpec(t *testing.T, cluster *clusterv1.Cluster) *baremetalspecv1.BareMetalClusterProviderSpec {
	codec, err := baremetalspecv1.NewCodec()
	assert.NoError(t, err)
	spec, err := codec.ClusterProviderFromProviderSpec(cluster.Spec.ProviderSpec)
	assert.NoError(t, err)
	return spec
}

func parseCluster(t *testing.T, r io.Reader) *clusterv1.Cluster {
	bytes, err := ioutil.ReadAll(r)
	assert.NoError(t, err)
	cluster := &clusterv1.Cluster{}
	err = yaml.Unmarshal(bytes, cluster)
	assert.NoError(t, err)
	return cluster

}

func parseClusterManifest(t *testing.T, file string) *clusterv1.Cluster {
	f, err := os.Open(file)
	assert.NoError(t, err)
	defer f.Close()
	return parseCluster(t, f)
}

func getClusterNamespaceAndName(t *testing.T) (string, string) {
	cluster := parseClusterManifest(t, "assets/cluster.yaml")
	meta := cluster.ObjectMeta
	name := meta.Name
	namespace := meta.Namespace
	if namespace == "" {
		return "default", name
	}
	return namespace, name
}

// The installer names the kubeconfig file from the cluster namespace and name
// ~/.wks
func wksKubeconfig(t *testing.T, l *clusterv1.MachineList) string {
	currentUser, err := user.Current()
	assert.NoError(t, err)
	master := machine.FirstMasterInArray(l.Items)
	assert.NotNil(t, master)
	namespace, name := getClusterNamespaceAndName(t)
	kubeconfig := path.Join(currentUser.HomeDir, ".wks", namespace, name, "kubeconfig")
	_, err = os.Stat(kubeconfig)
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
	exe := run.NewExecutor()

	// Prepare the machines manifest from terraform output.
	terraform, err := newTerraformOutputFromFile(options.terraform.outputPath)
	assert.NoError(t, err)

	machines := makeMachinesFromTerraform(t, terraform, terraform.numMachines()-1)
	setKubernetesVersion(machines, kubernetes.DefaultVersion)
	writeYamlManifest(t, machines, configPath("machines.yaml"))

	clusterManifestPath := configPath("cluster.yaml")
	machinesManifestPath := configPath("machines.yaml")
	clusterBytes, err := ioutil.ReadFile(clusterManifestPath)
	assert.NoError(t, err)
	cluster := &clusterv1.Cluster{}
	err = yaml.Unmarshal(clusterBytes, cluster)
	assert.NoError(t, err)
	cSpec := clusterSpec(t, cluster)
	master := firstMaster(machines)
	mSpec := machineSpec(t, master)
	ip := mSpec.Public.Address
	port := mSpec.Public.Port
	sshClient, err := ssh.NewClient(ssh.ClientParams{
		User:           cSpec.User,
		Host:           ip,
		Port:           port,
		PrivateKeyPath: sshKeyPath,
		PrintOutputs:   true,
	})
	assert.NoError(t, err)
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

	// Install the Cluster.
	run, err := apply(exe, "--cluster="+clusterManifestPath, "--machines="+machinesManifestPath, "--namespace=default",
		"--config-directory="+configDir, "--sealed-secret-key="+configPath("ss.key"), "--sealed-secret-cert="+configPath("ss.cert"),
		"--verbose=true", "--ssh-key="+sshKeyPath)
	assert.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode())

	// Extract the kubeconfig,
	run, err = kubeconfig(exe, "--cluster="+configPath("cluster.yaml"), "--machines="+configPath("machines.yaml"), "--namespace=default", "--ssh-key="+sshKeyPath)
	assert.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode())

	// Tell kube-state-harness about the location of the kubeconfig file.
	kubeconfig := wksKubeconfig(t, machines)
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
}
