package container

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/wksctl/pkg/cluster/nodes"
	corev1 "k8s.io/api/core/v1"
)

const clusterYAML = `apiVersion: cluster.k8s.io/v1alpha1
kind: Cluster
metadata:
  name: test-multimaster
spec:
  clusterNetwork:
    services:
      cidrBlocks: [10.96.0.0/12]
    pods:
      cidrBlocks: [192.168.0.0/16]
    serviceDomain: cluster.local
  providerSpec:
    value:
      apiVersion: baremetalproviderspec/v1alpha1
      kind: BareMetalClusterProviderSpec
      user: root
      imageRepository: %s:%d
      os:
        files:
        - source:
            configmap: repo
            key: kubernetes.repo
          destination: /etc/yum.repos.d/kubernetes.repo
        - source:
            configmap: repo
            key: docker-ce.repo
          destination: /etc/yum.repos.d/docker-ce.repo
        - source:
            configmap: docker
            key: daemon.json
          destination: /etc/docker/daemon.json
        - source:
            configmap: repo
            key: local.repo
          destination: /etc/yum.repos.d/local.repo
      cri:
        kind: docker
        package: docker-ce
        version: 19.03.8
      kubeletArguments:
      - name: alsologtostderr
        value: "true"
      - name: container-runtime
        value: docker
      apiServer:
        extraArguments:
        - name: alsologtostderr
          value: "true"
        - name: audit-log-maxsize
          value: "10000"
`

const clusterYAMLUbuntu = `apiVersion: cluster.k8s.io/v1alpha1
kind: Cluster
metadata:
  name: test-multimaster
spec:
  clusterNetwork:
    services:
      cidrBlocks: [10.96.0.0/12]
    pods:
      cidrBlocks: [192.168.0.0/16]
    serviceDomain: cluster.local
  providerSpec:
    value:
      apiVersion: baremetalproviderspec/v1alpha1
      kind: BareMetalClusterProviderSpec
      user: root
      imageRepository: %s:%d
      os:
        files:
        - source:
            configmap: repo
            key: kubernetes.repo
          destination: /etc/apt/sources.list.d/kubernetes.repo
        - source:
            configmap: repo
            key: docker-ce.repo
          destination: /etc/apt/sources.list.d/docker-ce.repo
        - source:
            configmap: docker
            key: daemon.json
          destination: /etc/docker/daemon.json
        - source:
            configmap: repo
            key: local.repo
          destination: /etc/apt/sources.list.d/local.repo
        - source:
            configmap: repo
            key: cloud-google-com.gpg.b64
          destination: /tmp/cloud-google-com.gpg.b64
      cri:
        kind: docker
        package: docker-ce
        version: 19.03.8
      kubeletArguments:
      - name: alsologtostderr
        value: "true"
      - name: container-runtime
        value: docker
      apiServer:
        extraArguments:
        - name: alsologtostderr
          value: "true"
        - name: audit-log-maxsize
          value: "10000"
`

const machinesYAML = `apiVersion: v1
kind: List
items:
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    name: master-1
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: baremetalproviderspec/v1alpha1
        kind: BareMetalMachineProviderSpec
        public:
          address: 127.0.0.1
          port: 2222
        private:
          address: %s
          port: 22
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    name: master-2
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: baremetalproviderspec/v1alpha1
        kind: BareMetalMachineProviderSpec
        public:
          address: 127.0.0.1
          port: 2223
        private:
          address: %s
          port: 22
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    name: master-3
    labels:
      set: master
  spec:
    providerSpec:
      value:
        apiVersion: baremetalproviderspec/v1alpha1
        kind: BareMetalMachineProviderSpec
        public:
          address: 127.0.0.1
          port: 2224
        private:
          address: %s
          port: 22
- apiVersion: cluster.k8s.io/v1alpha1
  kind: Machine
  metadata:
    name: worker-1
    labels:
      set: worker
  spec:
    providerSpec:
      value:
        apiVersion: baremetalproviderspec/v1alpha1
        kind: BareMetalMachineProviderSpec
        public:
          address: 127.0.0.1
          port: 2225
        private:
          address: %s
          port: 22
`

const dockerConfigMap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: docker
  namespace: system
data:
  daemon.json: |
    {
      "insecure-registries" : ["%s:%d"],
      "log-driver": "json-file",
      "log-opts": {
        "max-size": "100m"
      },
      "exec-opts": [
        "native.cgroupdriver=cgroupfs"
      ]
    }
`

const repoConfigMap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: repo
  namespace: system
data:
  kubernetes.repo: |
    [kubernetes]
    name=Kubernetes
    baseurl=https://packages.cloud.google.com/yum/repos/kubernetes-el7-x86_64
    enabled=1
    gpgcheck=1
    repo_gpgcheck=1
    gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
    exclude=kube*
  docker-ce.repo: |
    [docker-ce-stable]
    name=Docker CE Stable - $basearch
    baseurl=https://download.docker.com/linux/centos/7/$basearch/stable
    enabled=1
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-stable-debuginfo]
    name=Docker CE Stable - Debuginfo $basearch
    baseurl=https://download.docker.com/linux/centos/7/debug-$basearch/stable
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-stable-source]
    name=Docker CE Stable - Sources
    baseurl=https://download.docker.com/linux/centos/7/source/stable
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-edge]
    name=Docker CE Edge - $basearch
    baseurl=https://download.docker.com/linux/centos/7/$basearch/edge
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-edge-debuginfo]
    name=Docker CE Edge - Debuginfo $basearch
    baseurl=https://download.docker.com/linux/centos/7/debug-$basearch/edge
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-edge-source]
    name=Docker CE Edge - Sources
    baseurl=https://download.docker.com/linux/centos/7/source/edge
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-test]
    name=Docker CE Test - $basearch
    baseurl=https://download.docker.com/linux/centos/7/$basearch/test
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-test-debuginfo]
    name=Docker CE Test - Debuginfo $basearch
    baseurl=https://download.docker.com/linux/centos/7/debug-$basearch/test
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-test-source]
    name=Docker CE Test - Sources
    baseurl=https://download.docker.com/linux/centos/7/source/test
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-nightly]
    name=Docker CE Nightly - $basearch
    baseurl=https://download.docker.com/linux/centos/7/$basearch/nightly
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-nightly-debuginfo]
    name=Docker CE Nightly - Debuginfo $basearch
    baseurl=https://download.docker.com/linux/centos/7/debug-$basearch/nightly
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-nightly-source]
    name=Docker CE Nightly - Sources
    baseurl=https://download.docker.com/linux/centos/7/source/nightly
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

  local.repo: |
    [local]
    name=Local
    baseurl=http://%s
    enabled=1
    gpgcheck=0
`

const repoConfigMapUbuntu = `apiVersion: v1
kind: ConfigMap
metadata:
  name: repo
  namespace: system
data:
  kubernetes.repo: |
    [kubernetes]
    name=Kubernetes
    baseurl=https://packages.cloud.google.com/yum/repos/kubernetes-el7-x86_64
    enabled=1
    gpgcheck=1
    repo_gpgcheck=1
    gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
    exclude=kube*
  docker-ce.repo: |
    [docker-ce-stable]
    name=Docker CE Stable - $basearch
    baseurl=https://download.docker.com/linux/centos/7/$basearch/stable
    enabled=1
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-stable-debuginfo]
    name=Docker CE Stable - Debuginfo $basearch
    baseurl=https://download.docker.com/linux/centos/7/debug-$basearch/stable
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-stable-source]
    name=Docker CE Stable - Sources
    baseurl=https://download.docker.com/linux/centos/7/source/stable
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-edge]
    name=Docker CE Edge - $basearch
    baseurl=https://download.docker.com/linux/centos/7/$basearch/edge
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-edge-debuginfo]
    name=Docker CE Edge - Debuginfo $basearch
    baseurl=https://download.docker.com/linux/centos/7/debug-$basearch/edge
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-edge-source]
    name=Docker CE Edge - Sources
    baseurl=https://download.docker.com/linux/centos/7/source/edge
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-test]
    name=Docker CE Test - $basearch
    baseurl=https://download.docker.com/linux/centos/7/$basearch/test
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-test-debuginfo]
    name=Docker CE Test - Debuginfo $basearch
    baseurl=https://download.docker.com/linux/centos/7/debug-$basearch/test
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-test-source]
    name=Docker CE Test - Sources
    baseurl=https://download.docker.com/linux/centos/7/source/test
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-nightly]
    name=Docker CE Nightly - $basearch
    baseurl=https://download.docker.com/linux/centos/7/$basearch/nightly
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-nightly-debuginfo]
    name=Docker CE Nightly - Debuginfo $basearch
    baseurl=https://download.docker.com/linux/centos/7/debug-$basearch/nightly
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

    [docker-ce-nightly-source]
    name=Docker CE Nightly - Sources
    baseurl=https://download.docker.com/linux/centos/7/source/nightly
    enabled=0
    gpgcheck=1
    gpgkey=https://download.docker.com/linux/centos/gpg

  cloud-google-com.gpg.b64: |
    mQENBFUd6rIBCAD6mhKRHDn3UrCeLDp7U5IE7AhhrOCPpqGF7mfTemZYHf/5JdjxcOxoSFlK7zwm
    Fr3lVqJ+tJ9L1wd1K6P7RrtaNwCiZyeNPf/Y86AJ5NJwBe0VD0xHTXzPNTqRSByVYtdN94NoltXU
    YFAAPZYQls0x0nUD1hLMlOlC2HdTPrD1PMCnYq/NuL/Vk8sWrcUt4DIS+0RDQ8tKKe5PSV0+Pnma
    JvdF5CKawhh0qGTklS2MXTyKFoqjXgYDfY2EodI9ogT/LGr9Lm/+u4OFPvmN9VN6UG+s0DgJjWvp
    bmuHL/ZIRwMEn/tpuneaLTO7h1dCrXC849PiJ8wSkGzBnuJQUbXnABEBAAG0QEdvb2dsZSBDbG91
    ZCBQYWNrYWdlcyBBdXRvbWF0aWMgU2lnbmluZyBLZXkgPGdjLXRlYW1AZ29vZ2xlLmNvbT6JAT4E
    EwECACgFAlUd6rICGy8FCQWjmoAGCwkIBwMCBhUIAgkKCwQWAgMBAh4BAheAAAoJEDdGwginMXsP
    cLcIAKi2yNhJMbu4zWQ2tM/rJFovazcY28MF2rDWGOnc9giHXOH0/BoMBcd8rw0lgjmOosBdM2JT
    0HWZIxC/Gdt7NSRA0WOlJe04u82/o3OHWDgTdm9MS42noSP0mvNzNALBbQnlZHU0kvt3sV1Ysnrx
    ljoIuvxKWLLwren/GVshFLPwONjw3f9Fan6GWxJyn/dkX3OSUGaduzcygw51vksBQiUZLCD2Tlxy
    r9NvkZYTqiaWW78L6regvATsLc9L/dQUiSMQZIK6NglmHE+cuSaoK0H4ruNKeTiQUw/EGFaLecay
    6Qy/s3Hk7K0QLd+gl0hZ1w1VzIeXLo2BRlqnjOYFX4CwAgADmQENBFrBaNsBCADrF18KCbsZlo4N
    jAvVecTBCnp6WcBQJ5oSh7+E98jX9YznUCrNrgmeCcCMUvTDRDxfTaDJybaHugfba43nqhkbNpJ4
    7YXsIa+YL6eEE9emSmQtjrSWIiY+2YJYwsDgsgckF3duqkb02OdBQlh6IbHPoXB6H//b1PgZYsom
    B+841XW1LSJPYlYbIrWfwDfQvtkFQI90r6NknVTQlpqQh5GLNWNYqRNrGQPmsB+NrUYrkl1nUt1L
    RGu+rCe4bSaSmNbwKMQKkROE4kTiB72DPk7zH4Lm0uo0YFFWG4qsMIuqEihJ/9KNX8GYBr+tWgyL
    ooLlsdK3l+4dVqd8cjkJM1ExABEBAAG0QEdvb2dsZSBDbG91ZCBQYWNrYWdlcyBBdXRvbWF0aWMg
    U2lnbmluZyBLZXkgPGdjLXRlYW1AZ29vZ2xlLmNvbT6JAT4EEwECACgFAlrBaNsCGy8FCQWjmoAG
    CwkIBwMCBhUIAgkKCwQWAgMBAh4BAheAAAoJEGoDCyG6B/T78e8H/1WH2LN/nVNhm5TS1VYJG8B+
    IW8zS4BqyozxC9iJAJqZIVHXl8g8a/Hus8RfXR7cnYHcg8sjSaJfQhqO9RbKnffiuQgGrqwQxuC2
    jBa6M/QKzejTeP0Mgi67pyrLJNWrFI71RhritQZmzTZ2PoWxfv6b+Tv5v0rPaG+ut1J47pn+kYgt
    UaKdsJz1umi6HzK6AacDf0C0CksJdKG7MOWsZcB4xeOxJYuy6NuO6KcdEz8/XyEUjIuIOlhYTd0h
    H8E/SEBbXXft7/VBQC5wNq40izPi+6WFK/e1O42DIpzQ749ogYQ1eodexPNhLzekKR3XhGrNXJ95
    r5KO10VrsLFNd8KwAgAD

  local.repo: |
    [local]
    name=Local
    baseurl=http://%s
    enabled=1
    gpgcheck=0
`

/*func TestMultimasterSetup(t *testing.T) {
	tag := imageTag(t)
	registryPort := port(t, "REGISTRY_PORT", 5000)
	repositoryPort := port(t, "REPOSITORY_PORT", 8080)

	// Ensure the local Docker registry is running:
	if out := runIgnoreError(t, "docker", "inspect", "-f", "'{{.State.Running}}'", "registry"); !strings.Contains(out, "true") {
		run(t, "docker", "run", "-d", "-p", fmt.Sprintf("%d:5000", registryPort), "--restart", "always", "-v", "/tmp/registry:/var/lib/registry", "--name", "registry", "registry:2")
		waitForLocalRegistryToStart(t, registryPort)
	}
	if shouldRetagPush(t, registryPort) {
		run(t, "../../../environments/local-docker-registry/retag_push.sh", "-p", strconv.Itoa(registryPort))
	}
	run(t, "docker", "tag", fmt.Sprintf("docker.io/weaveworks/wksctl-controller:%s", tag), fmt.Sprintf("localhost:%d/weaveworks/wksctl-controller:%s", registryPort, tag))
	run(t, "docker", "push", fmt.Sprintf("localhost:%d/weaveworks/wksctl-controller:%s", registryPort, tag))
	registryIP := sanitizeIP(run(t, "docker", "inspect", "registry", "--format='{{.NetworkSettings.IPAddress}}'"))

	// Ensure the local YUM repo is running:
	if out := runIgnoreError(t, "docker", "inspect", "-f", "'{{.State.Running}}'", "yumrepo"); !strings.Contains(out, "true") {
		run(t, "docker", "run", "-d", "-p", fmt.Sprintf("%d:80", repositoryPort), "--restart", "always", "--name", "yumrepo", "weaveworks/local-yum-repo:master-7b063513")
	}
	yumRepoIP := sanitizeIP(run(t, "docker", "inspect", "yumrepo", "--format='{{.NetworkSettings.IPAddress}}'"))

	// Start the footloose container "VMs" used for testing:
	run(t, "footloose", "create", "-c", "../../../examples/footloose/centos7/docker/multimaster.yaml")
	node0IP := sanitizeIP(run(t, "docker", "inspect", "centos-multimaster-node0", "--format='{{.NetworkSettings.IPAddress}}'"))
	node1IP := sanitizeIP(run(t, "docker", "inspect", "centos-multimaster-node1", "--format='{{.NetworkSettings.IPAddress}}'"))
	node2IP := sanitizeIP(run(t, "docker", "inspect", "centos-multimaster-node2", "--format='{{.NetworkSettings.IPAddress}}'"))
	node3IP := sanitizeIP(run(t, "docker", "inspect", "centos-multimaster-node3", "--format='{{.NetworkSettings.IPAddress}}'"))

	dirName := tempDir(t)
	clusterYAML := saveToFile(t, dirName, "cluster.yaml", fmt.Sprintf(clusterYAML, registryIP, registryPort))
	machinesYAML := saveToFile(t, dirName, "machines.yaml", fmt.Sprintf(machinesYAML, node0IP, node1IP, node2IP, node3IP))
	_ = saveToFile(t, dirName, "repo-config.yaml", fmt.Sprintf(repoConfigMap, yumRepoIP))
	_ = saveToFile(t, dirName, "docker-config.yaml", fmt.Sprintf(dockerConfigMap, registryIP, registryPort))

	runWithStdout(t, "../../../cmd/wksctl/wksctl", "apply",
		fmt.Sprintf("--cluster=%s", clusterYAML), fmt.Sprintf("--machines=%s", machinesYAML),
		fmt.Sprintf("--config-directory=%s", dirName),
		"--verbose",
		fmt.Sprintf("--controller-image=docker.io/weaveworks/wksctl-controller:%s", tag))

	out := run(t, "../../../cmd/wksctl/wksctl", "kubeconfig",
		fmt.Sprintf("--cluster=%s", clusterYAML), fmt.Sprintf("--machines=%s", machinesYAML))

	var nodeList corev1.NodeList
	for {
		jsonOut := run(t, "kubectl", "get", "nodes", "-o", "json", fmt.Sprintf("--kubeconfig=%s", kubeconfig(out)))
		if err := json.Unmarshal([]byte(jsonOut), &nodeList); err != nil {
			log.Warnf("Error deserialising output of kubectl get nodes: %s", err)
		}
		log.Infof("The cluster currently has %d node(s)", len(nodeList.Items))
		if len(nodeList.Items) == 4 {
			break
		}
		time.Sleep(30 * time.Second)
	}

	assert.Len(t, nodeList.Items, 4)
	assert.Len(t, nodes.Masters(nodeList).Items, 3)
	assert.Len(t, nodes.Workers(nodeList).Items, 1)

	expectedKubeletArgs := []string{"alsologtostderr=true", "container-runtime=docker"}
	expectedApiServerArgs := []string{"alsologtostderr=true", "audit-log-maxsize=10000"}

	for i := 0; i < 4; i++ {
		for _, kubeletArg := range expectedKubeletArgs {
			log.Infof("Checking kubelet arg (%s) on node%d", kubeletArg, i)
			run(t, "footloose",
				"-c", "../../../examples/footloose/centos7/docker/multimaster.yaml",
				"ssh", fmt.Sprintf("root@node%d", i), fmt.Sprintf("ps -ef | grep -v 'ps -ef' | grep /usr/bin/kubelet | grep %s", kubeletArg))
		}

		// node0 - node2 are masters
		if i < 3 {
			for _, apiServerArg := range expectedApiServerArgs {
				log.Infof("Checking api server arg (%s) on node%d", apiServerArg, i)
				run(t, "footloose",
					"-c", "../../../examples/footloose/centos7/docker/multimaster.yaml",
					"ssh", fmt.Sprintf("root@node%d", i), fmt.Sprintf("ps -ef | grep -v 'ps -ef' | grep kube-apiserver | grep %s", apiServerArg))
			}
		}
	}

	if !t.Failed() { // Otherwise leave the footloose "VMs" & config files around for debugging purposes.
		// Clean up:
		defer run(t, "footloose", "delete", "-c", "../../../examples/footloose/centos7/docker/multimaster.yaml")
		defer os.Remove(dirName)
		defer os.Remove(clusterYAML)
		defer os.Remove(machinesYAML)
		defer os.Remove(repoConfigMap)
		defer os.Remove(repoConfigMap)
	}
}*/

func TestMultimasterSetupUbuntu(t *testing.T) {
	tag := imageTag(t)
	registryPort := port(t, "REGISTRY_PORT", 5000)
	repositoryPort := port(t, "REPOSITORY_PORT", 8080)

	// Ensure the local Docker registry is running:
	if out := runIgnoreError(t, "docker", "inspect", "-f", "'{{.State.Running}}'", "registry"); !strings.Contains(out, "true") {
		run(t, "docker", "run", "-d", "-p", fmt.Sprintf("%d:5000", registryPort), "--restart", "always", "-v", "/tmp/registry:/var/lib/registry", "--name", "registry", "registry:2")
		waitForLocalRegistryToStart(t, registryPort)
	}
	if shouldRetagPush(t, registryPort) {
		run(t, "../../../environments/local-docker-registry/retag_push.sh", "-p", strconv.Itoa(registryPort))
	}
	run(t, "docker", "tag", fmt.Sprintf("docker.io/weaveworks/wksctl-controller:%s", tag), fmt.Sprintf("localhost:%d/weaveworks/wksctl-controller:%s", registryPort, tag))
	run(t, "docker", "push", fmt.Sprintf("localhost:%d/weaveworks/wksctl-controller:%s", registryPort, tag))
	registryIP := sanitizeIP(run(t, "docker", "inspect", "registry", "--format='{{.NetworkSettings.IPAddress}}'"))

	// Ensure the local YUM repo is running:
	if out := runIgnoreError(t, "docker", "inspect", "-f", "'{{.State.Running}}'", "yumrepo"); !strings.Contains(out, "true") {
		run(t, "docker", "run", "-d", "-p", fmt.Sprintf("%d:80", repositoryPort), "--restart", "always", "--name", "yumrepo", "weaveworks/local-yum-repo:master-7b063513")
	}
	yumRepoIP := sanitizeIP(run(t, "docker", "inspect", "yumrepo", "--format='{{.NetworkSettings.IPAddress}}'"))

	// Start the footloose container "VMs" used for testing:
	run(t, "footloose", "create", "-c", "../../../examples/footloose/ubuntu1804/docker/multimaster.yaml")
	node0IP := sanitizeIP(run(t, "docker", "inspect", "ubuntu-multimaster-node0", "--format='{{.NetworkSettings.IPAddress}}'"))
	node1IP := sanitizeIP(run(t, "docker", "inspect", "ubuntu-multimaster-node1", "--format='{{.NetworkSettings.IPAddress}}'"))
	node2IP := sanitizeIP(run(t, "docker", "inspect", "ubuntu-multimaster-node2", "--format='{{.NetworkSettings.IPAddress}}'"))
	node3IP := sanitizeIP(run(t, "docker", "inspect", "ubuntu-multimaster-node3", "--format='{{.NetworkSettings.IPAddress}}'"))

	dirName := tempDir(t)
	clusterYAML := saveToFile(t, dirName, "cluster.yaml", fmt.Sprintf(clusterYAMLUbuntu, registryIP, registryPort))
	machinesYAML := saveToFile(t, dirName, "machines.yaml", fmt.Sprintf(machinesYAML, node0IP, node1IP, node2IP, node3IP))
	_ = saveToFile(t, dirName, "repo-config.yaml", fmt.Sprintf(repoConfigMapUbuntu, yumRepoIP))
	_ = saveToFile(t, dirName, "docker-config.yaml", fmt.Sprintf(dockerConfigMap, registryIP, registryPort))

	runWithStdout(t, "../../../cmd/wksctl/wksctl", "apply",
		fmt.Sprintf("--cluster=%s", clusterYAML), fmt.Sprintf("--machines=%s", machinesYAML),
		fmt.Sprintf("--config-directory=%s", dirName),
		"--verbose",
		fmt.Sprintf("--controller-image=docker.io/weaveworks/wksctl-controller:%s", tag))

	out := run(t, "../../../cmd/wksctl/wksctl", "kubeconfig",
		fmt.Sprintf("--cluster=%s", clusterYAML), fmt.Sprintf("--machines=%s", machinesYAML))

	var nodeList corev1.NodeList
	for {
		jsonOut := runIgnoreError(t, "kubectl", "get", "nodes", "-o", "json", fmt.Sprintf("--kubeconfig=%s", kubeconfig(out)))
		if err := json.Unmarshal([]byte(jsonOut), &nodeList); err != nil {
			log.Warnf("Error deserialising output of kubectl get nodes: %s", err)
		}
		log.Infof("The cluster currently has %d node(s)", len(nodeList.Items))
		if len(nodeList.Items) == 4 {
			break
		}
		time.Sleep(30 * time.Second)
	}

	assert.Len(t, nodeList.Items, 4)
	assert.Len(t, nodes.Masters(nodeList).Items, 3)
	assert.Len(t, nodes.Workers(nodeList).Items, 1)

	expectedKubeletArgs := []string{"alsologtostderr=true", "container-runtime=docker"}
	expectedApiServerArgs := []string{"alsologtostderr=true", "audit-log-maxsize=10000"}

	for i := 0; i < 4; i++ {
		for _, kubeletArg := range expectedKubeletArgs {
			log.Infof("Checking kubelet arg (%s) on node%d", kubeletArg, i)
			run(t, "footloose",
				"-c", "../../../examples/footloose/ubuntu1804/docker/multimaster.yaml",
				"ssh", fmt.Sprintf("root@node%d", i), fmt.Sprintf("ps -ef | grep -v 'ps -ef' | grep /usr/bin/kubelet | grep %s", kubeletArg))
		}

		// node0 - node2 are masters
		if i < 3 {
			for _, apiServerArg := range expectedApiServerArgs {
				log.Infof("Checking api server arg (%s) on node%d", apiServerArg, i)
				run(t, "footloose",
					"-c", "../../../examples/footloose/ubuntu1804/docker/multimaster.yaml",
					"ssh", fmt.Sprintf("root@node%d", i), fmt.Sprintf("ps -ef | grep -v 'ps -ef' | grep kube-apiserver | grep %s", apiServerArg))
			}
		}
	}

	if !t.Failed() { // Otherwise leave the footloose "VMs" & config files around for debugging purposes.
		// Clean up:
		defer run(t, "footloose", "delete", "-c", "../../../examples/footloose/ubuntu1804/docker/multimaster.yaml")
		defer os.Remove(dirName)
		defer os.Remove(clusterYAML)
		defer os.Remove(machinesYAML)
		defer os.Remove(repoConfigMap)
		defer os.Remove(repoConfigMap)
	}
}

func runWithStdout(t *testing.T, name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func imageTag(t *testing.T) string {
	tag, tagIsPresent := os.LookupEnv("IMAGE_TAG")
	if !tagIsPresent {
		log.Debug("no tag provided via the IMAGE_TAG env. var., now running tools/image-tag")
		tag = run(t, "../../../tools/image-tag")
	}
	tag = strings.TrimSpace(tag)
	assert.NotEmpty(t, tag)
	return tag
}

func port(t *testing.T, name string, defaultValue int) int {
	port, portIsPresent := os.LookupEnv(name)
	if !portIsPresent {
		return defaultValue
	}
	value, err := strconv.Atoi(port)
	assert.NoError(t, err)
	return value
}

func run(t *testing.T, name string, arg ...string) string {
	return doRun(t, false, name, arg...)
}

func runIgnoreError(t *testing.T, name string, arg ...string) string {
	return doRun(t, true, name, arg...)
}

func doRun(t *testing.T, ignoreError bool, name string, arg ...string) string {
	log.Infof("running %s %s", name, arg)
	cmd := exec.Command(name, arg...)
	out, err := cmd.Output()
	if err != nil && !ignoreError {
		stderr := err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		require.NoError(t, err, "Command %s failed. STDOUT: %s\nSTDERR: %s", cmd.Path, string(out), stderr)
	}
	return string(out)
}

func sanitizeIP(ip string) string {
	return strings.Replace(strings.TrimSpace(ip), "'", "", -1)
}

func waitForLocalRegistryToStart(t *testing.T, port int) {
	for {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/v2/", port))
		if err != nil {
			time.Sleep(1 * time.Second) // Container may still be starting and may return corrupted responses.
			continue
		}
		if resp.StatusCode == 200 {
			break
		}
	}
}

func shouldRetagPush(t *testing.T, port int) bool {
	images := run(t, "../../../environments/local-docker-registry/retag_push.sh", "-p", strconv.Itoa(port), "--print-local")
	for _, image := range strings.Split(images, "\n") {
		if image == "" {
			continue
		}
		image = strings.Replace(image, fmt.Sprintf("localhost:%d/", port), "", -1)
		parts := strings.Split(image, ":") // Separate image from tag.
		assert.Len(t, parts, 2)
		image := parts[0]
		tag := parts[1]
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/v2/%s/manifests/%s", port, image, tag))
		assert.NoError(t, err)
		if resp.StatusCode == 404 {
			return true
		}
	}
	return false
}

func tempDir(t *testing.T) string {
	dirName, err := ioutil.TempDir("", "multimaster_test_")
	assert.NoError(t, err)
	return dirName
}

func saveToFile(t *testing.T, dirName, fileName, content string) string {
	filePath := filepath.Join(dirName, fileName)
	err := ioutil.WriteFile(filePath, []byte(content), 0644)
	assert.NoError(t, err)
	return filePath
}

func kubeconfig(out string) string {
	kubeconfig := "KUBECONFIG="
	for _, line := range strings.Split(out, "\n") {
		if idx := strings.Index(line, kubeconfig); idx != -1 {
			return line[idx+len(kubeconfig):]
		}
	}
	return ""
}
