#!/bin/bash

# Description:
#   Retags the container images currently used by WKS, and pushes them to a
#   local Docker registry.
#
# N.B.:
#   Note that the below container images will change depending on the version
#   of Kubernetes WKS runs. You can retrieve these by running:
#     kubeadm config images list
#   (see also https://kubernetes.io/docs/reference/setup-tools/kubeadm/kubeadm-config/#cmd-config-images-list)
#   on the WKS seed master node, or by looking at the WKS
#   controller's logs.

set -e

# Images currently used by WKS:
IMAGES=(
k8s.gcr.io/kube-apiserver:v1.16.8
k8s.gcr.io/kube-controller-manager:v1.16.8
k8s.gcr.io/kube-scheduler:v1.16.8
k8s.gcr.io/kube-proxy:v1.16.8
k8s.gcr.io/pause:3.1
k8s.gcr.io/etcd:3.3.15-0
k8s.gcr.io/coredns:1.6.2
docker.io/weaveworks/weave-npc:2.5.1
docker.io/weaveworks/weave-kube:2.5.1
)

# Default host for the local Docker registry:
HOST=localhost
# Default port for the local Docker registry:
PORT=5000

function print_remote() {
    for remote_img in ${IMAGES[@]}; do
        echo "${remote_img}"
    done
}

function localise() {
    echo ${1} | sed "s@^[^/]*@${HOST}:${PORT}@"
}

function print_local() {
    for remote_img in ${IMAGES[@]}; do
        echo "$(localise "${remote_img}")"
    done
}

function retag_push() {
    for remote_img in ${IMAGES[@]}; do
        docker pull "${remote_img}"
        local local_img="$(localise "${remote_img}")"
        docker tag "${remote_img}" "${local_img}"
        docker push "${local_img}"
    done
}

function main() {
    while true; do
        case "${1}" in
            -h|--host)
                shift
                HOST="${1}"
                ;;
            -p|--port)
                shift
                PORT="${1}"
                ;;
            --print-remote)
                print_remote
                exit
                ;;
            --print-local)
                print_local
                exit
                ;;
            *)
                [ -z "${1}" ] && break
                echo "Unknown option or argument: \"${1}\"" && exit 1
                ;;
        esac
        shift
    done
    retag_push
}

main "${@}"
