#!/bin/bash

set -e

[ -n "$SECRET_KEY" ] || {
    echo "Cannot run smoke tests: no secret key"
    exit 1
}

# Ensure .ssh directory exists so we can unpack things into it
mkdir -p $HOME/.ssh

# Base name of VMs for integration tests:
export NAME=test-$CIRCLE_BUILD_NUM-$CIRCLE_NODE_INDEX

$SRCDIR/test/integration/bin/internal/run-integration-tests.sh configure
echo "Test VMs now provisioned and configured. $(date)."
