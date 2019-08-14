#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

firewall_rules=(
  "${network}-external-fw-allow-ping-and-ssh"
  "${network}-external-fw-allow-kubernetes-api"
  "${network}-external-fw-allow-node-ports"
  "${network}-internal-fw"
)

gcloud compute --project="${project}" firewall-rules delete "${firewall_rules[@]}" \
  --quiet

gcloud compute --project="${project}" networks delete "${network}" \
  --quiet
