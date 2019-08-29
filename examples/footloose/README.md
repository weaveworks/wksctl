# Run Kubernetes inside containers

1. Pick a distro of your choice:

   1. `centos7` (At the moment of writing, `centos7` is more mature)
   2. `ubuntu1804`

  ```console
  $ export DISTRO=centos7
  ```

2. Pick a backend of your choice:
   1. `docker` (not real VMs, but can be used on Mac)
   2. `ignite` (requires [Ignite](https://ignite.readthedocs.org) to be installed, and KVM functioning)

  ```console
  $ export BACKEND=docker
  ```

3. Install [`footloose`](https://github.com/weaveworks/footloose):

```console
GO111MODULE=on go install github.com/weaveworks/footloose
```

4. Start two machines using `footloose`:

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

5. Run `wksctl apply`:

```console
$ wksctl apply \
    --machines=machines.yaml \
    --cluster=cluster.yaml \
    --verbose
```

6. Run `wksctl kubeconfig` to be able to connect to the cluster:

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
$ footloose create -c ${DISTRO}/${BACKEND}/multimaster.yaml
```

```console
$ wksctl apply \
    --machines=machines-multimaster.yaml \
    --cluster=cluster.yaml \
    [...]
```

## Firekube
Combining wksctl + footloose + ignite

1. Create a GitHub repo to hold your cluster confguration.
1. Copy `cluster.yaml machines.yaml docker-config.yaml repo-config.yaml` files into your repo
1. Add custom workloads e.g., Kubernetes Dashboard
1. Commit and push your changes to GitHub
1. Generate a deploy key, add it to GitHub with write permissions
1. Execute `wksctl apply --git-url your-repo --git-deploy-key keyfile`
1. Observe the pods starting and running in your cluster

```console
_YOUR_ORG_=foo
mkdir -p $HOME/src/firekube-sample
git init $HOME/src/firekube-sample
cp cluster.yaml machines.yaml docker-config.yaml repo-config.yaml $HOME/src/firekube-sample
echo "firekube-sample-deploykey*" > $HOME/src/firekube-sample/.gitignore
ssh-keygen -N "" -q -f $HOME/src/firekube-sample/firekube-sample-deploykey
cd $HOME/src/firekube-sample
git add -A
git commit -a -m "Firekube"
git remote add origin git@github.com:$_YOUR_ORG_/firekube-sample.git
git push -u origin master
# Add $HOME/src/firekube-sample/firekube-sample-deploykey.pub to the deploy keys for firekube-sample, or if you have hub installed run:
hub api --method POST /repos/$_YOUR_ORG_/firekube-sample/keys --field title=thekey --field key="$(cat $HOME/src/firekube-sample/firekube-sample-deploykey.pub)" --field readOnly=true
cd - 
footloose create -c centos7/ignite/singlemaster.yaml
wksctl apply -v --git-url git@github.com:$_YOUR_ORG_/firekube-sample.git --git-deploy-key $HOME/src/firekube-sample/firekube-sample-deploykey
wksctl kubeconfig
export KUBECONFIG=$HOME/.wks/weavek8sops/example/kubeconfig
kubectl get pods --all-namespaces
curl https://raw.githubusercontent.com/kubernetes/dashboard/v2.0.0-beta1/aio/deploy/recommended.yaml > $HOME/src/firekube-sample/kubernetes-dashboard.yaml
cd -
git add kubernetes-dashboard.yaml
git commit -m "adds k8s dash"
git push
kubectl proxy
open http://localhost:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/
```

## Cleanup

```console
$ footloose delete -c ${DISTRO}/${BACKEND}/singlemaster.yaml
$ footloose delete -c ${DISTRO}/${BACKEND}/multimaster.yaml
```

## Upload development controller images

When developing `wksctl` and using a binary compiled from a branch, `wksctl`
will try to apply the controller image tagged with `$(tools/image-tag)`.

That image only exists on the developer machine and needs to be made
available to the docker daemon running inside the first master `footloose`
node. Two options:

1. `docker push` the image to quay.io. This has the advantage of not needing
the docker daemon to run inside the `footloose` container and so models
better a brand new node where docker needs to be installed.

1. Upload the controller image onto the `footloose` container the footloose
seed master. This is slightly faster than 1. and can be done with:

```console
./upload-controller-image.sh
```
