#!/bin/bash
set -euo pipefail

REPO="${REPO:-}"
MANIFESTS="${MANIFESTS:-}"

if [ -z "$REPO" ]; then echo "REPO is required"; exit 1; fi
if [ -z "$MANIFESTS" ]; then echo "MANIFESTS is required"; exit 1; fi

TEMP_COMMIT="false"
test -z "$(git status --porcelain)" || TEMP_COMMIT="true"

if [[ "${TEMP_COMMIT}" == "true" ]]; then
  git add .
  git commit -m "Temporary" || true
fi

REV=$(git rev-parse --short HEAD)
if [[ -z "${DOCKER+1}" ]] && command -v buildah >& /dev/null; then
  buildah bud -t $REPO:$REV .
  buildah push $REPO:$REV docker://$REPO:$REV
else
  docker build -t $REPO:$REV .
  docker push $REPO:$REV
fi

if [[ "${TEMP_COMMIT}" == "true" ]]; then
  git reset --soft HEAD~1
fi

cp -R manifests/* $MANIFESTS
# The dnses.operator.openshift.io CRD is delivered in the release payload by
# openshift/api, so it is no longer in manifests/.  Add the vendored copy so
# that applying $MANIFESTS (for example after hack/uninstall.sh, which deletes
# the CRD) still restores it.  The dnsnameresolvers CRD is not added because
# hack/uninstall.sh does not delete it.
cp vendor/github.com/openshift/api/operator/v1/zz_generated.crd-manifests/0000_70_dns_00_dnses.crd.yaml "$MANIFESTS/0000_70_dns-operator_00.crd.yaml"
cat manifests/0000_70_dns-operator_02-deployment.yaml | sed "s~openshift/origin-cluster-dns-operator:latest~$REPO:$REV~" > "$MANIFESTS/0000_70_dns-operator_02-deployment.yaml"
# To simulate CVO, ClusterOperator resource need to be created by the operator.
rm $MANIFESTS/0000_70_dns-operator_03-cluster-operator.yaml

echo "Pushed $REPO:$REV"
echo "Install manifests using:"
echo ""
echo "oc apply -f $MANIFESTS"
