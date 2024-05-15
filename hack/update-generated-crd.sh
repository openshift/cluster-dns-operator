#!/bin/bash
set -euo pipefail

function install_crd {
  if [[ -z "${SKIP_COPY+1}" ]]; then
    local SRC="$1"
    local DST="$2"
    if ! cmp -s "$DST" "$SRC"; then
      cp -f "$SRC" "$DST"
    fi
  fi
}

# Can't rely on associative arrays for old Bash versions (e.g. OSX)
install_crd \
  "vendor/github.com/openshift/api/operator/v1/zz_generated.crd-manifests/0000_70_dns_00_dnses.crd.yaml" \
  "manifests/0000_70_dns-operator_00.crd.yaml"

install_crd \
  "vendor/github.com/openshift/api/network/v1alpha1/zz_generated.crd-manifests/0000_70_dns_00_dnsnameresolvers-TechPreviewNoUpgrade.crd.yaml" \
  "manifests/0000_70_dns_00_dnsnameresolvers-TechPreviewNoUpgrade.crd.yaml"

install_crd \
  "vendor/github.com/openshift/api/network/v1alpha1/zz_generated.crd-manifests/0000_70_dns_00_dnsnameresolvers-CustomNoUpgrade.crd.yaml" \
  "manifests/0000_70_dns_00_dnsnameresolvers-CustomNoUpgrade.crd.yaml"

install_crd \
  "vendor/github.com/openshift/api/network/v1alpha1/zz_generated.crd-manifests/0000_70_dns_00_dnsnameresolvers-DevPreviewNoUpgrade.crd.yaml" \
  "manifests/0000_70_dns_00_dnsnameresolvers-DevPreviewNoUpgrade.crd.yaml"
