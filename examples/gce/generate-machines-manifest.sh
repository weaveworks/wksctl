#!/bin/bash

set -o errexit
set -o pipefail
set -o nounset

gcloud compute --project="${project}" instances list --filter="zone:(${zone}) AND name~'^${user}-wks-\d+$'" --format=json > instances.json
jk run -p user=$user generate-machines-manifest.js
