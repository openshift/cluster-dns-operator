package controller

import (
	"fmt"
	"testing"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	operatorv1 "github.com/openshift/api/operator/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDNSStatusConditions(t *testing.T) {
	type testInputs struct {
		haveClusterIP             bool
		avail, desire             int32
		haveResolverDaemonset     bool
		resolverDaemonsetHasOwner bool
	}
	type testOutputs struct {
		degraded, progressing, available, upgradeable bool
	}
	testCases := []struct {
		description string
		inputs      testInputs
		outputs     testOutputs
	}{
		{
			description: "no cluster ip, 0/0 pods available",
			inputs:      testInputs{false, 0, 0, false, false},
			outputs:     testOutputs{true, true, false, true},
		},
		{
			description: "cluster ip, 0/0 pods available",
			inputs:      testInputs{true, 0, 0, false, false},
			outputs:     testOutputs{true, false, false, true},
		},
		{
			description: "no cluster ip, 0/2 pods available",
			inputs:      testInputs{false, 0, 2, false, false},
			outputs:     testOutputs{true, true, false, true},
		},
		{
			description: "no cluster ip, 1/2 pods available",
			inputs:      testInputs{false, 1, 2, false, false},
			outputs:     testOutputs{true, true, false, true},
		},
		{
			description: "no cluster ip, 2/2 pods available",
			inputs:      testInputs{false, 2, 2, false, false},
			outputs:     testOutputs{true, true, false, true},
		},
		{
			description: "daemonset pod available on 0/0 nodes",
			inputs:      testInputs{true, 0, 0, false, false},
			outputs:     testOutputs{true, false, false, true},
		},
		{
			description: "daemonset pod available on 0/2 nodes",
			inputs:      testInputs{true, 0, 2, false, false},
			outputs:     testOutputs{true, true, false, true},
		},
		{
			description: "daemonset pod available on 1/2 nodes",
			inputs:      testInputs{true, 1, 2, false, false},
			outputs:     testOutputs{false, true, true, true},
		},
		{
			description: "have node resolver daemonset with incorrect owner",
			inputs:      testInputs{true, 2, 2, true, false},
			outputs:     testOutputs{false, false, true, false},
		},
		{
			description: "have node resolver daemonset with correct owner",
			inputs:      testInputs{true, 2, 2, true, true},
			outputs:     testOutputs{false, false, true, true},
		},
		{
			description: "daemonset pod available on 2/2 nodes",
			inputs:      testInputs{true, 2, 2, false, false},
			outputs:     testOutputs{false, false, true, true},
		},
		{
			description: "daemonset pod available on 1/3 nodes",
			inputs:      testInputs{true, 1, 3, false, false},
			outputs:     testOutputs{true, true, true, true},
		},
		{
			description: "daemonset pod available on 2/3 nodes",
			inputs:      testInputs{true, 2, 3, false, false},
			outputs:     testOutputs{false, true, true, true},
		},
		{
			description: "daemonset pod available on 0/1 nodes",
			inputs:      testInputs{true, 0, 1, false, false},
			outputs:     testOutputs{true, true, false, true},
		},
	}

	for i, tc := range testCases {
		var (
			clusterIP string

			haveResolverDaemonset bool
			resolverDaemonset     *appsv1.DaemonSet

			degraded    operatorv1.ConditionStatus
			progressing operatorv1.ConditionStatus
			available   operatorv1.ConditionStatus
			upgradeable operatorv1.ConditionStatus
		)
		if tc.inputs.haveClusterIP {
			clusterIP = "1.2.3.4"
		}
		dns := &operatorv1.DNS{
			ObjectMeta: metav1.ObjectMeta{
				Name: DefaultDNSController,
			},
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
		if tc.inputs.haveResolverDaemonset {
			haveResolverDaemonset = true
			resolverDaemonset = &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-resolver",
				},
			}
		}
		if tc.inputs.resolverDaemonsetHasOwner {
			resolverDaemonset.ObjectMeta.Labels = map[string]string{
				manifests.OwningDNSLabel: DNSDaemonSetLabel(dns),
			}
		}
		if tc.outputs.upgradeable {
			upgradeable = operatorv1.ConditionTrue
		} else {
			upgradeable = operatorv1.ConditionFalse
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
				Type:   operatorv1.OperatorStatusTypeUpgradeable,
				Status: upgradeable,
			},
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
		actual := computeDNSStatusConditions([]operatorv1.OperatorCondition{}, dns, clusterIP, ds, haveResolverDaemonset, resolverDaemonset)
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
