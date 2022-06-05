package controller

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	corev1 "k8s.io/api/core/v1"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// servingCertAnnotationKey is the annotation key to request
	// a certificate/key pair from the serving cert signer.
	servingCertAnnotationKey = "service.beta.openshift.io/serving-cert-secret-name"

	// topologyAwareHintsAnnotationKey is the annotation key to enable topology aware hints
	// on a service to prefer keeping traffic within a zone.
	// For docs see: https://kubernetes.io/docs/concepts/services-networking/topology-aware-hints/
	topologyAwareHintsAnnotationKey = "service.kubernetes.io/topology-aware-hints"
)

var (
	// managedDNSServiceAnnotations is the set of keys for annotations that
	// the operator manages for the DNS service.
	managedDNSServiceAnnotations = sets.NewString(
		servingCertAnnotationKey,
		topologyAwareHintsAnnotationKey,
	)
)

// ensureDNSService ensures that a service exists for a given DNS.
func (r *reconciler) ensureDNSService(dns *operatorv1.DNS, clusterIP string, daemonsetRef metav1.OwnerReference) (bool, *corev1.Service, error) {
	haveService, current, err := r.currentDNSService(dns)
	if err != nil {
		return false, nil, err
	}
	desired := desiredDNSService(dns, clusterIP, daemonsetRef)

	switch {
	case !haveService:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return false, nil, fmt.Errorf("failed to create dns service: %v", err)
		}
		logrus.Infof("created dns service: %s/%s", desired.Namespace, desired.Name)
		return r.currentDNSService(dns)
	case haveService:
		if updated, err := r.updateDNSService(current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentDNSService(dns)
		}
	}
	return true, current, nil
}

func (r *reconciler) currentDNSService(dns *operatorv1.DNS) (bool, *corev1.Service, error) {
	current := &corev1.Service{}
	err := r.client.Get(context.TODO(), DNSServiceName(dns), current)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, current, nil
}

func desiredDNSService(dns *operatorv1.DNS, clusterIP string, daemonsetRef metav1.OwnerReference) *corev1.Service {
	s := manifests.DNSService()

	name := DNSServiceName(dns)
	s.Namespace = name.Namespace
	s.Name = name.Name
	s.SetOwnerReferences([]metav1.OwnerReference{dnsOwnerRef(dns)})

	s.Annotations = map[string]string{
		MetricsServingCertAnnotation:    DNSMetricsSecretName(dns),
		topologyAwareHintsAnnotationKey: "auto",
	}

	s.Labels = map[string]string{
		manifests.OwningDNSLabel: DNSDaemonSetLabel(dns),
	}

	s.Spec.Selector = DNSDaemonSetPodSelector(dns).MatchLabels

	if len(clusterIP) > 0 {
		s.Spec.ClusterIP = clusterIP
	}
	return s
}

func (r *reconciler) updateDNSService(current, desired *corev1.Service) (bool, error) {
	changed, updated := serviceChanged(current, desired)
	if !changed {
		return false, nil
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update dns service %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated dns service %s/%s: %v", updated.Namespace, updated.Name, diff)
	return true, nil
}

func serviceChanged(current, expected *corev1.Service) (bool, *corev1.Service) {
	annotationCmpOpts := []cmp.Option{
		cmpopts.IgnoreMapEntries(func(k, _ string) bool {
			return !managedDNSServiceAnnotations.Has(k)
		}),
	}
	serviceCmpOpts := []cmp.Option{
		// Ignore fields that the API, other controllers, or user may
		// have modified.
		cmpopts.IgnoreFields(
			corev1.ServiceSpec{},
			"ClusterIP", "ClusterIPs",
			"IPFamilies", "IPFamilyPolicy",
		),
		cmp.Comparer(cmpServiceAffinity),
		cmp.Comparer(cmpServiceType),
		cmp.Comparer(cmpServiceInternalTrafficPolicyType),
		cmpopts.EquateEmpty(),
	}

	currentAnnotations := current.Annotations
	if currentAnnotations == nil {
		currentAnnotations = map[string]string{}
	}
	expectedAnnotations := expected.Annotations
	if expectedAnnotations == nil {
		expectedAnnotations = map[string]string{}
	}
	if cmp.Equal(current.Spec, expected.Spec, serviceCmpOpts...) && cmp.Equal(currentAnnotations, expectedAnnotations, annotationCmpOpts...) {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Spec = expected.Spec
	if updated.Annotations == nil {
		updated.Annotations = map[string]string{}
	}
	for k := range managedDNSServiceAnnotations {
		currentVal, have := current.Annotations[k]
		expectedVal, want := expected.Annotations[k]
		if want && (!have || currentVal != expectedVal) {
			updated.Annotations[k] = expected.Annotations[k]
		} else if have && !want {
			delete(updated.Annotations, k)
		}
	}

	// Preserve fields that the API, other controllers, or user may have
	// modified.
	updated.Spec.ClusterIP = current.Spec.ClusterIP

	return true, updated
}

func cmpServiceAffinity(a, b corev1.ServiceAffinity) bool {
	if len(a) == 0 {
		a = corev1.ServiceAffinityNone
	}
	if len(b) == 0 {
		b = corev1.ServiceAffinityNone
	}
	return a == b
}

func cmpServiceType(a, b corev1.ServiceType) bool {
	if len(a) == 0 {
		a = corev1.ServiceTypeClusterIP
	}
	if len(b) == 0 {
		b = corev1.ServiceTypeClusterIP
	}
	return a == b
}

func cmpServiceInternalTrafficPolicyType(a, b *corev1.ServiceInternalTrafficPolicyType) bool {
	defaultPolicy := corev1.ServiceInternalTrafficPolicyCluster
	if a == nil {
		a = &defaultPolicy
	}
	if b == nil {
		b = &defaultPolicy
	}
	return *a == *b
}
