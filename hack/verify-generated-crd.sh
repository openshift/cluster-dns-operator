#!/bin/bash
set -euo pipefail

TMP_DIR="$(mktemp -d)"

OUTDIR=$TMP_DIR SKIP_COPY=true ./hack/update-generated-crd.sh

diff -Naup "$TMP_DIR/operator.openshift.io_dnses.yaml" manifests/0000_70_dns-operator_00-custom-resource-definition.yaml
