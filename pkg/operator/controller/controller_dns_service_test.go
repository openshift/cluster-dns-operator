package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	operatorv1 "github.com/openshift/api/operator/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDesiredDNSServiceUsesTrafficDistribution(t *testing.T) {
	dns := &operatorv1.DNS{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	service := desiredDNSService(dns, "", metav1.OwnerReference{})

	if assert.NotNil(t, service.Spec.TrafficDistribution) {
		assert.Equal(t, corev1.ServiceTrafficDistributionPreferSameNode, *service.Spec.TrafficDistribution)
	}
	assert.NotContains(t, service.Annotations, topologyAwareHintsAnnotationKey)
}

func TestDNSServiceChanged(t *testing.T) {
	testCases := []struct {
		description    string
		mutateOriginal func(*corev1.Service)
		mutate         func(*corev1.Service)
		expect         bool
		verify         func(*corev1.Service, *corev1.Service, *corev1.Service, *testing.T)
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *corev1.Service) {},
			expect:      false,
		},
		{
			description: "if .uid changes",
			mutate: func(service *corev1.Service) {
				service.UID = "2"
			},
			expect: false,
		},
		{
			description: "if .spec.selector changes",
			mutate: func(service *corev1.Service) {
				service.Spec.Selector = map[string]string{"foo": "bar"}
			},
			expect: true,
		},
		{
			description: "if .spec.type is defaulted",
			mutate: func(service *corev1.Service) {
				service.Spec.Type = corev1.ServiceTypeClusterIP
			},
			expect: false,
		},
		{
			description: "if .spec.type changes",
			mutate: func(service *corev1.Service) {
				service.Spec.Type = corev1.ServiceTypeNodePort
			},
			expect: true,
		},
		{
			description: "if .spec.sessionAffinity is defaulted",
			mutate: func(service *corev1.Service) {
				service.Spec.SessionAffinity = corev1.ServiceAffinityNone
			},
			expect: false,
		},
		{
			description: "if .spec.sessionAffinity is set to a non-default value",
			mutate: func(service *corev1.Service) {
				service.Spec.SessionAffinity = corev1.ServiceAffinityClientIP
			},
			expect: true,
		},
		{
			description: "if .spec.internalTrafficPolicy is defaulted",
			mutate: func(service *corev1.Service) {
				policy := corev1.ServiceInternalTrafficPolicyCluster
				service.Spec.InternalTrafficPolicy = &policy
			},
			expect: false,
		},
		{
			description: "if .spec.internalTrafficPolicy is set to a non-default value",
			mutate: func(service *corev1.Service) {
				policy := corev1.ServiceInternalTrafficPolicyLocal
				service.Spec.InternalTrafficPolicy = &policy
			},
			expect: true,
		},
		{
			description: "if .spec.publishNotReadyAddresses changes",
			mutate: func(service *corev1.Service) {
				service.Spec.PublishNotReadyAddresses = true
			},
			expect: true,
		},
		{
			description: "if .spec.clusterIP changes",
			mutate: func(service *corev1.Service) {
				service.Spec.ClusterIP = "1.2.3.4"
				service.Spec.ClusterIPs = []string{"1.2.3.4"}
			},
			expect: false,
		},
		{
			description: "if .spec.ipFamilies or .spec.ipFamilyPolicy change",
			mutate: func(service *corev1.Service) {
				service.Spec.IPFamilies = []corev1.IPFamily{
					corev1.IPv4Protocol,
				}
				ipFamilyPolicy := corev1.IPFamilyPolicySingleStack
				service.Spec.IPFamilyPolicy = &ipFamilyPolicy
			},
			expect: false,
		},
		{
			description: "if service.beta.openshift.io/serving-cert-secret-name annotation changes",
			mutate: func(service *corev1.Service) {
				service.ObjectMeta.Annotations = map[string]string{
					"service.beta.openshift.io/serving-cert-secret-name": "foo",
				}
			},
			expect: true,
		},
		{
			description: "if service.beta.openshift.io/serving-cert-signed-by annotation is set",
			mutate: func(service *corev1.Service) {
				service.ObjectMeta.Annotations = map[string]string{
					"service.beta.openshift.io/serving-cert-signed-by": "foo",
				}
			},
			expect: false,
		},
		{
			description: "if the legacy service.kubernetes.io/topology-aware-hints annotation is set",
			mutateOriginal: func(service *corev1.Service) {
				service.ObjectMeta.Annotations = map[string]string{
					"service.kubernetes.io/topology-aware-hints": "auto",
				}
			},
			mutate: func(service *corev1.Service) {
				delete(service.ObjectMeta.Annotations, "service.kubernetes.io/topology-aware-hints")
			},
			expect: true,
			verify: func(_, _ *corev1.Service, updated *corev1.Service, t *testing.T) {
				assert.NotContains(t, updated.Annotations, "service.kubernetes.io/topology-aware-hints")
			},
		},
		{
			description: "if dual-stack fields exist in current but not in expected, and an update is triggered",
			mutateOriginal: func(service *corev1.Service) {
				ipFamilyPolicy := corev1.IPFamilyPolicyPreferDualStack
				service.Spec.ClusterIP = "1.2.3.4"
				service.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}
				service.Spec.IPFamilyPolicy = &ipFamilyPolicy
				service.Spec.ClusterIPs = []string{"1.2.3.4", "fd00::1"}
			},
			mutate: func(service *corev1.Service) {
				service.Spec.IPFamilies = nil
				service.Spec.IPFamilyPolicy = nil
				service.Spec.ClusterIPs = nil
				service.Spec.Selector = map[string]string{"foo": "bar"}
			},
			expect: true,
			verify: func(original, mutated, updated *corev1.Service, t *testing.T) {
				assert.Equal(t, original.Spec.ClusterIP, updated.Spec.ClusterIP)
				assert.Equal(t, original.Spec.ClusterIPs, updated.Spec.ClusterIPs)
				assert.Equal(t, original.Spec.IPFamilies, updated.Spec.IPFamilies)
				if assert.NotNil(t, updated.Spec.IPFamilyPolicy) {
					assert.Equal(t, *original.Spec.IPFamilyPolicy, *updated.Spec.IPFamilyPolicy)
				}
			},
		},
	}

	for _, tc := range testCases {
		original := corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dns-original",
				Namespace: "openshift-dns",
				UID:       "1",
			},
			Spec: corev1.ServiceSpec{},
		}
		if tc.mutateOriginal != nil {
			tc.mutateOriginal(&original)
		}
		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := serviceChanged(&original, mutated); changed != tc.expect {
			t.Errorf("%s, expect serviceChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if tc.verify != nil {
				tc.verify(&original, mutated, updated, t)
			}
			if changedAgain, _ := serviceChanged(mutated, updated); changedAgain {
				t.Errorf("%s, serviceChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
