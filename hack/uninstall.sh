#!/bin/bash
set -uo pipefail

# Disable the CVO
oc scale --replicas 0 -n openshift-cluster-version deployments/cluster-version-operator

# Uninstall cluster-dns-operator
oc delete namespaces/openshift-cluster-dns-operator
oc patch -n openshift-dns-operator clusterdnses/default --patch '{"metadata":{"finalizers": []}}' --type=merge
oc delete --force --grace-period 0 -n openshift-cluster-dns-operator clusterdnses/default
oc delete namespaces/openshift-cluster-dns
oc delete clusterroles/cluster-dns-operator:operator
oc delete clusterroles/cluster-dns:dns
oc delete clusterrolebindings/cluster-dns-operator:operator
oc delete clusterrolebindings/cluster-dns:dns
oc delete customresourcedefinition.apiextensions.k8s.io/clusterdnses.dns.openshift.io
