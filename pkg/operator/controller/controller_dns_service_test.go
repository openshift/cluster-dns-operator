package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDNSServiceChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(*corev1.Service)
		expect      bool
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
		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := serviceChanged(&original, mutated); changed != tc.expect {
			t.Errorf("%s, expect serviceChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if changedAgain, _ := serviceChanged(mutated, updated); changedAgain {
				t.Errorf("%s, serviceChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
