#!/bin/bash
set -euo pipefail

VENDORED_CRD='vendor/github.com/openshift/api/operator/v1/0000_70_dns-operator_00-custom-resource-definition.yaml'
LOCAL_CRD='manifests/0000_70_dns-operator_00-custom-resource-definition.yaml'

diff -Naup "$LOCAL_CRD" "$VENDORED_CRD"
