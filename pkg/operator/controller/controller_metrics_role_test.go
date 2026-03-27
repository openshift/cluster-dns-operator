package controller

import (
	"testing"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestDNSggMetricsRoleChanged(t *testing.T) {
	role1 := manifests.MetricsRole()
	role2 := manifests.MetricsRole()
	if changed, _ := dnsMetricsRoleChanged(role1, role2); changed {
		t.Fatal("expected changed to be false for two roles with identical rules")
	}
	role2.Rules = append(role2.Rules, rbacv1.PolicyRule{
		APIGroups: []string{"example.io"},
		Resources: []string{"foos"},
		Verbs:     []string{"get"},
	})
	if changed, updated := dnsMetricsRoleChanged(role1, role2); !changed {
		t.Fatal("expected changed to be true after adding a rule")
	} else if changedAgain, _ := dnsMetricsRoleChanged(role2, updated); changedAgain {
		t.Fatal("dnsMetricsRoleChanged does not behave as a fixed-point function")
	}
}
