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
	type testInputs struct {
		haveClusterIP bool
		avail, desire int32
	}
	type testOutputs struct {
		degraded, progressing, available bool
	}
	testCases := []struct {
		description string
		inputs      testInputs
		outputs     testOutputs
	}{
		{
			description: "no cluster ip, 0/0 pods available",
			inputs:      testInputs{false, 0, 0},
			outputs:     testOutputs{true, true, false},
		},
		{
			description: "cluster ip, 0/0 pods available",
			inputs:      testInputs{true, 0, 0},
			outputs:     testOutputs{true, false, false},
		},
		{
			description: "no cluster ip, 0/2 pods available",
			inputs:      testInputs{false, 0, 2},
			outputs:     testOutputs{true, true, false},
		},
		{
			description: "no cluster ip, 1/2 pods available",
			inputs:      testInputs{false, 1, 2},
			outputs:     testOutputs{true, true, false},
		},
		{
			description: "no cluster ip, 2/2 pods available",
			inputs:      testInputs{false, 2, 2},
			outputs:     testOutputs{true, true, false},
		},
		{
			description: "daemonset pod available on 0/0 nodes",
			inputs:      testInputs{true, 0, 0},
			outputs:     testOutputs{true, false, false},
		},
		{
			description: "daemonset pod available on 0/2 nodes",
			inputs:      testInputs{true, 0, 2},
			outputs:     testOutputs{true, true, false},
		},
		{
			description: "daemonset pod available on 1/2 nodes",
			inputs:      testInputs{true, 1, 2},
			outputs:     testOutputs{false, true, true},
		},
		{
			description: "daemonset pod available on 2/2 nodes",
			inputs:      testInputs{true, 2, 2},
			outputs:     testOutputs{false, false, true},
		},
		{
			description: "daemonset pod available on 1/3 nodes",
			inputs:      testInputs{true, 1, 3},
			outputs:     testOutputs{true, true, true},
		},
		{
			description: "daemonset pod available on 2/3 nodes",
			inputs:      testInputs{true, 2, 3},
			outputs:     testOutputs{false, true, true},
		},
		{
			description: "daemonset pod available on 0/1 nodes",
			inputs:      testInputs{true, 0, 1},
			outputs:     testOutputs{true, true, false},
		},
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
		ds := &appsv1.DaemonSet{
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
				DesiredNumberScheduled: tc.inputs.desire,
				NumberAvailable:        tc.inputs.avail,
			},
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
		actual := computeDNSStatusConditions([]operatorv1.OperatorCondition{}, clusterIP, ds)
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
		if !gotExpected {
			t.Fatalf("%q:\nexpected %#v\ngot %#v", tc.description,
				expected, actual)
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
		name      string
		clusterIP string
		ds        *appsv1.DaemonSet
		expected  operatorv1.ConditionStatus
	}{
		{
			name:      "invalid MaxUnavailable (string with digits without a percent sign)",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(6, 6, intstr.IntOrString{Type: intstr.String, StrVal: "10"}),
			expected:  operatorv1.ConditionUnknown,
		},
		{
			name:      "invalid MaxUnavailable (string with letters)",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(6, 6, intstr.IntOrString{Type: intstr.String, StrVal: "TEST"}),
			expected:  operatorv1.ConditionUnknown,
		},
		{
			name:      "no clusterIP, 0 available",
			clusterIP: "",
			ds:        makeDaemonSet(6, 0, intstr.FromString("10%")),
			expected:  operatorv1.ConditionTrue,
		},
		{
			name:      "no clusterIP",
			clusterIP: "",
			ds:        makeDaemonSet(6, 6, intstr.FromString("10%")),
			expected:  operatorv1.ConditionTrue,
		},
		{
			name:      "0 desired",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(0, 0, intstr.FromString("10%")),
			expected:  operatorv1.ConditionTrue,
		},
		{
			name:      "0 available",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(6, 0, intstr.FromString("10%")),
			expected:  operatorv1.ConditionTrue,
		},
		{
			name:      "too few available (percentage)",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(100, 80, intstr.FromString("10%")),
			expected:  operatorv1.ConditionTrue,
		},
		{
			name:      "too few available (integer)",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(6, 4, intstr.FromInt(1)),
			expected:  operatorv1.ConditionTrue,
		},
		{
			name:      "enough available (percentage)",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(100, 90, intstr.FromString("10%")),
			expected:  operatorv1.ConditionFalse,
		},
		{
			name:      "enough available (integer)",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(6, 5, intstr.FromInt(1)),
			expected:  operatorv1.ConditionFalse,
		},
		{
			name:      "all available",
			clusterIP: "172.30.0.10",
			ds:        makeDaemonSet(6, 6, intstr.FromString("10%")),
			expected:  operatorv1.ConditionFalse,
		},
	}

	for _, tc := range testCases {
		oldCondition := &operatorv1.OperatorCondition{
			Type:   operatorv1.OperatorStatusTypeDegraded,
			Status: operatorv1.ConditionUnknown,
		}
		actual := computeDNSDegradedCondition(oldCondition, tc.clusterIP, tc.ds)
		if actual.Status != tc.expected {
			t.Errorf("%q: expected status to be %s, got %s", tc.name, tc.expected, actual.Status)
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
