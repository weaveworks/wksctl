# Weave Kubernetes Subscription Control (WKSCTL)

## Overview
The `wksctl` command allows simple creation of a Kubernetes cluster
given a set of IP addresses and an SSH key. It can be run in a
standalone environment but is best used via a GitOps approach in which
cluster and machine descriptions are stored in Git and the state of
the cluster tracks changes to the descriptions. In standalone mode,
`wksctl` builds a static cluster based on the contents of
`cluster.yaml` and `machines.yaml` files passed on the command line;
in GitOps mode, changes to `cluster.yaml` and `machines.yaml` files
stored in Git will cause updates to the state of the live cluster.

## Notes

### code-generator

```console
make gen
```

## Development

### Build

```console
make
```

#### Upgrading the build image

- Update `build/Dockerfile` as required.
- Test the build locally:

```console
rm build/.uptodate
make !$
```

- Push this change, get it reviewed, and merge it to `master`.
- Run:

```console
$ git checkout master ; git fetch origin master ; git merge --ff-only master
$ rm build/.uptodate
$ make !$
[...]
Successfully built deadbeefcafe
Successfully tagged quay.io/wksctl/build:latest
docker tag quay.io/wks/build quay.io/wksctl/build:master-XXXXXXX
touch build/.uptodate
$ docker push quay.io/wksctl/build:$(tools/image-tag)
```

- Update `.circleci/config.yml` to use the newly pushed image.
- Push this change, get it reviewed, and merge it to `master`.

# Using with a config repo instead of cluster and machine yaml files
We will create a cluster by pulling the cluster and machine yaml from git.

The following are new commandline arguments to `wksctl apply` which will result in a cluster being created.

- **git-url** The git repo url containing the cluster and machine yaml
- **git-branch**  The branch within the repo to pull the cluster info from
- **git-deploy-key** The deploy key configured for the GitHub repo
- **git-path** Relative path to files in Git (optional)

The git command line arguments will be passed instead of --cluster and --machines.

```console
$ wksctl apply
  --git-url git@github.com:meseeks/config-repo.git \
  --git-branch dev \
  --git-deloy-key-path ./deploy-key
```
Using the url, branch, and deploy key, we will clone the repo - if we can't clone the repo we will error out.

These `--git` arguments are then used to set up and configure [flux](https://www.weave.works/oss/flux/) to automate cluster management.

We will rely on the user installing [fluxctl](https://github.com/weaveworks/flux/blob/master/site/fluxctl.md) to interact with flux directly instead of trying to replicate the functionality within `wksctl`

To see a more detailed example combining Wksctl, [GitOps](https://www.weave.works/technologies/gitops/), [Ignite](https://ignite.readthedocs.io/en/stable/) also know as FireKube see [Firekube](examples/footloose/README.md#firekube-gitops)
