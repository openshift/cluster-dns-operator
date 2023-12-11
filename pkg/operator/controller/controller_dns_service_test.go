package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"

	operatorv1 "github.com/openshift/api/operator/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			description: "if service.kubernetes.io/topology-aware-hints annotation is set",
			mutate: func(service *corev1.Service) {
				service.ObjectMeta.Annotations = map[string]string{
					"service.kubernetes.io/topology-aware-hints": "auto",
				}
			},
			expect: true,
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

func Test_shouldEnableTopologyAwareHints(t *testing.T) {
	emptyLabels := map[string]string{}
	someCPU := map[corev1.ResourceName]resource.Quantity{
		"cpu": resource.MustParse("1m"),
	}
	noCPU := map[corev1.ResourceName]resource.Quantity{
		"cpu": resource.MustParse("0"),
	}
	readyConditions := []corev1.NodeCondition{{
		Type:   "Ready",
		Status: "True",
	}}
	notReadyConditions := []corev1.NodeCondition{{
		Type:   "Ready",
		Status: "False",
	}}
	zone1Label := map[string]string{"topology.kubernetes.io/zone": "z1"}
	zone2Label := map[string]string{"topology.kubernetes.io/zone": "z2"}
	zone3Label := map[string]string{"topology.kubernetes.io/zone": "z3"}
	zone1AndControlPlaneLabels := map[string]string{
		"topology.kubernetes.io/zone":           "z1",
		"node-role.kubernetes.io/control-plane": "",
	}
	zone2AndControlPlaneLabels := map[string]string{
		"topology.kubernetes.io/zone":           "z2",
		"node-role.kubernetes.io/control-plane": "",
	}
	node := func(name string, labels map[string]string, allocatableResources corev1.ResourceList, conditions []corev1.NodeCondition) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
				Name:   name,
			},
			Status: corev1.NodeStatus{
				Allocatable: allocatableResources,
				Conditions:  conditions,
			},
		}
	}
	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expect          bool
	}{
		{
			name:            "no nodes",
			existingObjects: []runtime.Object{},
			expect:          false,
		},
		{
			name: "1/1 nodes labeled",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
			},
			expect: false,
		},
		{
			name: "0/3 nodes labeled",
			existingObjects: []runtime.Object{
				node("n1", emptyLabels, someCPU, readyConditions),
				node("n2", emptyLabels, someCPU, readyConditions),
				node("n3", emptyLabels, someCPU, readyConditions),
			},
			expect: false,
		},
		{
			name: "1/3 nodes labeled",
			existingObjects: []runtime.Object{
				node("n1", emptyLabels, someCPU, readyConditions),
				node("n2", zone1Label, someCPU, readyConditions),
				node("n3", emptyLabels, someCPU, readyConditions),
			},
			expect: false,
		},
		{
			name: "2/3 nodes labeled",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
				node("n2", zone2Label, someCPU, readyConditions),
				node("n3", emptyLabels, someCPU, readyConditions),
			},
			expect: false,
		},
		{
			name: "2/2 nodes labeled, but they're in the same zone",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
				node("n2", zone1Label, someCPU, readyConditions),
			},
			expect: false,
		},
		{
			name: "2/2 nodes labeled, and they're in different zones",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
				node("n2", zone2Label, someCPU, readyConditions),
			},
			expect: true,
		},
		{
			name: "3/3 nodes labeled in 2 zones",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
				node("n2", zone2Label, someCPU, readyConditions),
				node("n3", zone2Label, someCPU, readyConditions),
			},
			expect: true,
		},
		{
			name: "3/3 nodes labeled in 3 zones",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
				node("n2", zone2Label, someCPU, readyConditions),
				node("n3", zone3Label, someCPU, readyConditions),
			},
			expect: true,
		},
		{
			name: "3/3 nodes labeled but 1 node has no CPU",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
				node("n2", zone2Label, noCPU, readyConditions),
				node("n3", zone3Label, someCPU, readyConditions),
			},
			expect: false,
		},
		{
			name: "2/2 nodes labeled but 1 node is not ready",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
				node("n2", zone2Label, someCPU, notReadyConditions),
			},
			expect: false,
		},
		{
			name: "3/3 nodes labeled but 1 node is not ready",
			existingObjects: []runtime.Object{
				node("n1", zone1Label, someCPU, readyConditions),
				node("n2", zone2Label, someCPU, notReadyConditions),
				node("n3", zone3Label, someCPU, readyConditions),
			},
			expect: true,
		},
		{
			name: "3/3 nodes labeled but they are all control-plane nodes",
			existingObjects: []runtime.Object{
				node("n1", zone1AndControlPlaneLabels, someCPU, readyConditions),
				node("n2", zone2AndControlPlaneLabels, someCPU, readyConditions),
				node("n3", zone2AndControlPlaneLabels, someCPU, readyConditions),
			},
			expect: false,
		},
	}

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tc.existingObjects...).
				Build()
			informer := informertest.FakeInformers{Scheme: scheme}
			cache := fakeCache{Informers: &informer, Reader: fakeClient}
			reconciler := &reconciler{cache: cache}
			dns := operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{Name: "default"},
			}
			result, err := reconciler.shouldEnableTopologyAwareHints(&dns)
			assert.NoError(t, err)
			assert.Equal(t, tc.expect, result)
		})
	}
}

type fakeCache struct {
	cache.Informers
	client.Reader
}
