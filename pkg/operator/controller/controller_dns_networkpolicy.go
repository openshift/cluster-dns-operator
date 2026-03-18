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
	"k8s.io/apimachinery/pkg/types"
)

func (r *reconciler) ensureDenyAllNetworkPolicy(ctx context.Context) (bool, *networkingv1.NetworkPolicy, error) {
	haveNP, current, err := r.currentDenyAllNetworkPolicy(ctx)
	if err != nil {
		return false, nil, err
	}

	desired := manifests.NetworkPolicyDenyAll()
	switch {
	case !haveNP:
		if err := r.client.Create(ctx, desired); err != nil {
			return false, nil, fmt.Errorf("failed to create networkpolicy: %w", err)
		}
		logrus.Infof("created networkpolicy: %s/%s", desired.Namespace, desired.Name)
		return r.currentDenyAllNetworkPolicy(ctx)
	case haveNP:
		if updated, err := r.updateDNSNetworkPolicy(ctx, current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentDenyAllNetworkPolicy(ctx)
		}
	}
	return true, current, nil
}

func (r *reconciler) currentDenyAllNetworkPolicy(ctx context.Context) (bool, *networkingv1.NetworkPolicy, error) {
	current := manifests.NetworkPolicyDenyAll()
	name := types.NamespacedName{
		Name:      current.Name,
		Namespace: current.Namespace,
	}

	if err := r.client.Get(ctx, name, current); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, current, nil
}

// ensureDNSNetworkPolicy ensures that a NetworkPolicy exists for the given DNS.
func (r *reconciler) ensureDNSNetworkPolicy(ctx context.Context, dns *operatorv1.DNS) (bool, *networkingv1.NetworkPolicy, error) {
	haveNP, current, err := r.currentDNSNetworkPolicy(ctx, dns)
	if err != nil {
		return false, nil, err
	}

	desired := desiredDNSNetworkPolicy(dns)

	switch {
	case !haveNP:
		if err := r.client.Create(ctx, desired); err != nil {
			return false, nil, fmt.Errorf("failed to create dns networkpolicy: %w", err)
		}
		logrus.Infof("created dns networkpolicy: %s/%s", desired.Namespace, desired.Name)
		return r.currentDNSNetworkPolicy(ctx, dns)
	case haveNP:
		if updated, err := r.updateDNSNetworkPolicy(ctx, current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentDNSNetworkPolicy(ctx, dns)
		}
	}
	return true, current, nil
}

func (r *reconciler) currentDNSNetworkPolicy(ctx context.Context, dns *operatorv1.DNS) (bool, *networkingv1.NetworkPolicy, error) {
	current := &networkingv1.NetworkPolicy{}
	if err := r.client.Get(ctx, DNSNetworkPolicyName(dns), current); err != nil {
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

func (r *reconciler) updateDNSNetworkPolicy(ctx context.Context, current, desired *networkingv1.NetworkPolicy) (bool, error) {
	changed, updated := networkPolicyChanged(current, desired)
	if !changed {
		return false, nil
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(ctx, updated); err != nil {
		return false, fmt.Errorf("failed to update dns networkpolicy %s/%s: %w", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated dns networkpolicy %s/%s: %v", updated.Namespace, updated.Name, diff)
	return true, nil
}

func networkPolicyChanged(current, desired *networkingv1.NetworkPolicy) (bool, *networkingv1.NetworkPolicy) {
	// Ignore fields that the API, other controllers, or users may modify.
	npCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
	}
	changed := false
	updated := current.DeepCopy()

	if !cmp.Equal(current.Spec, desired.Spec, npCmpOpts...) {
		updated.Spec = desired.Spec
		changed = true
	}

	if current.Labels == nil {
		current.Labels = map[string]string{}
	}
	if desired.Labels == nil {
		desired.Labels = map[string]string{}
	}
	if !cmp.Equal(current.Labels, desired.Labels, npCmpOpts...) {
		updated.Labels = desired.Labels
		changed = true
	}

	if !cmp.Equal(current.OwnerReferences, desired.OwnerReferences, npCmpOpts...) {
		updated.OwnerReferences = desired.OwnerReferences
		changed = true
	}

	return changed, updated
}
