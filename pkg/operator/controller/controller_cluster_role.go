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
	desired := desiredDNSClusterRole()

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

func desiredDNSClusterRole() *rbacv1.ClusterRole {
	cr := manifests.DNSClusterRole()
	return cr
}

func (r *reconciler) updateDNSClusterRole(current, desired *rbacv1.ClusterRole) (bool, error) {
	changed, updated := clusterRoleChanged(current, desired)
	if !changed {
		return false, nil
	}

	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update dns cluster role %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated dns cluster role: %s/%s", updated.Namespace, updated.Name)
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
