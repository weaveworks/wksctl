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

The following are new commandline arguments to `wksctl apply` which will result in a cluster being created.

- **git-url** The git repo url containing the `cluster.yaml` and `machine.yaml`
- **git-branch**  The branch within the repo to pull the cluster info from
- **git-deploy-key** The deploy key configured for the GitHub repo
- **git-path** Relative path to files in Git (optional)

The git command line arguments will be passed instead of `--cluster` and `--machines`.

```console
wksctl apply \
  --git-url git@github.com:$YOUR_GITHUB_ORG/config-repo.git \
  --git-branch dev \
  --git-deloy-key-path ./deploy-key
```

Using the url, branch, and deploy key, we will clone the repo - if we can't clone the repo we will error out.

These `--git` arguments are then used to set up and configure [flux](https://www.weave.works/oss/flux/) to automate cluster management.

We will rely on the user installing [fluxctl](https://docs.fluxcd.io/en/latest/references/fluxctl.html#installing-fluxctl) to interact with flux directly instead of trying to replicate the functionality within `wksctl`

To see a more detailed example combining Wksctl, [GitOps](https://www.weave.works/technologies/gitops/), [Ignite](https://ignite.readthedocs.io/en/stable/) also know as FireKube see [Firekube](examples/footloose/README.md#firekube-gitops)
