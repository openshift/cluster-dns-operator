package controller

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	corev1 "k8s.io/api/core/v1"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		MetricsServingCertAnnotation: DNSMetricsSecretName(dns),
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
	serviceCmpOpts := []cmp.Option{
		// Ignore fields that the API, other controllers, or user may
		// have modified.
		//
		// TODO: Remove TopologyKeys when the service topology feature gate is enabled.
		cmpopts.IgnoreFields(
			corev1.ServiceSpec{},
			"ClusterIP", "ClusterIPs",
			"IPFamilies", "IPFamilyPolicy",
			"TopologyKeys",
		),
		cmp.Comparer(cmpServiceAffinity),
		cmp.Comparer(cmpServiceType),
		cmp.Comparer(cmpServiceInternalTrafficPolicyType),
		cmpopts.EquateEmpty(),
	}

	servingCertAnnotationKey := "service.beta.openshift.io/serving-cert-secret-name"
	currentServingCertAnnotation := current.ObjectMeta.Annotations[servingCertAnnotationKey]
	expectedServingCertAnnotation := expected.ObjectMeta.Annotations[servingCertAnnotationKey]
	annotationMatches := currentServingCertAnnotation == expectedServingCertAnnotation

	if cmp.Equal(current.Spec, expected.Spec, serviceCmpOpts...) && annotationMatches {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Spec = expected.Spec
	updated.ObjectMeta.Annotations = expected.ObjectMeta.Annotations

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
