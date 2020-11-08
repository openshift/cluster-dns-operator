package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
)

// ensureNodeResolverDaemonset ensures the node resolver daemonset exists if it
// should or does not exist if it should not exist.  Returns a Boolean
// indicating whether the daemonset exists, the daemonset if it does exist, and
// an error value.
func (r *reconciler) ensureNodeResolverDaemonSet(clusterIP, clusterDomain string) (bool, *appsv1.DaemonSet, error) {
	haveDS, current, err := r.currentNodeResolverDaemonSet()
	if err != nil {
		return false, nil, err
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
