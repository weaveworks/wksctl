package init

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const fluxInputs = `
apiVersion: v1
items:
- apiVersion: v1
  kind: Namespace
  metadata:
    name: weavek8sops
  spec: {}
  status: {}
- apiVersion: v1
  kind: ServiceAccount
  metadata:
    labels:
      name: flux
    name: flux
    namespace: weavek8sops
- apiVersion: apps/v1beta1
  kind: Deployment
  metadata:
    name: flux
    namespace: weavek8sops
  spec:
    replicas: 1
    selector:
      matchLabels:
        name: flux
    strategy:
      type: Recreate
    template:
      metadata:
        annotations:
          prometheus.io.port: "3031"
        labels:
          name: flux
      spec:
        containers:
        - args:
          - --ssh-keygen-dir=/var/fluxd/keygen
          - --git-url=git@github.com:weaveworks/wkp-test.git
          - --git-branch=master
          - --git-poll-interval=30s
          - --git-path="."
          - --memcached-hostname=memcached.weavek8sops.svc.cluster.local
          - --memcached-service=memcached
          - --listen-metrics=:3031
          - --sync-garbage-collection
          image: fluxcd/flux:1.13.3
          imagePullPolicy: IfNotPresent
          name: flux
          ports:
          - containerPort: 3030
          resources: {}
          volumeMounts:
          - mountPath: /etc/fluxd/ssh
            name: git-key
            readOnly: true
          - mountPath: /var/fluxd/keygen
            name: git-keygen
        serviceAccount: flux
        tolerations:
        - effect: NoSchedule
          key: node-role.kubernetes.io/master
          operator: Exists
        - key: CriticalAddonsOnly
          operator: Exists
        volumes:
        - name: git-key
          secret:
            defaultMode: 256
            secretName: flux-git-deploy
        - emptyDir:
            medium: Memory
          name: git-keygen
  status: {}
kind: List
metadata: {}
`

const fluxOutputs = `
apiVersion: v1
items:
- apiVersion: v1
  kind: Namespace
  metadata:
    name: blonskar
  spec: {}
  status: {}
- apiVersion: v1
  kind: ServiceAccount
  metadata:
    labels:
      name: flux
    name: flux
    namespace: blonskar
- apiVersion: apps/v1beta1
  kind: Deployment
  metadata:
    name: flux
    namespace: blonskar
  spec:
    replicas: 1
    selector:
      matchLabels:
        name: flux
    strategy:
      type: Recreate
    template:
      metadata:
        annotations:
          prometheus.io.port: "3031"
        labels:
          name: flux
      spec:
        containers:
        - args:
          - --ssh-keygen-dir=/var/fluxd/keygen
          - --git-url=git@github.com:weaveworks/foo.bar
          - --git-branch=rickey
          - --git-poll-interval=30s
          - --git-path=eightfold
          - --memcached-hostname=memcached.weavek8sops.svc.cluster.local
          - --memcached-service=memcached
          - --listen-metrics=:3031
          - --sync-garbage-collection
          image: fluxcd/flux:1.13.3
          imagePullPolicy: IfNotPresent
          name: flux
          ports:
          - containerPort: 3030
          resources: {}
          volumeMounts:
          - mountPath: /etc/fluxd/ssh
            name: git-key
            readOnly: true
          - mountPath: /var/fluxd/keygen
            name: git-keygen
        serviceAccount: flux
        tolerations:
        - effect: NoSchedule
          key: node-role.kubernetes.io/master
          operator: Exists
        - key: CriticalAddonsOnly
          operator: Exists
        volumes:
        - name: git-key
          secret:
            defaultMode: 256
            secretName: flux-git-deploy
        - emptyDir:
            medium: Memory
          name: git-keygen
  status: {}
kind: List
metadata: {}
`

func TestFluxTranslate(t *testing.T) {
	res, err := updateFluxManifests([]byte(fluxInputs),
		initOptionType{
			namespace: "blonskar",
			gitURL:    "git@github.com:weaveworks/foo.bar",
			gitBranch: "rickey",
			gitPath:   "eightfold",
		})
	assert.NoError(t, err)
	assert.Equal(t, string(res), fluxOutputs)
}

const controllerInputs = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: wks-controller
  namespace: system
  labels:
    name: wks-controller
    control-plane: wks-controller
    controller-tools.k8s.io: "1.0"
spec:
  replicas: 1
  selector:
    matchLabels:
      name: wks-controller
  template:
    metadata:
      labels:
        name: wks-controller
        control-plane: wks-controller
        controller-tools.k8s.io: "1.0"
    spec:
      nodeSelector:
        node-role.kubernetes.io/master: ""
      tolerations:
      # Allow scheduling on master nodes. This is required because during
      # bootstrapping of the cluster, we may initially have just one master,
      # and would then need to deploy this controller there to set the entire
      # cluster up.
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
        operator: Exists
      # Mark this as a critical addon:
      - key: CriticalAddonsOnly
        operator: Exists
      # Only schedule on nodes which are ready and reachable:
      - effect: NoExecute
        key: node.alpha.kubernetes.io/notReady
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/unreachable
        operator: Exists
      containers:
      - name: controller
        image: quay.io/wksctl/controller:latest
        command:
        - /bin/controller
        - --verbose
        resources:
          limits:
            cpu: 100m
            memory: 30Mi
          requests:
            cpu: 100m
            memory: 20Mi
`
const controllerOutputs = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: wks-controller
  namespace: system
  labels:
    name: wks-controller
    control-plane: wks-controller
    controller-tools.k8s.io: "1.0"
spec:
  replicas: 1
  selector:
    matchLabels:
      name: wks-controller
  template:
    metadata:
      labels:
        name: wks-controller
        control-plane: wks-controller
        controller-tools.k8s.io: "1.0"
    spec:
      nodeSelector:
        node-role.kubernetes.io/master: ""
      tolerations:
      # Allow scheduling on master nodes. This is required because during
      # bootstrapping of the cluster, we may initially have just one master,
      # and would then need to deploy this controller there to set the entire
      # cluster up.
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
        operator: Exists
      # Mark this as a critical addon:
      - key: CriticalAddonsOnly
        operator: Exists
      # Only schedule on nodes which are ready and reachable:
      - effect: NoExecute
        key: node.alpha.kubernetes.io/notReady
        operator: Exists
      - effect: NoExecute
        key: node.alpha.kubernetes.io/unreachable
        operator: Exists
      containers:
      - name: controller
        env:
        - name: FOOTLOOSE_SERVER_ADDR
          value: 192.168.51.1:28496
        - name: FOOTLOOSE_BACKEND
          value: docker
        image: quay.io/wksctl/controller:version1.2.3
        command:
        - /bin/controller
        - --verbose
        resources:
          limits:
            cpu: 100m
            memory: 30Mi
          requests:
            cpu: 100m
            memory: 20Mi
`

func TestControllerTranslate(t *testing.T) {
	res, err := updateControllerManifests([]byte(controllerInputs),
		initOptionType{
			version:          "version1.2.3",
			footlooseIP:      "192.168.51.1:28496",
			footlooseBackend: "docker",
		})
	assert.NoError(t, err)
	assert.Equal(t, string(res), controllerOutputs)
}
