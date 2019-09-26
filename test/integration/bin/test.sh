#!/bin/bash

set -e

# Run integration tests
IMGTAG=$(./tools/image-tag)

export PATH=$GOROOT/bin:$PATH
# Work around for broken docker package in RHEL
export DOCKER_VERSION='1.13.1-75*'
go test -failfast -v -timeout 1h ./test/integration/test -args -run.interactive -cmd /tmp/workspace/cmd/wksctl/wksctl -tags.wks-k8s-krb5-server=$IMGTAG -tags.wks-mock-authz-server=$IMGTAG
