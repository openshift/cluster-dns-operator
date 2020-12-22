package controller

import (
	"context"
	"fmt"

	operatorv1 "github.com/openshift/api/operator/v1"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
)

// ensureNodeResolverDaemonset ensures the node resolver daemonset exists if it
// should or does not exist if it should not exist.  Returns a Boolean
// indicating whether the daemonset exists, the daemonset if it does exist, and
// an error value.
func (r *reconciler) ensureNodeResolverDaemonSet(dns *operatorv1.DNS, clusterIP, clusterDomain string) (bool, *appsv1.DaemonSet, error) {
	haveDS, current, err := r.currentNodeResolverDaemonSet()
	if err != nil {
		return false, nil, err
	}
	if haveDS {
		if owns, err := dnsOwnsObject(dns, current); err != nil {
			return haveDS, current, err
		} else if !owns {
			// Return the daemonset without checking if it needs to
			// be updated because we do not own it.
			return haveDS, current, nil
		}
	}
	wantDS, _, err := desiredNodeResolverDaemonSet(clusterIP, clusterDomain, r.OpenshiftCLIImage)
	if err != nil {
		return haveDS, current, fmt.Errorf("failed to build node resolver daemonset: %v", err)
	}
	switch {
	case !wantDS && haveDS:
		if err := r.deleteNodeResolverDaemonSet(current); err != nil {
			return true, current, err
		}
	}
	return false, nil, nil
}

// desiredNodeResolverDaemonSet returns the desired node resolver daemonset.
func desiredNodeResolverDaemonSet(clusterIP, clusterDomain, openshiftCLIImage string) (bool, *appsv1.DaemonSet, error) {
	return false, nil, nil
}

// currentNodeResolverDaemonSet returns the current DNS node resolver
// daemonset.
func (r *reconciler) currentNodeResolverDaemonSet() (bool, *appsv1.DaemonSet, error) {
	daemonset := &appsv1.DaemonSet{}
	if err := r.client.Get(context.TODO(), NodeResolverDaemonSetName(), daemonset); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, daemonset, nil
}

// deleteNodeResolverDaemonSet deletes a DNS node resolver daemonset.
func (r *reconciler) deleteNodeResolverDaemonSet(daemonset *appsv1.DaemonSet) error {
	if err := r.client.Delete(context.TODO(), daemonset); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete node resolver daemonset %s/%s: %v", daemonset.Namespace, daemonset.Name, err)
	}
	logrus.Infof("deleted node resolver daemonset: %s/%s", daemonset.Namespace, daemonset.Name)
	return nil
}

// verifyNodeResolverDaemonIsUpgradeable returns an error value indicating if
// the node resolver daemonset is safe to upgrade.  In particular, if the
// daemonset exists but is not labeled as owned by the dns, then the daemonset
// is not upgradeable, and an error is returned.  Otherwise, nil is returned.
func verifyNodeResolverDaemonIsUpgradeable(dns *operatorv1.DNS, ds *appsv1.DaemonSet) error {
	owns, err := dnsOwnsObject(dns, ds)
	if err != nil {
		return err
	}
	if !owns {
		return fmt.Errorf("daemonset %s/%s exists but is not owned by dns %s", ds.Namespace, ds.Name, dns.Name)
	}
	return nil
}
