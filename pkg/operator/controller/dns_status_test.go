package controller

import (
	"fmt"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDNSStatusConditions(t *testing.T) {
	type testIn struct {
		haveClusterIP       bool
		haveDNS             bool
		availDNS, desireDNS int32
		haveNR              bool
		availNR, desireNR   int32
	}
	type testOut struct {
		degraded, progressing, available bool
	}
	testCases := []struct {
		inputs  testIn
		outputs testOut
	}{
		{testIn{false, false, 0, 0, false, 0, 0}, testOut{true, true, false}},
		{testIn{false, true, 0, 0, true, 0, 0}, testOut{true, true, false}},
		{testIn{false, true, 0, 0, true, 0, 2}, testOut{true, true, false}},
		{testIn{false, true, 0, 2, true, 0, 0}, testOut{true, true, false}},
		{testIn{false, true, 0, 2, true, 0, 2}, testOut{true, true, false}},
		{testIn{false, true, 1, 2, true, 0, 2}, testOut{true, true, false}},
		{testIn{false, true, 0, 2, true, 1, 2}, testOut{true, true, false}},
		{testIn{false, true, 1, 2, true, 1, 2}, testOut{true, true, false}},
		{testIn{false, true, 1, 2, true, 2, 2}, testOut{true, true, false}},
		{testIn{false, true, 2, 2, true, 1, 2}, testOut{true, true, false}},
		{testIn{false, true, 2, 2, true, 2, 2}, testOut{true, true, false}},
		{testIn{true, true, 0, 0, true, 0, 0}, testOut{true, false, false}},
		{testIn{true, true, 0, 0, true, 0, 2}, testOut{true, true, false}},
		{testIn{true, true, 0, 2, true, 0, 0}, testOut{true, true, false}},
		{testIn{true, true, 0, 2, true, 0, 2}, testOut{true, true, false}},
		{testIn{true, true, 0, 2, true, 1, 2}, testOut{true, true, false}},
		{testIn{true, true, 1, 2, true, 0, 2}, testOut{true, true, true}},
		{testIn{true, true, 1, 2, true, 1, 2}, testOut{false, true, true}},
		{testIn{true, true, 1, 2, true, 2, 2}, testOut{false, true, true}},
		{testIn{true, true, 2, 2, true, 1, 2}, testOut{false, true, true}},
		{testIn{true, true, 2, 2, true, 2, 2}, testOut{false, false, true}},
		{testIn{true, true, 1, 3, true, 3, 3}, testOut{true, true, true}},
		{testIn{true, true, 3, 3, true, 1, 3}, testOut{true, true, true}},
		{testIn{true, true, 2, 3, true, 3, 3}, testOut{false, true, true}},
		{testIn{true, true, 0, 1, true, 0, 1}, testOut{true, true, false}},
	}

	for i, tc := range testCases {
		var (
			clusterIP string

			degraded, progressing, available operatorv1.ConditionStatus
		)
		if tc.inputs.haveClusterIP {
			clusterIP = "1.2.3.4"
		}
		maxUnavailable := intstr.FromInt(1)
		var dnsDaemonset, nodeResolverDaemonset *appsv1.DaemonSet
		if tc.inputs.haveDNS {
			dnsDaemonset = &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("dns-%d", i+1),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						RollingUpdate: &appsv1.RollingUpdateDaemonSet{
							MaxUnavailable: &maxUnavailable,
						},
					},
				},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: tc.inputs.desireDNS,
					NumberAvailable:        tc.inputs.availDNS,
				},
			}
		}
		if tc.inputs.haveDNS {
			nodeResolverDaemonset = &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("node-resolver-%d", i+1),
				},
				Spec: appsv1.DaemonSetSpec{
					UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
						RollingUpdate: &appsv1.RollingUpdateDaemonSet{
							MaxUnavailable: &maxUnavailable,
						},
					},
				},
				Status: appsv1.DaemonSetStatus{
					DesiredNumberScheduled: tc.inputs.desireNR,
					NumberAvailable:        tc.inputs.availNR,
				},
			}
		}
		if tc.outputs.degraded {
			degraded = operatorv1.ConditionTrue
		} else {
			degraded = operatorv1.ConditionFalse
		}
		if tc.outputs.progressing {
			progressing = operatorv1.ConditionTrue
		} else {
			progressing = operatorv1.ConditionFalse
		}
		if tc.outputs.available {
			available = operatorv1.ConditionTrue
		} else {
			available = operatorv1.ConditionFalse
		}
		expected := []operatorv1.OperatorCondition{
			{
				Type:   operatorv1.OperatorStatusTypeDegraded,
				Status: degraded,
			},
			{
				Type:   operatorv1.OperatorStatusTypeProgressing,
				Status: progressing,
			},
			{
				Type:   operatorv1.OperatorStatusTypeAvailable,
				Status: available,
			},
		}
		actual := computeDNSStatusConditions([]operatorv1.OperatorCondition{}, clusterIP, tc.inputs.haveDNS, dnsDaemonset, tc.inputs.haveNR, nodeResolverDaemonset)
		gotExpected := true
		if len(actual) != len(expected) {
			gotExpected = false
		}
		for _, conditionA := range actual {
			foundMatchingCondition := false

			for _, conditionB := range expected {
				if conditionA.Type == conditionB.Type &&
					conditionA.Status == conditionB.Status {
					foundMatchingCondition = true
					break
				}
			}

			if !foundMatchingCondition {
				gotExpected = false
			}
		}
		haveClusterIP := "cluster ip"
		if !tc.inputs.haveClusterIP {
			haveClusterIP = "no cluster ip"
		}
		description := fmt.Sprintf("%s, %d/%d DNS pods available, %d/%d node-resolver pods available", haveClusterIP, tc.inputs.availDNS, tc.inputs.desireDNS, tc.inputs.availNR, tc.inputs.desireNR)
		if !gotExpected {
			t.Fatalf("%q:\nexpected %#v\ngot %#v", description, expected, actual)
		}
	}
}

// TestComputeDNSDegradedCondition verifies the computeDNSDegradedCondition has
// the expected behavior.
func TestComputeDNSDegradedCondition(t *testing.T) {
	makeDaemonSet := func(desired, available int, maxUnavailable intstr.IntOrString) *appsv1.DaemonSet {
		return &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
					RollingUpdate: &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: int32(desired),
				NumberAvailable:        int32(available),
			},
		}
	}
	testCases := []struct {
		name         string
		clusterIP    string
		dnsDaemonset *appsv1.DaemonSet
		nrDaemonset  *appsv1.DaemonSet
		expected     operatorv1.ConditionStatus
	}{
		{
			name:         "0 available, DNS invalid MaxUnavailable",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 0, intstr.FromString("TEST")),
			nrDaemonset:  makeDaemonSet(6, 0, intstr.FromString("10%")),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "0 available, node-resolver invalid MaxUnavailable",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 0, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(6, 0, intstr.FromString("TEST")),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "DNS invalid MaxUnavailable (string with digits without a percent sign)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, intstr.IntOrString{Type: intstr.String, StrVal: "10"}),
			nrDaemonset:  makeDaemonSet(6, 6, intstr.FromString("10%")),
			expected:     operatorv1.ConditionUnknown,
		},
		{
			name:         "node-resolver invalid MaxUnavailable (string with digits without a percent sign)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(6, 6, intstr.IntOrString{Type: intstr.String, StrVal: "10"}),
			expected:     operatorv1.ConditionUnknown,
		},
		{
			name:         "DNS invalid MaxUnavailable (string with letters)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, intstr.IntOrString{Type: intstr.String, StrVal: "TEST"}),
			nrDaemonset:  makeDaemonSet(6, 6, intstr.FromString("10%")),
			expected:     operatorv1.ConditionUnknown,
		},
		{
			name:         "node-resolver invalid MaxUnavailable (string with letters)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(6, 6, intstr.IntOrString{Type: intstr.String, StrVal: "TEST"}),
			expected:     operatorv1.ConditionUnknown,
		},
		{
			name:         "no clusterIP, 0 available",
			clusterIP:    "",
			dnsDaemonset: makeDaemonSet(6, 0, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(6, 0, intstr.FromString("10%")),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "no clusterIP",
			clusterIP:    "",
			dnsDaemonset: makeDaemonSet(6, 6, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(6, 6, intstr.FromString("10%")),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "0 desired",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(0, 0, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(0, 0, intstr.FromString("10%")),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "0 available",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 0, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(6, 0, intstr.FromString("10%")),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "too few pods DNS pods available (percentage)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(100, 80, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(100, 100, intstr.FromString("10%")),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "too few pods node-resolver pods available (percentage)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(100, 100, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(100, 80, intstr.FromString("10%")),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "too few DNS pods available (integer)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 4, intstr.FromInt(1)),
			nrDaemonset:  makeDaemonSet(6, 6, intstr.FromInt(1)),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "too few node-resolver pods available (integer)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, intstr.FromInt(1)),
			nrDaemonset:  makeDaemonSet(6, 4, intstr.FromInt(1)),
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "enough available (percentage)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(100, 90, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(100, 90, intstr.FromString("10%")),
			expected:     operatorv1.ConditionFalse,
		},
		{
			name:         "enough available (integer)",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 5, intstr.FromInt(1)),
			nrDaemonset:  makeDaemonSet(6, 5, intstr.FromInt(1)),
			expected:     operatorv1.ConditionFalse,
		},
		{
			name:         "all available",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, intstr.FromString("10%")),
			nrDaemonset:  makeDaemonSet(6, 6, intstr.FromString("10%")),
			expected:     operatorv1.ConditionFalse,
		},
	}

	for _, tc := range testCases {
		oldCondition := &operatorv1.OperatorCondition{
			Type:   operatorv1.OperatorStatusTypeDegraded,
			Status: operatorv1.ConditionUnknown,
		}
		actual := computeDNSDegradedCondition(oldCondition, tc.clusterIP, true, tc.dnsDaemonset, true, tc.nrDaemonset)
		if actual.Status != tc.expected {
			t.Errorf("%q: expected status to be %s, got %s: %#v", tc.name, tc.expected, actual.Status, actual)
		}
	}
}

func TestDNSStatusesEqual(t *testing.T) {
	testCases := []struct {
		description string
		expected    bool
		a, b        operatorv1.DNSStatus
	}{
		{
			description: "nil and non-nil slices are equal",
			expected:    true,
			a: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{},
			},
		},
		{
			description: "empty slices should be equal",
			expected:    true,
			a: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{},
			},
			b: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{},
			},
		},
		{
			description: "condition LastTransitionTime should not be ignored",
			expected:    false,
			a: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:               operatorv1.OperatorStatusTypeAvailable,
						Status:             operatorv1.ConditionTrue,
						LastTransitionTime: metav1.Unix(0, 0),
					},
				},
			},
			b: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:               operatorv1.OperatorStatusTypeAvailable,
						Status:             operatorv1.ConditionTrue,
						LastTransitionTime: metav1.Unix(1, 0),
					},
				},
			},
		},
		{
			description: "condition differs",
			expected:    false,
			a: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:   operatorv1.OperatorStatusTypeAvailable,
						Status: operatorv1.ConditionTrue,
					},
				},
			},
			b: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:   operatorv1.OperatorStatusTypeDegraded,
						Status: operatorv1.ConditionTrue,
					},
				},
			},
		},
		{
			description: "check condition reason differs",
			expected:    false,
			a: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:   operatorv1.OperatorStatusTypeAvailable,
						Status: operatorv1.ConditionFalse,
						Reason: "foo",
					},
				},
			},
			b: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:   operatorv1.OperatorStatusTypeAvailable,
						Status: operatorv1.ConditionFalse,
						Reason: "bar",
					},
				},
			},
		},
		{
			description: "check condition message differs",
			expected:    false,
			a: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:    operatorv1.OperatorStatusTypeAvailable,
						Status:  operatorv1.ConditionFalse,
						Message: "foo",
					},
				},
			},
			b: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:    operatorv1.OperatorStatusTypeAvailable,
						Status:  operatorv1.ConditionFalse,
						Message: "bar",
					},
				},
			},
		},
		{
			description: "condition status differs",
			expected:    false,
			a: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:   operatorv1.OperatorStatusTypeProgressing,
						Status: operatorv1.ConditionTrue,
					},
				},
			},
			b: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:   operatorv1.OperatorStatusTypeProgressing,
						Status: operatorv1.ConditionFalse,
					},
				},
			},
		},
		{
			description: "check duplicate with single condition",
			expected:    false,
			a: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:    operatorv1.OperatorStatusTypeAvailable,
						Message: "foo",
					},
				},
			},
			b: operatorv1.DNSStatus{
				Conditions: []operatorv1.OperatorCondition{
					{
						Type:    operatorv1.OperatorStatusTypeAvailable,
						Message: "foo",
					},
					{
						Type:    operatorv1.OperatorStatusTypeAvailable,
						Message: "foo",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		if actual := dnsStatusesEqual(tc.a, tc.b); actual != tc.expected {
			t.Fatalf("%q: expected %v, got %v", tc.description, tc.expected, actual)
		}
	}
}
