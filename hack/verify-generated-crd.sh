#!/bin/bash
set -euo pipefail

function verify_crd {
  local SRC="$1"
  local DST="$2"
  if ! diff -Naup "$SRC" "$DST"; then
    echo "invalid CRD: $SRC => $DST"
    exit 1
  fi
}

verify_crd \
  "vendor/github.com/openshift/api/operator/v1/zz_generated.crd-manifests/0000_70_dns_00_dnses.crd.yaml" \
  "manifests/0000_70_dns-operator_00.crd.yaml"
