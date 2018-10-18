#!/bin/bash
set -uo pipefail

# Disable the CVO
oc patch -n openshift-cluster-version daemonsets/cluster-version-operator --patch '{"spec": {"template": {"spec": {"nodeSelector": {"node-role.kubernetes.io/fake": ""}}}}}'

# Uninstall kube-dns
oc scale -n kube-system --replicas 0 deployments/kube-core-operator
oc scale -n kube-system --replicas 0 deployments/kube-dns
oc delete -n kube-system services/kube-dns

# Uninstall cluster-dns-operator
oc delete --force --grace-period 0 -n openshift-cluster-dns-operator clusterdnses/default
oc delete namespaces/openshift-cluster-dns-operator
oc delete namespaces/openshift-cluster-dns
oc delete clusterroles/cluster-dns-operator:operator
oc delete clusterroles/cluster-dns:dns
oc delete clusterrolebindings/cluster-dns-operator:operator
oc delete clusterrolebindings/cluster-dns:dns
oc delete customresourcedefinition.apiextensions.k8s.io/clusterdnses.dns.openshift.io
