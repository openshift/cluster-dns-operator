package controller

import (
	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestDNSNamespaceLabelsChanged(t *testing.T) {
	originalNamespace := manifests.DNSNamespace()

	testCases := []struct {
		description    string
		mutate         func(namespace *corev1.Namespace)
		changeExpected bool
	}{
		{
			description:    "if nothing changes",
			mutate:         func(_ *corev1.Namespace) {},
			changeExpected: false,
		},
		{
			description: "if arbitrary label added",
			mutate: func(namespace *corev1.Namespace) {
				if namespace.Labels == nil {
					namespace.Labels = map[string]string{}
				}
				namespace.Labels["foo"] = "bar"
			},
			changeExpected: false,
		},
		{
			description: "if required label changes",
			mutate: func(namespace *corev1.Namespace) {
				if namespace.Labels == nil {
					namespace.Labels = map[string]string{}
				}
				namespace.Labels[namespacePodSecurityEnforceLabel] = ""
			},
			changeExpected: true,
		},
	}

	for _, tc := range testCases {
		mutated := originalNamespace.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := namespaceLabelsChanged(originalNamespace, mutated); changed != tc.changeExpected {
			t.Errorf("%s, expect namespaceLabelsChanged to be %t, got %t", tc.description, tc.changeExpected, changed)
		} else if changed {
			if changedAgain, _ := namespaceLabelsChanged(updated, mutated); changedAgain {
				t.Errorf("%s, namespaceLabelsChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
