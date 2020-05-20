#!/bin/bash

scriptdir="$(cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd)"
toolsdir=$scriptdir/../../tools

$scriptdir/upload-image.sh node0 docker.io/weaveworks/wksctl-controller:$($toolsdir/image-tag) $@
