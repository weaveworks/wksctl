# Weave Kubernetes System Control - `wksctl`

`wksctl` allows simple creation of a Kubernetes cluster given a **set of IP addresses** and an **SSH key**. It can be run in a standalone environment but is best used via a [GitOps approach](https://www.weave.works/technologies/gitops/) in which cluster and machine descriptions are stored in Git and the state of the cluster tracks changes to the descriptions.

Its features include:

- simple creation of Kubernetes clusters
- manage cluster and machine descriptions using Git
- manage addons like Weave Net or Flux
- Sealed Secret integration

## Install wksctl binary

1. Download the OS specific `wksctl` release package from the [release page](https://github.com/weaveworks/wksctl/releases)
1. Unpack and add the `wksctl` binary to your path

For example:

```console
cd <download dir>
tar xfz wksctl-0.7.0-linux-x86_64.tar.gz
chmod +x wksctl
sudo mv wksctl /usr/local/bin/
```

Check out [our Get Started doc](https://wksctl.readthedocs.io/en/latest/get-started.html) to dive deeper into the different ways to operate `wksctl`.

## Quick start

We put together a couple of guides to get you up and running with WKS in combination with [Footloose](https://github.com/weaveworks/footloose), [Vagrant](https://www.vagrantup.com) and others!

- [Firekube](https://github.com/weaveworks/wks-quickstart-firekube) - use our WKS + Footloose + Ignite demo and be up and running in no time
- [WKS and Footloose](https://wksctl.readthedocs.io/en/latest/wks-and-footloose.html)
- [WKS and Vagrant](https://wksctl.readthedocs.io/en/latest/wks-and-vagrant.html)
- [WKS on GCE](https://wksctl.readthedocs.io/en/latest/wks-on-gce.html)

## Contributing

Please see [CONTRIBUTING.md](CONTRIBUTING.md) and our [Code Of Conduct](CODE_OF_CONDUCT.md).

Other interesting resources include:

- [The issue tracker](https://github.com/weaveworks/wksctl/issues)
- [Developing `wksctl`](https://wksctl.readthedocs.io/en/latest/development.html)

## More Documentation

- [Frequently asked questions](https://wksctl.readthedocs.io/en/latest/faq.html)
- [Developing `wksctl`](https://wksctl.readthedocs.io/en/latest/development.html)

## Getting Help

If you have any questions about, feedback for or problems with `wksctl`:

- Invite yourself to the <a href="https://slack.weave.works/" target="_blank">Weave Users Slack</a>.
- Ask a question on the [#wksctl](https://weave-community.slack.com/messages/wksctl/) slack channel.
- [File an issue](https://github.com/weaveworks/wksctl/issues/new).

Your feedback is always welcome!

## License

[Apache 2.0](LICENSE)
