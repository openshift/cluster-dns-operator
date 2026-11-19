#!/bin/bash
set -euo pipefail

# openshift/api delivers the dnses.operator.openshift.io and
# dnsnameresolvers.network.openshift.io CRDs in the release payload.  Keep them
# out of this component's manifests to avoid applying two independently
# ordered copies of the same CRD.
#
# Any manifest that names either CRD is rejected.  No other manifest in this
# repository uses these names, so matching on the name alone is sufficient, and
# it also catches flow-style YAML, JSON, and List-wrapped manifests.
#
# Usage: hack/verify-generated-crd.sh [MANIFEST_DIRECTORY]

MANIFESTS_DIR="${1:-manifests}"
CRD_NAMES='dnses\.operator\.openshift\.io|dnsnameresolvers\.network\.openshift\.io'
NAME_PATTERN="(^|[[:space:]{,])[\"']?name[\"']?[[:space:]]*:[[:space:]]*[\"']?(${CRD_NAMES})[\"']?([[:space:],}#]|\$)"

if [[ ! -d "$MANIFESTS_DIR" ]]; then
  echo "manifest directory not found: $MANIFESTS_DIR" >&2
  exit 2
fi

status=0
matches=$(grep -rlE --include='*.yaml' --include='*.yml' --include='*.json' \
  -e "$NAME_PATTERN" -- "$MANIFESTS_DIR") || status=$?

case "$status" in
  0)
    printf '%s\n' "$matches" >&2
    echo "dnses.operator.openshift.io and dnsnameresolvers.network.openshift.io CRDs must only ship from openshift/api" >&2
    exit 1
    ;;
  1)
    exit 0
    ;;
  *)
    if [[ -n "$matches" ]]; then
      printf '%s\n' "$matches" >&2
    fi
    echo "failed to scan $MANIFESTS_DIR for CRDs that must only ship from openshift/api" >&2
    exit 2
    ;;
esac
