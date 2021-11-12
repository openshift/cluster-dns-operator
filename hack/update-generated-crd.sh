#!/bin/bash
set -euo pipefail

VENDORED_CRD='vendor/github.com/openshift/api/operator/v1/0000_70_dns-operator_00.crd.yaml'
LOCAL_CRD='manifests/0000_70_dns-operator_00.crd.yaml'

if [[ -z "${SKIP_COPY+1}" ]]; then
  if ! cmp -s "$LOCAL_CRD" "$VENDORED_CRD"; then
    cp -f "$VENDORED_CRD" "$LOCAL_CRD"
  fi
fi
