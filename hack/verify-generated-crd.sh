#!/bin/bash
set -euo pipefail

# openshift/api delivers the dnses.operator.openshift.io and
# dnsnameresolvers.network.openshift.io CRDs in the release payload.  Keep them
# out of this component's manifests to avoid applying two independently
# ordered copies of the same CRD.
#
# Any manifest that contains either CRD name as a standalone token, that is,
# not adjacent to an alphanumeric character, "_", "." or "-", is rejected
# wherever the name appears: after "name:" on the same line or on the next
# line, in a block scalar, in flow-style YAML, in JSON, in a List, or in a
# comment.  Names obscured by escape sequences or line continuations inside
# quoted scalars are not detected.  No other manifest in this repository uses
# these names, so matching on the name alone is safe.
#
# Usage: hack/verify-generated-crd.sh [MANIFEST_DIRECTORY]

MANIFESTS_DIR="${1:-manifests}"
CRD_NAMES='dnses\.operator\.openshift\.io|dnsnameresolvers\.network\.openshift\.io'
NAME_PATTERN="(^|[^[:alnum:]_.-])(${CRD_NAMES})([^[:alnum:]_.-]|\$)"

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
