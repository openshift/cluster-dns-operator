package controller

import (
	"testing"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestDNSMetricsRoleChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(*rbacv1.Role)
		expect      bool
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *rbacv1.Role) {},
			expect:      false,
		},
		{
			description: "if a rule is added",
			mutate: func(role *rbacv1.Role) {
				role.Rules = append(role.Rules, rbacv1.PolicyRule{
					APIGroups: []string{"example.io"},
					Resources: []string{"foos"},
					Verbs:     []string{"get"},
				})
			},
			expect: true,
		},
		{
			description: "if a rule is removed",
			mutate: func(role *rbacv1.Role) {
				role.Rules = role.Rules[1:]
			},
			expect: true,
		},
		{
			description: "if an annotation is added",
			mutate: func(role *rbacv1.Role) {
				role.Annotations = map[string]string{
					"test": "test",
				}
			},
			expect: false,
		},
	}

	for _, tc := range testCases {
		original := manifests.MetricsRole()
		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := dnsMetricsRoleChanged(original, mutated); changed != tc.expect {
			t.Errorf("%s, expect dnsMetricsRoleChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if changedAgain, _ := dnsMetricsRoleChanged(mutated, updated); changedAgain {
				t.Errorf("%s, dnsMetricsRoleChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
