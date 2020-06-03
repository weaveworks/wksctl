#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

ensure_ssh_key() {
    name=$1
    [ -f $name ] && return
    ssh-keygen -q -t rsa -b 4096 -C "wks-dev@weave.works" -f $name -N ""
}

ensure_ssh_key cluster-key
echo "wks:$(cat ./cluster-key.pub | awk '{print $1" "$2" wks"}')" > ssh_key.list

gcloud compute --project="${project}" instances create ${user}-wks-{1,2,3} \
  --network="${network}" \
  --zone="${zone}" \
  --metadata-from-file="ssh-keys=ssh_key.list" \
  --machine-type=n1-standard-2 \
  --image-project=${image_project} \
  --image-family=${image_family}

for i in ${user}-wks-1 ; do
  gcloud compute --project="${project}" instances add-tags "${i}" \
  --zone="${zone}" \
  --tags=master
done

for i in ${user}-wks-{2,3} ; do
  gcloud compute --project="${project}" instances add-tags "${i}" \
  --zone="${zone}" \
  --tags=node
done
