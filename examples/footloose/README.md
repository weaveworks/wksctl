# Run Kubernetes inside containers

1. Pick a `DISTRO` of your choice: `centos7` or `ubuntu1804`. At the moment of writing, `centos7` is more mature. Replace `<DISTRO>` in the command lines below with your choice.

2. Install [`footloose`](https://github.com/weaveworks/footloose):

```console
GO111MODULE=on go install github.com/weaveworks/footloose
```

3. Start two "VMs":

```console
$ footloose create -c <DISTRO>/singlemaster.yaml
INFO[0000] Image: quay.io/footloose/centos7 present locally
INFO[0000] Creating machine: cluster-node0 ...
INFO[0001] Creating machine: cluster-node1 ...

$ docker ps
CONTAINER ID        IMAGE                     COMMAND             CREATED             STATUS              PORTS                                          NAMES
ab4f4b75f63d        quay.io/wksctl/vm-centos7 "/sbin/init"        5 seconds ago         Up 4 seconds          0.0.0.0:2223->22/tcp, 0.0.0.0:6444->6443/tcp   cluster-node1
0ce280129e79        quay.io/wksctl/vm-centos7 "/sbin/init"        6 seconds ago         Up 5 seconds          0.0.0.0:6443->6443/tcp, 0.0.0.0:2222->22/tcp   cluster-node0
```

4. Run `apply`:

```console
$ wksctl apply \
    --machines=machines.yaml \
    --cluster=cluster.yaml \
    --verbose
```

5. \o/

```console
$ footloose ssh -c <DISTRO>/singlemaster.yaml root@node0
[root@node0 ~]# kubectl get pods --all-namespaces
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
footloose create -c <DISTRO>/multimaster.yaml
```

```console
$ wksctl apply \
    --machines=machines-multimaster.yaml \
    --cluster=cluster.yaml \
    [...]
```

```console
footloose delete -c <DISTRO>/multimaster.yaml
```

## To use kubectl to point to your new cluster

```console
$ wksctl kubeconfig --cluster=cluster.yaml
```

## Accessing UI applications

The default configuration for footloose exposes two ports for ingress services - http at 30080 and https at 30443.  To expose a WebUI on these ports, update your cluster yaml to include the ingress-nginx addon.  For example:

```yaml
      addons:
      - name: ingress-nginx
        deps: ["https://raw.githubusercontent.com/kubernetes/ingress-nginx/master/deploy/mandatory.yaml"]
        params:
          type: "NodePort"
          httpPort: "30080"
          httpsPort: "30443"
```

If you want to expose difference ports, update the footloose configuration file and the addon.

Next, you will need to define an ingress object to front your web app.  Here is an example for the yipee.io app:

```yaml
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: yipee-ingress
  namespace: yipee
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
  - http:
      paths:
      - path: /
        backend:
          serviceName: yipee-ui
          servicePort: 80
```

Once the cluster is up and your application is deployed you will be able to access the application at https://localhost:30443

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
