# Weave Kubernetes Subscription Control - `wksctl`

The `wksctl` command allows simple creation of a Kubernetes cluster given a **set of IP addresses** and an **SSH key**. It can be run in a standalone environment but is best used via a [GitOps approach](https://www.weave.works/technologies/gitops/) in which cluster and machine descriptions are stored in Git and the state of the cluster tracks changes to the descriptions.

## Install wksctl binary 

1. Download the OS specific `wksctl` release package from the [release page](https://github.com/weaveworks/wksctl/releases)
1. Unpack and add the `wksctl` binary to your path

For example:
```
cd <download dir>
tar xf wksctl-0.7.0-linux-x86_64.tar.gz
chmod +x wksctl
sudo mv wksctl /usr/local/bin/
```

## Quick start

The [examples](examples) folder (also included in the release package download) contains guides to get you up and running with [Footloose](https://github.com/weaveworks/footloose), [Vagrant](https://www.vagrantup.com) and others!

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
  --git-url git@github.com:meseeks/config-repo.git \
  --git-branch dev \
  --git-deloy-key-path ./deploy-key
```
Using the url, branch, and deploy key, we will clone the repo - if we can't clone the repo we will error out.

These `--git` arguments are then used to set up and configure [flux](https://www.weave.works/oss/flux/) to automate cluster management.

We will rely on the user installing [fluxctl](https://github.com/weaveworks/flux/blob/master/site/fluxctl.md) to interact with flux directly instead of trying to replicate the functionality within `wksctl`

To see a more detailed example combining Wksctl, [GitOps](https://www.weave.works/technologies/gitops/), [Ignite](https://ignite.readthedocs.io/en/stable/) also know as FireKube see [Firekube](examples/footloose/README.md#firekube-gitops)

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
git checkout master ; git fetch origin master ; git merge --ff-only master
rm build/.uptodate
make !$
> [...]
> Successfully built deadbeefcafe
> Successfully tagged quay.io/wksctl/build:latest
> docker tag quay.io/wks/build quay.io/wksctl/build:master-XXXXXXX
> touch build/.uptodate
docker push quay.io/wksctl/build:$(tools/image-tag)
```

- Update `.circleci/config.yml` to use the newly pushed image.
- Push this change, get it reviewed, and merge it to `master`.
# Checkpoint

`wksctl` contacts Weaveworks servers for available versions. When a new version is available, `wksctl` will print out a message along with a URL to download it.

The information sent in this check is:
- wksctl version
- Machine Architecture 
- Operating System
- Host UUID hash

To disable this check, run the following before executing `wksctl`:
```console
export CHECKPOINT_DISABLE=1
```


## Contributing

Please see [CONTRIBUTING.md](CONTRIBUTING.md) and our [Code Of Conduct](CODE_OF_CONDUCT.md).

Other interesting resources include:

- [The issue tracker](https://github.com/weaveworks/wksctl/issues)

## Getting Help

If you have any questions about, feedback for or problems with `wksctl`:

- Invite yourself to the <a href="https://slack.weave.works/" target="_blank">Weave Users Slack</a>.
- Ask a question on the [#general](https://weave-community.slack.com/messages/general/) slack channel.
- [File an issue](https://github.com/weaveworks/wksctl/issues/new).

Your feedback is always welcome!

## License

[Apache 2.0](LICENSE.md)
