package controller

import (
	"testing"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestDNSClusterRoleChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(*rbacv1.ClusterRole)
		expect      bool
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *rbacv1.ClusterRole) {},
			expect:      false,
		},
		{
			description: "if rule is added",
			mutate: func(cr *rbacv1.ClusterRole) {
				cr.Rules = append(cr.Rules, rbacv1.PolicyRule{})
			},
			expect: true,
		},
		{
			description: "if rule is removed",
			mutate: func(cr *rbacv1.ClusterRole) {
				cr.Rules = cr.Rules[1:]
			},
			expect: true,
		},
		{
			description: "if an annotation is added",
			mutate: func(cr *rbacv1.ClusterRole) {
				cr.Annotations = map[string]string{
					"test": "test",
				}
			},
			expect: false,
		},
	}

	for _, tc := range testCases {
		original := manifests.DNSClusterRole()
		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := clusterRoleChanged(original, mutated); changed != tc.expect {
			t.Errorf("%s, expect clusterRoleChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if changedAgain, _ := clusterRoleChanged(mutated, updated); changedAgain {
				t.Errorf("%s, clusterRoleChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
