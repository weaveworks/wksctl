# Cluster Definition

Here's an example of the cluster definition.

```
apiVersion: cluster.k8s.io/v1alpha1
kind: Cluster
metadata:
  name: example
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
      sshKeyPath: cluster-key
      user: root
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
      cri:
        kind: docker
        package: docker-ce
        version: 18.09.7
      apiServer:
        extraArguments:
        - name: alsologtostderr
          value: "true"
        - name: audit-log-maxsize
          value: "10000"
      kubeletArguments:
      - name: alsologtostderr
        value: "true"
      - name: container-runtime
        value: docker
```

## Passing extra arguments to the API Server

`spec.providerSpec.value.apiServer.extraArguments` is where we specify extra arguments for the API servers. Each pair of `name` and `value` is used to form an argument as `name=value`. From the example, we'll have `alsologtostderr=true` and `audit-log-maxsize=10000` as extra arguments for every API server.

## Passing extra arguments to Kubelet

`spec.providerSpec.value.kubeletArguments` is the place to specify extra arguments for Kubelet. From the above example, we'll have `alsologtostderr=true` and `container-runtime=docker` as extra arguments.

