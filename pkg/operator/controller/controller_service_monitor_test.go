package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDNSServiceMonitorChanged(t *testing.T) {
	testCases := []struct {
		description string
		mutate      func(*unstructured.Unstructured)
		expect      bool
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *unstructured.Unstructured) {},
			expect:      false,
		},
		{
			description: "if spec.endpoints.scheme changes",
			mutate: func(serviceMonitor *unstructured.Unstructured) {
				serviceMonitor.Object["spec"] = map[string]interface{}{
					"selector": map[string]interface{}{},
					"endpoints": []interface{}{
						map[string]interface{}{
							"scheme": "http",
						},
					},
				}
			},
			expect: true,
		},
		{
			description: "if labels change",
			mutate: func(serviceMonitor *unstructured.Unstructured) {
				serviceMonitor.SetLabels(map[string]string{
					"test": "test",
				})
			},
			expect: false,
		},
	}

	for _, tc := range testCases {
		original := unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "openshift-dns",
					"name":      "dns-original",
				},
				"spec": map[string]interface{}{
					"selector": map[string]interface{}{},
					"endpoints": []interface{}{
						map[string]interface{}{
							"scheme": "https",
						},
					},
				},
			},
		}

		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := serviceMonitorChanged(&original, mutated); changed != tc.expect {
			t.Errorf("%s, expect serviceMonitorChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if changedAgain, _ := serviceMonitorChanged(mutated, updated); changedAgain {
				t.Errorf("%s, serviceMonitorChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
