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
  "vendor/github.com/openshift/api/operator/v1/0000_70_dns-operator_00.crd.yaml" \
  "manifests/0000_70_dns-operator_00.crd.yaml"

verify_crd \
  "vendor/github.com/openshift/api/network/v1alpha1/0000_70_dnsnameresolver_00-techpreview.crd.yaml" \
  "manifests/0000_70_dnsnameresolver_00-techpreview.crd.yaml"

verify_crd \
  "vendor/github.com/openshift/api/network/v1alpha1/0000_70_dnsnameresolver_00-customnoupgrade.crd.yaml" \
  "manifests/0000_70_dnsnameresolver_00-customnoupgrade.crd.yaml"
