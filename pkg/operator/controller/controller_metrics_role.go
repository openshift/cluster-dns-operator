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

func (r *reconciler) ensureDNSMetricsRole() (bool, *rbacv1.Role, error) {
	desired := manifests.MetricsRole()

	have, current, err := r.currentDNSMetricsRole()
	if err != nil {
		return false, nil, err
	}

	switch {
	case !have:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return false, nil, fmt.Errorf("failed to create dns metrics role %s/%s: %v", desired.GetNamespace(), desired.GetName(), err)
		}
		logrus.Infof("created dns metrics role %s/%s", desired.GetNamespace(), desired.GetName())
		return r.currentDNSMetricsRole()
	case have:
		if updated, err := r.updateDNSMetricsRole(current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentDNSMetricsRole()
		}
	}
	return true, current, nil
}

func (r *reconciler) currentDNSMetricsRole() (bool, *rbacv1.Role, error) {
	desired := manifests.MetricsRole()
	current := &rbacv1.Role{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: desired.GetNamespace(), Name: desired.GetName()}, current); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, current, nil
}

func (r *reconciler) updateDNSMetricsRole(current, desired *rbacv1.Role) (bool, error) {
	changed, updated := dnsMetricsRoleChanged(current, desired)
	if !changed {
		return false, nil
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update dns metrics role %s/%s: %v", updated.GetNamespace(), updated.GetName(), err)
	}
	logrus.Infof("updated dns metrics role %s/%s: %v", updated.GetNamespace(), updated.GetName(), diff)
	return true, nil
}

func dnsMetricsRoleChanged(current, desired *rbacv1.Role) (bool, *rbacv1.Role) {
	if cmp.Equal(current.Rules, desired.Rules, cmpopts.EquateEmpty()) {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Rules = desired.Rules

	return true, updated
}
