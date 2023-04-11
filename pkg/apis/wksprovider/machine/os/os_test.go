package os

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	capeios "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/apis/wksprovider/machine/os"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan"
	capeiresource "github.com/weaveworks/cluster-api-provider-existinginfra/pkg/plan/resource"
	"github.com/weaveworks/cluster-api-provider-existinginfra/pkg/utilities/object"
	appsv1 "k8s.io/api/apps/v1"
	v1beta2 "k8s.io/api/apps/v1beta2"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

func TestReplaceImage(t *testing.T) {
	var tests = []struct {
		yaml                  string
		newImage              string
		expInFileOutFileMatch bool
		expErr                bool
		msg                   string
	}{
		{"", "", true, false, "Expected files to match"},
		{`apiVersion: v1
kind: Secret`, "", true, false, "Exected files to match"},
		{`apiVersion: v1
kind: Deployment`, "newimage", false, true, "Expected err - no containers"},
		{`apiVersion: v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: foo
        image: fooimage`, "newimage", false, true, "Expected new file even though there isn't a Controller container"},
		{`apiVersion: v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: controller
        image: controllerimage`, "newimage", false, false, "Expected new file"},
		{`apiVersion: v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: controller
        image: controllerimage
      - name: foo
        image: controllerimage`, "newimage", false, false, "Expected new file"},
	}
	for _, test := range tests {
		in := []byte(test.yaml)
		out, err := capeios.UpdateControllerImage(in, test.newImage)
		if test.expErr {
			assert.NotNil(t, err, test.msg)
			assert.Empty(t, out, test.msg)
			continue
		}
		assert.Nil(t, err, test.msg)
		if test.expInFileOutFileMatch {
			assert.Equal(t, in, out, test.msg)
		} else {
			assert.NotEqual(t, in, out, test.msg)
		}
		if test.newImage == "" {
			continue
		}
		d := &v1beta2.Deployment{}
		assert.NoError(t, yaml.Unmarshal(out, d))
		if len(d.Spec.Template.Spec.Containers) == 0 {
			continue
		}
		for _, c := range d.Spec.Template.Spec.Containers {
			if c.Name == "controller" {
				assert.Equal(t, test.newImage, c.Image, test.msg)
			} else {
				assert.NotEqual(t, test.newImage, c.Image, test.msg)
			}
		}
	}
}

func TestFlux(t *testing.T) {
	var gitURL = "git@github.com/testorg/repo1"
	var gitBranch = "my-unique-prod-branch"
	dk := "the deploy key"
	f, err := ioutil.TempFile("", "")
	assert.NoError(t, err)
	_, err = f.WriteString(dk)
	assert.NoError(t, f.Close())
	assert.NoError(t, err)

	var gitDeployKeyPath = f.Name()
	var tests = []struct {
		URL, branch, deployKeyPath, notExp, msg string
	}{
		{"", "", "", "flux", "expected plan without flux"},
		{gitURL, "", "", "", "expected plan w/o branch or deploy key"},
		{gitURL, "", gitDeployKeyPath, "", "expected flux yaml with deploy key"},
		{gitURL, "", "", "", "expected flux yaml without deploy key"},
		{gitURL, gitBranch, "", "", "expected flux yaml with branch"},
		{gitURL, gitBranch, "", "", "expected to be in the system namespace"},
		{gitURL, gitBranch, "", "", "flux should not be the namespace"},
	}

	for _, test := range tests {
		b := plan.NewBuilder()
		applyClstrRsc := &capeiresource.KubectlApply{ManifestPath: object.String("")}
		b.AddResource("kubectl:apply:cluster", applyClstrRsc)
		applyMachinesRsc := &capeiresource.KubectlApply{ManifestPath: object.String("")}
		b.AddResource("kubectl:apply:machines", applyMachinesRsc)
		err = capeios.ConfigureFlux(b, capeios.SeedNodeParams{GitData: capeios.GitParams{GitURL: test.URL, GitBranch: test.branch, GitDeployKeyPath: test.deployKeyPath},
			Namespace: "system"})
		assert.NoError(t, err)
		p, err := b.Plan()
		assert.NoError(t, err)
		rjson := p.ToJSON()
		if test.URL == "" {
			assert.NotContains(t, rjson, test.notExp)
			continue
		}
		mani, err := p.State().GetObject("install:flux:main")
		assert.NoError(t, err)
		_, ok := mani["manifest"]
		assert.True(t, ok)
	}
}

// Test a manifest list without the weave-net daemonset
var wrongManifestList = `apiVersion: v1
kind: List
items:
- apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: weave-net
    namespace: kube-system
    labels:
      name: weave-net
  roleRef:
    kind: Role
    name: weave-net
    apiGroup: rbac.authorization.k8s.io
  subjects:
    - kind: ServiceAccount
      name: weave-net
      namespace: kube-system
- apiVersion: rbac.authorization.k8s.io/v1
  kind: ClusterRoleBinding
  metadata:
    name: weave-net
    labels:
      name: weave-net
  roleRef:
    kind: ClusterRole
    name: weave-net
    apiGroup: rbac.authorization.k8s.io
  subjects:
    - kind: ServiceAccount
      name: weave-net
      namespace: kube-system
`

// Test a valid manifest list with the weave-net daemonset
var validManifestList = `apiVersion: v1
kind: List
items:
- apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: weave-net
    namespace: kube-system
    labels:
      name: weave-net
  roleRef:
    kind: Role
    name: weave-net
    apiGroup: rbac.authorization.k8s.io
  subjects:
    - kind: ServiceAccount
      name: weave-net
      namespace: kube-system
- apiVersion: apps/v1
  kind: DaemonSet
  metadata:
    name: weave-net
    labels:
      name: weave-net
    namespace: kube-system
  spec:
    # Wait 5 seconds to let pod connect before rolling next pod
    selector:
      matchLabels:
        name: weave-net
    minReadySeconds: 5
    template:
      metadata:
        labels:
          name: weave-net
      spec:
        containers:
          - name: weave
            command:
              - /home/weave/launch.sh
            env:
              - name: HOSTNAME
                valueFrom:
                  fieldRef:
                    apiVersion: v1
                    fieldPath: spec.nodeName
            image: 'docker.io/weaveworks/weave-kube:2.7.0'
            imagePullPolicy: Always
            readinessProbe:
              httpGet:
                host: 127.0.0.1
                path: /status
                port: 6784
            resources:
              requests:
                cpu: 50m
            securityContext:
              privileged: true
            volumeMounts:
              - name: weavedb
                mountPath: /weavedb
              - name: cni-bin
                mountPath: /host/opt
              - name: cni-bin2
                mountPath: /host/home
              - name: cni-conf
                mountPath: /host/etc
              - name: dbus
                mountPath: /host/var/lib/dbus
              - name: lib-modules
                mountPath: /lib/modules
              - name: xtables-lock
                mountPath: /run/xtables.lock
                readOnly: false
          - name: weave-npc
            env:
              - name: HOSTNAME
                valueFrom:
                  fieldRef:
                    apiVersion: v1
                    fieldPath: spec.nodeName
            image: 'docker.io/weaveworks/weave-npc:2.7.0'
            imagePullPolicy: Always
            # npc-args
            resources:
              requests:
                cpu: 50m
            securityContext:
              privileged: true
            volumeMounts:
              - name: xtables-lock
                mountPath: /run/xtables.lock
                readOnly: false
        hostNetwork: true
        dnsPolicy: ClusterFirstWithHostNet
        hostPID: false
        restartPolicy: Always
        securityContext:
          seLinuxOptions: {}
        serviceAccountName: weave-net
        tolerations:
          - effect: NoSchedule
            operator: Exists
          - effect: NoExecute
            operator: Exists
        volumes:
          - name: weavedb
            hostPath:
              path: /var/lib/weave
          - name: cni-bin
            hostPath:
              path: /opt
          - name: cni-bin2
            hostPath:
              path: /home
          - name: cni-conf
            hostPath:
              path: /etc
          - name: dbus
            hostPath:
              path: /var/lib/dbus
          - name: lib-modules
            hostPath:
              path: /lib/modules
          - name: xtables-lock
            hostPath:
              path: /run/xtables.lock
              type: FileOrCreate
        priorityClassName: system-node-critical
    updateStrategy:
      type: RollingUpdate
- apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: weave-net
    namespace: kube-system
    labels:
      name: weave-net
  roleRef:
    kind: Role
    name: weave-net
    apiGroup: rbac.authorization.k8s.io
  subjects:
    - kind: ServiceAccount
      name: weave-net
      namespace: kube-system
`

var sampleDaemonSet = `
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: fluentd-elasticsearch
  namespace: kube-system
  labels:
    k8s-app: fluentd-logging
spec:
  selector:
    matchLabels:
      name: fluentd-elasticsearch
  template:
    metadata:
      labels:
        name: fluentd-elasticsearch
    spec:
      tolerations:
      # this toleration is to have the daemonset runnable on master nodes
      # remove it if your masters can't run pods
      - key: node-role.kubernetes.io/master
        effect: NoSchedule
      containers:
      - name: fluentd-elasticsearch
        image: quay.io/fluentd_elasticsearch/fluentd:v2.5.2
        resources:
          limits:
            memory: 200Mi
          requests:
            cpu: 100m
            memory: 200Mi
        volumeMounts:
        - name: varlog
          mountPath: /var/log
        - name: varlibdockercontainers
          mountPath: /var/lib/docker/containers
          readOnly: true
      terminationGracePeriodSeconds: 30
      volumes:
      - name: varlog
        hostPath:
          path: /var/log
      - name: varlibdockercontainers
        hostPath:
          path: /var/lib/docker/containers
`

func TestFindDaemonSet(t *testing.T) {
	// nil case
	_, _, err := capeios.FindDaemonSet(nil)
	assert.Error(t, err, "nil manifest list should fail")

	// empty case
	manifestList := &v1.List{}
	_, _, err = capeios.FindDaemonSet(manifestList)
	assert.Error(t, err, "empty manifest list should fail")

	// invalid manifest case
	err = yaml.Unmarshal([]byte(wrongManifestList), manifestList)
	assert.NoError(t, err)
	_, _, err = capeios.FindDaemonSet(manifestList)
	assert.Error(t, err, "manifest list without a daemonset should fail")

	// valid manifest case
	err = yaml.Unmarshal([]byte(validManifestList), manifestList)
	assert.NoError(t, err)
	idx, _, err := capeios.FindDaemonSet(manifestList)
	// the daemonSet is found at the 2nd position in the list
	assert.Equal(t, idx, 1)
	assert.NoError(t, err)
}

func TestInjectEnvVarToContainer(t *testing.T) {
	// get a daemonset first which has containers
	manifestList := &v1.List{}
	err := yaml.Unmarshal([]byte(validManifestList), manifestList)
	assert.NoError(t, err)
	_, daemonSet, err := capeios.FindDaemonSet(manifestList)
	assert.NoError(t, err)

	// inject a new env var
	ipallocRange := &v1.EnvVar{
		Name:  "IPALLOC_RANGE",
		Value: "10.96.0.0/16",
	}
	// nil case
	err = capeios.InjectEnvVarToContainer(nil, "", *ipallocRange)
	assert.Error(t, err, "nil case should return error")

	// valid case, env var should be contained in daemonset
	err = capeios.InjectEnvVarToContainer(daemonSet.Spec.Template.Spec.Containers, "weave", *ipallocRange)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(daemonSet.String(), "IPALLOC_RANGE"))

	// env var exists with different value
	ipallocRange = &v1.EnvVar{
		Name:  "IPALLOC_RANGE",
		Value: "172.20.0.0/23",
	}
	err = capeios.InjectEnvVarToContainer(daemonSet.Spec.Template.Spec.Containers, "weave", *ipallocRange)
	assert.Error(t, err, "env var exists with different value, should fail")

	// test with sample manifest with containers that don't include env
	daemonSet = &appsv1.DaemonSet{}
	err = yaml.Unmarshal([]byte(sampleDaemonSet), daemonSet)
	assert.NoError(t, err)

	err = capeios.InjectEnvVarToContainer(daemonSet.Spec.Template.Spec.Containers, "fluentd-elasticsearch", *ipallocRange)
	assert.NoError(t, err)
	assert.True(t, strings.Contains(daemonSet.String(), "IPALLOC_RANGE"))
}
