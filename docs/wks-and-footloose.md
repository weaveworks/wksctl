# WKS and Footloose

1. Pick a distro of your choice:

   1. `centos7` (At the moment of writing, `centos7` is more mature)
   2. `ubuntu1804`

   ```console
   export DISTRO=centos7
   ```

1. Pick a backend of your choice:

   1. `docker` (not real VMs, but can be used on Mac)
   2. `ignite` (requires [Ignite](https://ignite.readthedocs.org) to be installed, and KVM functioning)

   ```console
   export BACKEND=docker
   ```

1. Install [`footloose`](https://github.com/weaveworks/footloose):

   ```console
   GO111MODULE=on go install github.com/weaveworks/footloose
   ```

1. Start two machines using `footloose`:

   ```console
   $ footloose create -c ${DISTRO}/${BACKEND}/singlemaster.yaml
   INFO[0000] Image: quay.io/footloose/centos7 present locally
   INFO[0000] Creating machine: cluster-node0 ...
   INFO[0001] Creating machine: cluster-node1 ...
   ```

   You should now see the Container Machines running with `docker ps` or `ignite ps` (depending on your backend):

   ```console
   $ docker ps
   CONTAINER ID        IMAGE                     COMMAND             CREATED             STATUS              PORTS                                          NAMES
   ab4f4b75f63d        quay.io/wksctl/vm-centos7 "/sbin/init"        5 seconds ago         Up 4 seconds          0.0.0.0:2223->22/tcp, 0.0.0.0:6444->6443/tcp   cluster-node1
   0ce280129e79        quay.io/wksctl/vm-centos7 "/sbin/init"        6 seconds ago         Up 5 seconds          0.0.0.0:6443->6443/tcp, 0.0.0.0:2222->22/tcp   cluster-node0
   ```

   or:

   ```console
   $ ignite ps
   VM ID			IMAGE				KERNEL					SIZE	CPUS	MEMORY		CREATED	STATUS	IPS		PORTS						NAME
   3fbe4611682b3e16	weaveworks/ignite-centos:latest	weaveworks/ignite-kernel:4.19.47	4.0 GB	2	1024.0 MB	10m ago	Up 10m	172.17.0.3	0.0.0.0:30444->30443/tcp, 0.0.0.0:30081->30080/tcp, 0.0.0.0:2223->22/tcp, 0.0.0.0:6444->6443/tcp	centos-singlemaster-node1
   b4fdde36eb122804	weaveworks/ignite-centos:latest	weaveworks/ignite-kernel:4.19.47	4.0 GB	2	1024.0 MB	10m ago	Up 10m	172.17.0.2	0.0.0.0:2222->22/tcp, 0.0.0.0:6443->6443/tcp, 0.0.0.0:30443->30443/tcp, 0.0.0.0:30080->30080/tcp	centos-singlemaster-node0
   ```

   In case you would like to ssh into a machine e.g. `node0`, run:

   ```console
   footloose ssh -c ${DISTRO}/${BACKEND}/singlemaster.yaml root@node0
   ```

   as the default user name is `root` for both backends.

1. Run `wksctl apply`:

   ```console
   wksctl apply \
    --machines=machines.yaml \
    --cluster=cluster.yaml \
    --verbose
   ```

1. Run `wksctl kubeconfig` to be able to connect to the cluster:

   ```console
   $ wksctl kubeconfig --cluster=cluster.yaml
   To use kubectl with the example cluster, enter:
   export KUBECONFIG=$HOME/.wks/weavek8sops/example/kubeconfig
   $ export KUBECONFIG=/home/lucas/.wks/weavek8sops/example/kubeconfig
   $ kubectl get nodes
   NAME               STATUS   ROLES    AGE   VERSION
   b4fdde36eb122804   Ready    master   77s   v1.14.1
   $ kubectl get pods --all-namespaces
   NAMESPACE     NAME                              READY   STATUS    RESTARTS   AGE
   kube-system   coredns-86c58d9df4-26gv9          1/1     Running   0          55s
   kube-system   coredns-86c58d9df4-mb4h9          1/1     Running   0          55s
   kube-system   etcd-13e2dc14bf30                 1/1     Running   0          6s
   kube-system   kube-apiserver-13e2dc14bf30       1/1     Running   0          8s
   kube-system   kube-proxy-l2fv7                  1/1     Running   0          55s
   kube-system   kube-scheduler-13e2dc14bf30       1/1     Running   0          9s
   kube-system   weave-net-n7lqb                   2/2     Running   0          55s
   system        wks-controller-654d7cfb7c-47f9g   1/1     Running   0          54s
   ```

## Multi-masters

Follow the above steps, but pass the multi-master manifests:

```console
footloose create -c ${DISTRO}/${BACKEND}/multimaster.yaml
```

```console
$ wksctl apply \
    --machines=machines-multimaster.yaml \
    --cluster=cluster.yaml \
    [...]
```
