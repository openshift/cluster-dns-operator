package controller

import (
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDNSNetworkPolicyChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(*networkingv1.NetworkPolicy)
		expect      bool
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *networkingv1.NetworkPolicy) {},
			expect:      false,
		},
		{
			description: "if .spec.podSelector changes",
			mutate: func(np *networkingv1.NetworkPolicy) {
				np.Spec.PodSelector = metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}}
			},
			expect: true,
		},
		{
			description: "if an ingress rule is added",
			mutate: func(np *networkingv1.NetworkPolicy) {
				np.Spec.Ingress = append(np.Spec.Ingress, networkingv1.NetworkPolicyIngressRule{})
			},
			expect: true,
		},
		{
			description: "if a label is added",
			mutate: func(np *networkingv1.NetworkPolicy) {
				if np.Labels == nil {
					np.Labels = map[string]string{}
				}
				np.Labels[manifests.OwningDNSLabel] = "changed"
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		dns := &operatorv1.DNS{ObjectMeta: metav1.ObjectMeta{Name: "default", UID: "1"}}
		original := desiredDNSNetworkPolicy(dns)
		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := networkPolicyChanged(original, mutated); changed != tc.expect {
			t.Errorf("%s, expect networkPolicyChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if changedAgain, _ := networkPolicyChanged(mutated, updated); changedAgain {
				t.Errorf("%s, networkPolicyChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
