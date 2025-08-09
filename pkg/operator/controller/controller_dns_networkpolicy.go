package controller

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	"github.com/sirupsen/logrus"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ensureDNSNetworkPolicy ensures that a NetworkPolicy exists for the given DNS.
func (r *reconciler) ensureDNSNetworkPolicy(dns *operatorv1.DNS) (bool, *networkingv1.NetworkPolicy, error) {
	haveNP, current, err := r.currentDNSNetworkPolicy(dns)
	if err != nil {
		return false, nil, err
	}

	desired := desiredDNSNetworkPolicy(dns)

	switch {
	case !haveNP:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return false, nil, fmt.Errorf("failed to create dns networkpolicy: %v", err)
		}
		logrus.Infof("created dns networkpolicy: %s/%s", desired.Namespace, desired.Name)
		return r.currentDNSNetworkPolicy(dns)
	case haveNP:
		if updated, err := r.updateDNSNetworkPolicy(current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentDNSNetworkPolicy(dns)
		}
	}
	return true, current, nil
}

func (r *reconciler) currentDNSNetworkPolicy(dns *operatorv1.DNS) (bool, *networkingv1.NetworkPolicy, error) {
	current := &networkingv1.NetworkPolicy{}
	if err := r.client.Get(context.TODO(), DNSNetworkPolicyName(dns), current); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, current, nil
}

func desiredDNSNetworkPolicy(dns *operatorv1.DNS) *networkingv1.NetworkPolicy {
	np := manifests.DNSNetworkPolicy()

	name := DNSNetworkPolicyName(dns)
	np.Namespace = name.Namespace
	np.Name = name.Name
	np.SetOwnerReferences([]metav1.OwnerReference{dnsOwnerRef(dns)})

	if np.Labels == nil {
		np.Labels = map[string]string{}
	}
	np.Labels[manifests.OwningDNSLabel] = DNSDaemonSetLabel(dns)

	// Ensure pod selector matches the DNS daemonset pods for this instance.
	if sel := DNSDaemonSetPodSelector(dns); sel != nil {
		np.Spec.PodSelector = *sel
	}

	return np
}

func (r *reconciler) updateDNSNetworkPolicy(current, desired *networkingv1.NetworkPolicy) (bool, error) {
	changed, updated := networkPolicyChanged(current, desired)
	if !changed {
		return false, nil
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update dns networkpolicy %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated dns networkpolicy %s/%s: %v", updated.Namespace, updated.Name, diff)
	return true, nil
}

func networkPolicyChanged(current, expected *networkingv1.NetworkPolicy) (bool, *networkingv1.NetworkPolicy) {
	// Ignore fields that the API, other controllers, or users may modify.
	npCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
	}

	currentLabels := current.Labels
	if currentLabels == nil {
		currentLabels = map[string]string{}
	}
	expectedLabels := expected.Labels
	if expectedLabels == nil {
		expectedLabels = map[string]string{}
	}

	if cmp.Equal(current.Spec, expected.Spec, npCmpOpts...) && cmp.Equal(currentLabels, expectedLabels, npCmpOpts...) {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Spec = expected.Spec
	if updated.Labels == nil {
		updated.Labels = map[string]string{}
	}
	for k, v := range expectedLabels {
		updated.Labels[k] = v
	}

	return true, updated
}
