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

func (r *reconciler) ensureDNSClusterRole() (*rbacv1.ClusterRole, error) {
	current, err := r.currentDNSClusterRole()
	if err != nil {
		return nil, err
	}
	desired := desiredDNSClusterRole()

	switch {
	case desired != nil && current == nil:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return nil, fmt.Errorf("failed to create dns cluster role: %v", err)
		}
		logrus.Infof("created dns cluster role: %s/%s", desired.Namespace, desired.Name)
	case desired != nil && current != nil:
		if err := r.updateDNSClusterRole(current, desired); err != nil {
			return nil, err
		}
	}
	return r.currentDNSClusterRole()
}

func (r *reconciler) currentDNSClusterRole() (*rbacv1.ClusterRole, error) {
	current := &rbacv1.ClusterRole{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: manifests.DNSClusterRole().Name}, current)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return current, nil
}

func desiredDNSClusterRole() *rbacv1.ClusterRole {
	cr := manifests.DNSClusterRole()
	return cr
}

func (r *reconciler) updateDNSClusterRole(current, desired *rbacv1.ClusterRole) error {
	changed, updated := clusterRoleChanged(current, desired)
	if !changed {
		return nil
	}

	if err := r.client.Update(context.TODO(), updated); err != nil {
		return fmt.Errorf("failed to update dns cluster role %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated dns cluster role: %s/%s", updated.Namespace, updated.Name)
	return nil
}

func clusterRoleChanged(current, expected *rbacv1.ClusterRole) (bool, *rbacv1.ClusterRole) {
	if cmp.Equal(current.Rules, expected.Rules, cmpopts.EquateEmpty()) {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Rules = expected.Rules

	return true, updated
}
