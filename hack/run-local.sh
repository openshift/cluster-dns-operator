#!/usr/bin/env bash

set -euo pipefail

oc scale --replicas 0 -n openshift-cluster-version deployments/cluster-version-operator
oc scale --replicas 0 -n openshift-dns-operator deployments dns-operator

IMAGE=$(oc get -n openshift-dns-operator deployments/dns-operator -o json | jq -r '.spec.template.spec.containers[0].env[] | select(.name=="IMAGE").value')
OPENSHIFT_CLI_IMAGE=$(oc get -n openshift-dns-operator deployments/dns-operator -o json | jq -r '.spec.template.spec.containers[0].env[] | select(.name=="OPENSHIFT_CLI_IMAGE").value')
KUBE_RBAC_PROXY_IMAGE=$(oc get -n openshift-dns-operator deployments/dns-operator -o json | jq -r '.spec.template.spec.containers[0].env[] | select(.name=="KUBE_RBAC_PROXY_IMAGE").value')
RELEASE_VERSION=$(oc get clusterversion/version -o json | jq -r '.status.desired.version')
NAMESPACE="${NAMESPACE:-"openshift-dns-operator"}"

echo "Image: ${IMAGE}"
echo "OpenShift CLI Image: ${OPENSHIFT_CLI_IMAGE}"
echo "Kube RBAC Proxy Image: ${KUBE_RBAC_PROXY_IMAGE}"
echo "Release version: ${RELEASE_VERSION}"
echo "Operator Namespace: ${NAMESPACE}"

RELEASE_VERSION="${RELEASE_VERSION}" IMAGE="${IMAGE}" OPENSHIFT_CLI_IMAGE="${OPENSHIFT_CLI_IMAGE}" \
KUBE_RBAC_PROXY_IMAGE="${KUBE_RBAC_PROXY_IMAGE}" ./dns-operator "$@"
