# wksctl Documentation

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

## Table of contents

- [Get started](get-started.md)
- [WKS and Firekube](wks-and-firekube.md)
- [WKS and Footloose](wks-and-footloose.md)
- [WKS and Vagrant](wks-and-vagrant.md)
- [WKS on GCE](wks-on-gce.md)
- [FAQ](faq.md)
- [Development](development.md)


## Getting Help

If you have any questions about, feedback for or problems with `wksctl`:

- Invite yourself to the <a href="https://slack.weave.works/" target="_blank">Weave Users Slack</a>.
- Ask a question on the [#wksctl](https://weave-community.slack.com/messages/wksctl/) slack channel.
- [File an issue](https://github.com/weaveworks/wksctl/issues/new).

Your feedback is always welcome!
