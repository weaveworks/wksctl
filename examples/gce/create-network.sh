#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

gcloud compute --project="${project}" networks create "${network}"

# Allow ping and SSH for all instances
gcloud compute --project="${project}" firewall-rules create "${network}-external-fw-allow-ping-and-ssh" \
  --network="${network}" \
  --allow="tcp:22,icmp" \
  --target-tags=master,node

# Allow Kubernetes API port
gcloud compute --project="${project}" firewall-rules create "${network}-external-fw-allow-kubernetes-api" \
  --network="${network}" \
  --allow="tcp:6443" \
  --target-tags=master

# Allow node ports for all instances
gcloud compute --project="${project}" firewall-rules create "${network}-external-fw-allow-node-ports" \
  --network="${network}" \
  --allow="tcp:30000-32767" \
  --target-tags=node

# Allow all high ports internally
gcloud compute --project="${project}" firewall-rules create "${network}-internal-fw" \
  --network="${network}" \
  --allow="tcp:1024-65535,udp:1024-65535,icmp" \
  --source-tags=master,node \
  --target-tags=master,node
