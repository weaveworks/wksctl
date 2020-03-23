# Get started with `wksctl`

Using `wksctl` you have two modes of operation. **Standalone** mode and **GitOps** mode. The latter will enable you to keep the state of the cluster itself in Git too.

## Modes of use

In **standalone mode**, `wksctl` builds a static cluster based on the contents of `cluster.yaml` and `machines.yaml` files passed on the command line; in **GitOps mode**, changes to `cluster.yaml` and `machines.yaml` files stored in Git will cause updates to the state of the live cluster.

### Standalone mode

Run `wksctl apply` and pass in the paths to `cluster.yaml` and `machines.yaml`

```console
wksctl apply \
  --cluster cluster.yaml \
  --machines machines.yaml
```

### GitOps mode

We will create a cluster by pulling the cluster and machine yaml from git.

The following are commandline arguments to `wksctl apply` which will result in a cluster being created.

- **git-url** The git repo url containing the `cluster.yaml` and `machine.yaml`
- **git-branch**  The branch within the repo to pull the cluster info from
- **git-deploy-key** The deploy key configured for the GitHub repo
- **git-path** Relative path to files in Git (optional)

The git command line arguments are passed instead of `--cluster` and `--machines`.

```console
wksctl apply \
  --git-url git@github.com:$YOUR_GITHUB_ORG/config-repo.git \
  --git-branch dev \
  --git-deploy-key ./deploy-key
```

Using the url, branch, and deploy key, `wksctl` will clone the repo and create the cluster.

These `--git` arguments are then used to set up and configure [flux](https://www.weave.works/oss/flux/) to automate cluster management via Git aka [GitOps](https://www.weave.works/technologies/gitops/)

We will rely on the user installing [fluxctl](https://docs.fluxcd.io/en/latest/references/fluxctl.html#installing-fluxctl) to interact with flux directly.  `wksctl` does not replicate this functionality.

```console
### wksctl apply 
A complete description of the apply command

wksctl apply --help
Create or update a Kubernetes cluster

Usage:
  wksctl apply [flags]

Flags:
      --cluster string              Location of cluster manifest (default "cluster.yaml")
      --config-directory string     Directory containing configuration information for the cluster (default ".")
      --git-branch string           Git branch WKS should use to sync with your cluster (default "master")
      --git-deploy-key string       Path to the Git deploy key
      --git-path string             Relative path to files in Git (default ".")
      --git-url string              Git repo containing your cluster and machine information
  -h, --help                        help for apply
      --machines string             Location of machines manifest (default "machines.yaml")
      --namespace string            namespace override for WKS components (default "weavek8sops")
      --sealed-secret-cert string   Path to a certificate used to encrypt sealed secrets
      --sealed-secret-key string    Path to a key used to decrypt sealed secrets
      --ssh-key string              Path to a key authorized to log in to machines by SSH (default "./cluster-key")
      --use-manifest-namespace      use namespaces from supplied manifests (overriding any --namespace argument)
```
