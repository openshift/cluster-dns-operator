#!/bin/bash
set -euo pipefail

if [ -z "${KUBECONFIG:-}" ]; then echo "KUBECONFIG is required"; exit 1; fi

# Required for the operator-sdk.
export KUBERNETES_CONFIG="${KUBECONFIG}"

go test -v -tags integration ./test/integration
