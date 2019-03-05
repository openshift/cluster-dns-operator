package stub

import (
	"fmt"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComputeStatusConditions(t *testing.T) {
	type testInputs struct {
		haveNamespace                           bool
		numWanted, numAvailable, numUnavailable int
	}
	type testOutputs struct {
		failing, progressing, available bool
	}
	testCases := []struct {
		description string
		inputs      testInputs
		outputs     testOutputs
	}{
		{"no namespace", testInputs{false, 0, 0, 0}, testOutputs{true, false, true}},
		{"no clusterdnses, no daemonsets", testInputs{true, 0, 0, 0}, testOutputs{false, false, true}},
		{"scaling up", testInputs{true, 1, 0, 0}, testOutputs{false, true, false}},
		{"scaling down", testInputs{true, 0, 1, 0}, testOutputs{false, true, true}},
		{"0/2 daemonsets available", testInputs{true, 2, 0, 2}, testOutputs{false, false, false}},
		{"1/2 daemonsets available", testInputs{true, 2, 1, 1}, testOutputs{false, false, false}},
		{"2/2 daemonsets available", testInputs{true, 2, 2, 0}, testOutputs{false, false, true}},
	}

	for _, tc := range testCases {
		var (
			namespace    *corev1.Namespace
			clusterdnses []dnsv1alpha1.ClusterDNS
			daemonsets   []appsv1.DaemonSet

			failing, progressing, available configv1.ConditionStatus
		)
		if tc.inputs.haveNamespace {
			namespace = &corev1.Namespace{}
		}
		for i := 0; i < tc.inputs.numWanted; i++ {
			clusterdnses = append(clusterdnses,
				dnsv1alpha1.ClusterDNS{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("%d", i+1),
					},
				})
		}
		numDaemonsets := tc.inputs.numAvailable + tc.inputs.numUnavailable
		for i := 0; i < numDaemonsets; i++ {
			numberAvailable := 0
			if i < tc.inputs.numAvailable {
				numberAvailable = 1
			}
			daemonsets = append(daemonsets, appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("dns-%d", i+1),
				},
				Status: appsv1.DaemonSetStatus{
					NumberAvailable: int32(numberAvailable),
				},
			})
		}
		if tc.outputs.failing {
			failing = configv1.ConditionTrue
		} else {
			failing = configv1.ConditionFalse
		}
		if tc.outputs.progressing {
			progressing = configv1.ConditionTrue
		} else {
			progressing = configv1.ConditionFalse
		}
		if tc.outputs.available {
			available = configv1.ConditionTrue
		} else {
			available = configv1.ConditionFalse
		}
		expected := []configv1.ClusterOperatorStatusCondition{
			{
				Type:   configv1.OperatorFailing,
				Status: failing,
			},
			{
				Type:   configv1.OperatorProgressing,
				Status: progressing,
			},
			{
				Type:   configv1.OperatorAvailable,
				Status: available,
			},
		}
		new := computeStatusConditions(
			[]configv1.ClusterOperatorStatusCondition{},
			namespace,
			clusterdnses,
			daemonsets,
		)
		gotExpected := true
		if len(new) != len(expected) {
			gotExpected = false
		}
		for _, conditionA := range new {
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
			t.Fatalf("%q: expected %#v, got %#v", tc.description, expected, new)
		}
	}
}
