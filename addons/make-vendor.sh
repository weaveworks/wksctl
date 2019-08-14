#!/bin/sh -e

rm -rf vendor

jb install

# cleanup the ksonnet repo to just keep the beta.3 jsonnet library
find vendor/ksonnet/ | grep -v -e ksonnet.beta.3 -e ^vendor/ksonnet/$ | xargs rm -rf

# https://github.com/kubernetes-monitoring/kubernetes-mixin/issues/129
rm vendor/kubernetes-mixin/.circleci/jsonnet

# left behind by jb
rmdir vendor/.tmp
