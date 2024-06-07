package controller

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	"github.com/sirupsen/logrus"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (r *reconciler) ensureDNSClusterRole() (bool, *rbacv1.ClusterRole, error) {
	haveCR, current, err := r.currentDNSClusterRole()
	if err != nil {
		return false, nil, err
	}
	desired := desiredDNSClusterRole(r.dnsNameResolverEnabled)

	switch {
	case !haveCR:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return false, nil, fmt.Errorf("failed to create dns cluster role: %v", err)
		}
		logrus.Infof("created dns cluster role: %s/%s", desired.Namespace, desired.Name)
		return r.currentDNSClusterRole()
	case haveCR:
		if updated, err := r.updateDNSClusterRole(current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentDNSClusterRole()
		}
	}
	return true, current, nil
}

func (r *reconciler) currentDNSClusterRole() (bool, *rbacv1.ClusterRole, error) {
	current := &rbacv1.ClusterRole{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: manifests.DNSClusterRole().Name}, current)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, current, nil
}

func desiredDNSClusterRole(dnsNameResolverEnabled bool) *rbacv1.ClusterRole {
	cr := manifests.DNSClusterRole()
	if dnsNameResolverEnabled {
		addDNSNameResolverPolicyRule(cr)
	}
	return cr
}

func addDNSNameResolverPolicyRule(cr *rbacv1.ClusterRole) {
	cr.Rules = append(cr.Rules, rbacv1.PolicyRule{
		APIGroups: []string{"network.openshift.io"},
		Resources: []string{"dnsnameresolvers"},
		Verbs:     []string{"get", "list", "watch"},
	})
	cr.Rules = append(cr.Rules, rbacv1.PolicyRule{
		APIGroups: []string{"network.openshift.io"},
		Resources: []string{"dnsnameresolvers/status"},
		Verbs:     []string{"get", "update", "patch"},
	})
}

func (r *reconciler) updateDNSClusterRole(current, desired *rbacv1.ClusterRole) (bool, error) {
	changed, updated := clusterRoleChanged(current, desired)
	if !changed {
		return false, nil
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update dns cluster role %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated dns cluster role %s/%s: %v", updated.Namespace, updated.Name, diff)
	return true, nil
}

func clusterRoleChanged(current, expected *rbacv1.ClusterRole) (bool, *rbacv1.ClusterRole) {
	if cmp.Equal(current.Rules, expected.Rules, cmpopts.EquateEmpty()) {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Rules = expected.Rules

	return true, updated
}
