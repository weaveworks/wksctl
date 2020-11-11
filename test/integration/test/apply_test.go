package test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	existinginfrav1 "github.com/weaveworks/cluster-api-provider-existinginfra/apis/cluster.weave.works/v1alpha3"
	capeimachine "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/cluster/machine"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/kubernetes"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/runners/ssh"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/specs"
	"github.com/weaveworks/wksctl/pkg/cluster/machine"
	spawn "github.com/weaveworks/wksctl/test/integration/spawn"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
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

func appendMachine(t *testing.T, ordinal int, ml *[]*clusterv1.Machine, bl *[]*existinginfrav1.ExistingInfraMachine, clusterName, role role, publicIP, privateIP string) {
	name := generateName(role, ordinal)
	spec := existinginfrav1.ExistingInfraMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cluster.weave.works/v1alpha3",
			Kind:       "ExistingInfraMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: existinginfrav1.MachineSpec{
			Public: existinginfrav1.EndPoint{
				Address: publicIP,
				Port:    22,
			},
			Private: existinginfrav1.EndPoint{
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
func makeMachinesFromTerraform(t *testing.T, clusterName string, terraform *terraformOutput, numMachines int) (ml []*clusterv1.Machine, bl []*existinginfrav1.ExistingInfraMachine) {
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

func writeYamlManifests(t *testing.T, path string, machines []*clusterv1.Machine, bml []*existinginfrav1.ExistingInfraMachine) {
	var buf bytes.Buffer
	err := machine.WriteMachines(&buf, machines, bml)
	assert.NoError(t, err)
	err = ioutil.WriteFile(path, buf.Bytes(), 0644)
	assert.NoError(t, err)
}

func numMasters(l []*clusterv1.Machine) int {
	n := 0
	for _, m := range l {
		if capeimachine.IsMaster(m) {
			n++
		}
	}
	return n
}

func numWorkers(l []*clusterv1.Machine) int {
	n := 0
	for _, m := range l {
		if capeimachine.IsNode(m) {
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

func parseClusterManifest(t *testing.T, file string) (*clusterv1.Cluster, *existinginfrav1.ExistingInfraCluster) {
	f, err := os.Open(file)
	assert.NoError(t, err)
	cluster, eiCluster, err := specs.ParseCluster(f)
	assert.NoError(t, err)
	return cluster, eiCluster
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

// func testDebugLogging(t *testing.T, kubeconfig string) {
//  exe := run.NewExecutor()

//  run, err := exe.RunV(kubectl,
//      fmt.Sprintf("--kubeconfig=%s", kubeconfig), "get", "pods", "-l", "name=wks-controller", "-o", "jsonpath={.items[].spec.containers[].command}")
//  assert.NoError(t, err)
//  assert.Equal(t, 0, run.ExitCode())
//  verbose := false
//  if run.Contains("--verbose") {
//      verbose = true
//  }

//  run, err = exe.RunV(kubectl,
//      fmt.Sprintf("--kubeconfig=%s", kubeconfig), "logs", "-l", "name=wks-controller")
//  assert.NoError(t, err)
//  assert.Equal(t, 0, run.ExitCode())
//  if verbose {
//      assert.True(t, run.Contains("level=debug"))
//  } else {
//      assert.False(t, run.Contains("level=debug"))
//  }
// }

func testCIDRBlocks(t *testing.T, kubeconfig string) {
	cmdItems := []string{kubectl,
		fmt.Sprintf("--kubeconfig=%s", kubeconfig), "get", "pods", "-l", "name=wks-controller", "-o", "jsonpath={.items[].status.podIP}", "--namespace=weavek8sops"}
	cmd := exec.Command(cmdItems[0], cmdItems[1:]...)
	podIP, err := cmd.CombinedOutput()
	log.Printf("wks-controller has IP: %s\n", string(podIP))
	assert.NoError(t, err)
	isValid, err := assertIPisWithinRange(string(podIP), "192.168.128.0/17")
	assert.NoError(t, err)
	log.Printf("Pod IP %s is inside 192.168.127.0/17 range? %v\n", podIP, isValid)
	assert.True(t, isValid)

	cmdItems = []string{kubectl,
		fmt.Sprintf("--kubeconfig=%s", kubeconfig), "get", "service", "kubernetes", "-o", "jsonpath={.spec.clusterIP}"}
	cmd = exec.Command(cmdItems[0], cmdItems[1:]...)
	serviceIP, err := cmd.CombinedOutput()
	log.Printf("kubernetes service has IP: %s\n", string(serviceIP))
	assert.NoError(t, err)
	isValid, err = assertIPisWithinRange(string(serviceIP), "172.20.0.0/23")
	assert.NoError(t, err)
	log.Printf("Service IP %s is inside 172.20.0.0/23 range? %v\n", serviceIP, isValid)
	assert.True(t, isValid)

}

func nodeIsMaster(n *v1.Node) bool {
	const masterLabel = "node-role.kubernetes.io/master"
	if _, ok := n.Labels[masterLabel]; ok {
		return true
	}
	return false
}

func assertIPisWithinRange(ip string, ipRange string) (bool, error) {
	_, subnet, err := net.ParseCIDR(ipRange)
	if err != nil {
		log.Printf("failed to parse CIDR from %s, err: %s\n", ipRange, err)
		return false, err
	}

	parsedIP := net.ParseIP(ip)
	return subnet.Contains(parsedIP), nil
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

func testNodes(t *testing.T, numMasters, numWorkers int, kubeconfig string) {
	test := kube.NewTest(t)
	defer test.Close()
	// Wait for two nodes to be available
	nodes := test.ListNodes(metav1.ListOptions{})
	for {
		if len(nodes.Items) == numMasters+numWorkers {
			break
		}
		log.Println("waiting for nodes - retrying in 10s")
		fmt.Printf("NODE COUNT: %d\n", len(nodes.Items))
		cmd := exec.Command(
			"sh", "-c", fmt.Sprintf("kubectl logs -l name=wks-controller --kubeconfig=%s",
				kubeconfig))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		cmdItems := []string{kubectl,
			fmt.Sprintf("--kubeconfig=%s", kubeconfig), "get", "pods", "-l", "name=wks-controller", "-o", "yaml"}
		cmd = exec.Command(cmdItems[0], cmdItems[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		time.Sleep(10 * time.Second)
		nodes = test.ListNodes(metav1.ListOptions{})
	}
	assert.Equal(t, numMasters+numWorkers, len(nodes.Items))
	assert.Equal(t, numMasters, nodesNumMasters(nodes))
	assert.Equal(t, numWorkers, nodesNumWorkers(nodes))
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

func configPath(filename string) string {
	return filepath.Join(configDir, filename)
}

func writeFile(content []byte, dstPath string, perm os.FileMode, runner *ssh.Client) error {
	input := bytes.NewReader(content)
	cmd := fmt.Sprintf("mkdir -pv $(dirname %q) && sed -n 'w %s' && chmod 0%o %q", dstPath, dstPath, perm, dstPath)
	_, err := runner.RunCommand(context.Background(), cmd, input)
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
	clusterManifestPath := configPath("cluster.yaml")
	_, c := parseClusterManifest(t, clusterManifestPath)

	exe := run.NewExecutor()

	// Prepare the machines manifest from terraform output.
	terraform, err := newTerraformOutputFromFile(options.terraform.outputPath)
	require.NoError(t, err)

	machines, eiMachines := makeMachinesFromTerraform(t, c.Name, terraform, terraform.numMachines()-1)
	setKubernetesVersion(machines, kubernetes.DefaultVersion)
	writeYamlManifests(t, configPath("machines.yaml"), machines, eiMachines)

	// Generate bad version to check failure return codes
	savedAddress := eiMachines[0].Spec.Private.Address
	eiMachines[0].Spec.Private.Address = "192.168.111.111"
	writeYamlManifests(t, configPath("badmachines.yaml"), machines, eiMachines)
	eiMachines[0].Spec.Private.Address = savedAddress

	machinesManifestPath := configPath("machines.yaml")
	_, m := capeimachine.FirstMaster(machines, eiMachines)
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
		_, err := sshClient.RunCommand(context.Background(), "/tmp/authserver --pem-dir=/tmp", nil)
		if err != nil {
			fmt.Printf("AUTHZ ERROR: %v", err)
		}
	}()

	// First test that bad apply returns non-zero exit code
	badMachinesManifestPath := configPath("badmachines.yaml")
	// Fail to install the cluster.
	run, _ := apply(exe, "--cluster="+clusterManifestPath, "--machines="+badMachinesManifestPath,
		"--config-directory="+configDir, "--sealed-secret-key="+configPath("ss.key"), "--sealed-secret-cert="+configPath("ss.cert"),
		"--verbose=true", "--ssh-key="+sshKeyPath)
	assert.Equal(t, 1, run.ExitCode())

	// Install the Cluster.
	run, err = apply(exe, "--cluster="+clusterManifestPath, "--machines="+machinesManifestPath,
		"--config-directory="+configDir, "--sealed-secret-key="+configPath("ss.key"), "--sealed-secret-cert="+configPath("ss.cert"),
		// FIXME: controller-image flag doesn't seem to work right now so don't use
		// "--controller-image=docker.io/weaveworks/cluster-api-existinginfra-controller:v0.0.8",
		"--verbose=true", "--ssh-key="+sshKeyPath)
	assert.NoError(t, err)
	require.Equal(t, 0, run.ExitCode())

	// Extract the kubeconfig,
	run, err = kubeconfig(exe, "--cluster="+configPath("cluster.yaml"), "--machines="+configPath("machines.yaml"), "--ssh-key="+sshKeyPath)
	assert.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode())

	// Tell kube-state-harness about the location of the kubeconfig file.
	kubeconfig := wksKubeconfig(t)
	err = kube.SetKubeconfig(kubeconfig)
	assert.NoError(t, err)
	conf := exec.Command("sudo", "cat", "/root/.kube/config")
	conf.Stdout = os.Stdout
	conf.Stderr = os.Stderr
	_ = conf.Run()

	// Test we have the number of nodes we asked for.
	t.Run("Nodes", func(t *testing.T) {
		testNodes(t, numMasters(machines), numWorkers(machines), kubeconfig)
	})

	t.Log("Waiting 1 minute for nodes to settle")
	time.Sleep(1 * time.Minute)

	//Test we have installed the specified version.
	t.Run("KubernetesVersion", func(t *testing.T) {
		testApplyKubernetesVersion(t, "1.17.13")
	})

	// Test we can run kubectl against the cluster.
	t.Run("kubectl", func(t *testing.T) {
		testKubectl(t, kubeconfig)
	})

	// // Test the we are getting debug logging messages.
	// t.Run("loglevel", func(t *testing.T) {
	//  testDebugLogging(t, kubeconfig)
	// })

	// Test that the pods.cidrBlocks are passed to weave-net
	t.Run("CIDRBlocks", func(t *testing.T) {
		testCIDRBlocks(t, kubeconfig)
	})
}
