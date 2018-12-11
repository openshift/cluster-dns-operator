#!/bin/bash
set -uo pipefail

# Disable the CVO
oc scale --replicas 0 -n openshift-cluster-version deployments/cluster-version-operator

# Uninstall openshift-dns-operator
oc patch -n openshift-dns-operator clusterdnses/default --patch '{"metadata":{"finalizers": []}}' --type=merge
oc delete --force --grace-period 0 -n openshift-dns-operator clusterdnses/default
oc delete namespaces/openshift-dns-operator
oc delete namespaces/openshift-dns
oc delete clusterroles/openshift-dns-operator
oc delete clusterroles/openshift-dns
oc delete clusterrolebindings/openshift-dns-operator
oc delete clusterrolebindings/openshift-dns
oc delete customresourcedefinition.apiextensions.k8s.io/clusterdnses.dns.openshift.io
