#!/bin/bash

set -e

# XXX: This assumes wksctl is going to install a specific upstream version of
# docker. It may not be true in the future. The best way to solve this
# generically would be able to query wksctl what is its plan and install the
# same docker we want to install anyway.

# XXX: This also will not work with other CRI runtimes.

if [ $# -ne 2 ] && [ $# -ne 4 ]; then
    echo "Usage: $(basename $0) NODE IMAGE [-c footlooseconfig]"
    exit 1
fi

# Holds the footloose command.  If an alternate footloose.yaml is passed via the command line,
# override the footloose command to use it
footlooseCmd=footloose
if [ $# -eq 4 ]; then
    echo "Using config $@"
    footlooseCmd="footloose $3 $4"
fi

ensure_docker_centos() {
    node=$1
    $footlooseCmd ssh root@$node -- sudo yum-config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
    $footlooseCmd ssh root@$node -- sudo yum install -y docker-ce-19.03.1
    $footlooseCmd ssh root@$node -- sudo systemctl start docker
}

ensure_docker_ubuntu() {
    node=$1
    $footlooseCmd ssh root@$node -- apt-get update
    $footlooseCmd ssh root@$node -- apt-get install docker.io
}

ensure_docker() {
    node=$1
    os=$2
    case $os in
        centos)
            ensure_docker_centos $node
            ;;
        ubuntu)
            ensure_docker_ubuntu $node
            ;;
        *)
            echo "error: unknown os: '$os'"
            exit 0
    esac
}

os() {
    node=$1
    os=$($footlooseCmd ssh root@$node cat /etc/os-release  | grep ^ID= | cut -d= -f2 | tr -d '"')
    echo $os
}

upload() {
    node=$1
    image=$2
    docker save $image | $footlooseCmd ssh root@$node docker load
}

node=$1
image=$2
os=$(os $node)

ensure_docker $node $os
upload $node $image
