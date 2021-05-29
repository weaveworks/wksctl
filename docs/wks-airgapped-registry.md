# WKS Airgapped Registry

### Use Case / User Story

The primary intended use of WKS for us was development of private / custom apps. Our development clusters required connecting to a proprietary database, on a VPN/VPC link configured to work only with completely private networks (not strictly air-gapped, but cloistered behind a NAT gateway and security groups, isolated from the public Internet.)

WKS can apparently run in an air-gap, as long as `cluster.yaml` mentions an image repository in the field `spec.providerSpec.value.imageRepository`, where images used by WKS are made available (see [environments/local-docker-registry/retag_push.sh](https://github.com/weaveworks/wksctl/blob/master/environments/local-docker-registry/retag_push.sh)). While a strict air gap was not needed in our environment, image registry hosting definitely was!

At first there was no support of any kind for containerized infrastructure, CI/CD, or other GitOps best-practices pre-reqs, so an expected workflow was for devs to use Docker to build images on a workstation, from upstream images or from scratch, and test them out locally before CI, without any server costs or using up bandwidth by pushing to Docker Hub.

GitOps was used to deploy locally in a way that was repeatable and disposable; keeping builds locally and staging on a cluster without pushing outside of the air-gap saves bandwidth and time as well as CPU cycles; this document describes how to perform testing of private workloads using [Firekube quickstart][] deployed using Footloose, on Docker for Mac.

## Docker Registry

WKS installs and configures Docker on the target footloose nodes, and since the rest of our network was already private and internal, for the proof-of-concept we decided there was no need for making plans to implement any complex encryption schemes or certificate infrastructure.

We hoped that our PoC could utilize a local registry to host our unpublished builds, where our manifests in Git repository could refer to them by image tags based on commit hashes (and later "SemVer tags", or Semantic Version tags). It is somewhat required in GitOps for images to be available from a stable location.

So, the image references (like the commits in the GitOps repo) should be stable relative to the Docker machine or Footloose cluster.

An insecure registry can be configured [as described here](https://github.com/weaveworks/wksctl/tree/master/environments/local-docker-registry).

First, an insecure registry is started on the Docker machine, in a container named `registry`. It starts with an address on the default bridge network:

```
$ docker run -d -p 5000:5000 --restart=always --name registry registry:2
```

Our footloose "VMs" (from Firekube quickstart) will also join this bridge by default, so while our Docker for Mac workstation needs a published port `-p 5000:5000` to reach the registry at `localhost:5000` in order to push images there from outside, the WKS cluster on the other hand can reach the Registry directly at the container's private IP address.

Obtain the registry IP:

```
$ docker inspect registry --format "{{.NetworkSettings.IPAddress}}"
```

A registry is listening on port 5000, in my case the container has IPAddress: `172.25.0.4`. Next, since no TLS termination will be provided, be sure that Kubernetes manifests and pod specs that attempt to pull images by referencing the new (insecure) Docker registry will succeed. (If your air-gapped environment provides a production registry that is terminated with TLS, then you can skip the following configuration for Insecure Registry.)

### Configuring Footloose for Insecure Registry

Footloose installs and configures Docker on the target nodes, and Docker default configuration requires all images pulled from external registries be secured by TLS. You can enable an insecure registry by adding the registry address to `insecure-registries`; in Firekube, this can be configured in `docker-config.yaml`:

```diff
@@ -10,6 +10,9 @@ data:
       "log-opts": {
         "max-size": "100m"
       },
+      "insecure-registries": [
+        "172.25.0.4:5000"
+      ],
       "exec-opts": [
         "native.cgroupdriver=cgroupfs"
       ]
```

This configuration change should be made before `./setup.sh` and before the cluster is created since it affects the Firekube nodes' Docker daemons.

When this configuration is changed, the developer with Docker access on the workstation can run:

```
$ docker build app-repo/ -t localhost:5000/app:tag
$ docker push localhost:5000/app:tag
```

to build and push an image into the private registry, and those images can then be used in pod definitions for workloads on the WKS cluster.

The pod spec can refer to the image on the registry as `172.25.0.4:5000/app:tag`; since that seemed to have met our needs at first, we considered that we were done! (Almost...)

### Addressing registry by name from within the Kubernetes cluster

Addressing the registry by IP in this configuration is not ideal; even though the container is outside of our WKS cluster, it is inside of a Docker machine, where there's no hard address stability guarantee. If containers are restarted in a different order, or if the Docker machine's bridge network configuration changes for some other reason, then the registry's IP address will likely change too. Now we have to admit, this method does not look very GitOps.

Given that Docker must be restarted in order to apply a new Docker config, you may find you have a chicken and egg problem where updating `daemon.json` and restarting the Docker Machine results in a new IP address for registry, again requiring an update to `daemon.json`. Then Firekube cluster also needed to be reconfigured and recreated, ...ad infinitum.

It also seems unwise to use an unstable IP address in GitOps deployment manifests that may need to be reused later; fortunately, there is a better way!

A Kubernetes `Service` can be created without any pod selectors, for referring to external services. It is backed by a corresponding (unmanaged) `Endpoints` object which can be updated directly. Images can now use a `Service` name and port, now we also mention it in the Firekube `docker-config.yaml` when setting `insecure-registries` as described before.

Create a `Service` and an `Endpoints` object with the IP address(es) where your registry can be reached.

```yaml
kind: Service
apiVersion: v1
metadata:
 name: registry
 namespace: default
spec:
 type: ClusterIP
 ports:
 - port: 5000
   targetPort: 5000
---
kind: Endpoints
apiVersion: v1
metadata:
 name: registry
 namespace: default
subsets:
 - addresses:
     - ip: 172.25.0.4
   ports:
     - port: 5000
```

```diff
@@ -10,6 +10,9 @@ data:
       "log-opts": {
         "max-size": "100m"
       },
+      "insecure-registries": [
+        "registry.default.svc:5000"
+      ],
       "exec-opts": [
         "native.cgroupdriver=cgroupfs"
       ]
```

If your air gapped environment includes DNS support and the registry IP is already discoverable via DNS, (whether your registry is insecure as in our case, or even if it is secured by TLS), you can also use an `ExternalName` service instead of creating `Endpoints`. This is equivalent to a CNAME. These examples were both adapted from the article [Kubernetes best practices: mapping external services](https://cloud.google.com/blog/products/gcp/kubernetes-best-practices-mapping-external-services) on the Google Cloud Blog.

```yaml
kind: Service
apiVersion: v1
metadata:
 name: registry
 namespace: default
spec:
 type: ExternalName
 externalName: my-registry.homelab.example.com
```

Now the registry service is decoupled from the "external" IP that backs it, and whenever the registry IP Address should change, it can be centrally updated in `Endpoints` or via DNS, without updating `docker-config.yaml` (👉 and without any need for restarting the footloose VMs, or rebuilding WKS from scratch... 🎉).

### Docker Desktop configuration

On the workstation, with the configuration as above, it may not be needed to change the Docker configuration in `daemon.json` – Although older versions did, `docker push` does not currently seem to require TLS or registry security when the registry is at `localhost:5000`. Images do not embed the hostname or port in the image tag name, so while your GitOps manifests can now refer to `registry.default.svc:5000/image-name:tag`, we can always still `docker push` images to `localhost:5000/image-name:tag` without reconfiguring Docker.

It may be desirable to push and pull from the exact same address interchangeably, both within the WKS cluster and from the Docker workstation (where the Docker bridge network [may not be routed](https://docs.docker.com/docker-for-mac/networking/#there-is-no-docker0-bridge-on-macos)).

If so, add `registry.default.svc` to `/etc/hosts` as an alias like:

```
127.0.0.1 localhost registry.default.svc
```

Then you can push and pull image references like `registry.default.svc:5000/image-name:tag` from in the WKS cluster or on the workstation outside of the Docker Machine. (It is not necessary to add any entry to insecure-registries in the host machine's Docker for Desktop `daemon.json` config, apparently, as long as the registry is hosted from localhost.)

For more information about using WKS in this configuration, see the [Firekube quickstart][]

[Firekube quickstart]: https://github.com/weaveworks/wks-quickstart-firekube
[Configuring a registry]: https://docs.docker.com/registry/configuration/
