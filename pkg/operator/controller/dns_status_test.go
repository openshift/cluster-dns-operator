package controller

import (
	"fmt"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	retryable "github.com/openshift/cluster-dns-operator/pkg/util/retryableerror"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilclock "k8s.io/utils/clock"
	utilclocktesting "k8s.io/utils/clock/testing"
)

var (
	maxUnavailable                 = intstr.FromInt(1)
	clock          utilclock.Clock = utilclock.RealClock{}
)

func TestDNSStatusConditions(t *testing.T) {
	type testIn struct {
		haveClusterIP                   bool
		haveDNS                         bool
		availDNS, desireDNS, updatedDNS int32
		haveNR                          bool
		availNR, desireNR, updatedNR    int32
		managementState                 operatorv1.ManagementState
	}
	type testOut struct {
		degraded, progressing, available, upgradeable bool
	}
	testCases := []struct {
		inputs  testIn
		outputs testOut
	}{
		// It is always Progressing=true and Degraded=false when cluster ip is missing.
		{testIn{false, false, 0, 0, 0, false, 0, 0, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 0, 0, 0, true, 0, 0, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 0, 0, 0, true, 0, 2, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 0, 2, 0, true, 0, 0, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 0, 2, 0, true, 0, 2, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 1, 2, 1, true, 0, 2, 1, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 0, 2, 0, true, 1, 2, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 1, 2, 1, true, 1, 2, 1, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 1, 2, 1, true, 2, 2, 1, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 2, 2, 2, true, 1, 2, 2, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{false, true, 2, 2, 2, true, 2, 2, 2, operatorv1.Managed}, testOut{false, true, false, true}},
		// It is Progressing=false and Degraded=true when desireDNS and/or desireNR are 0, and there are no availDNS.  No checks involving time in this suite.
		{testIn{true, true, 0, 0, 0, true, 0, 0, 0, operatorv1.Managed}, testOut{true, false, false, true}},
		{testIn{true, true, 0, 0, 0, true, 2, 2, 2, operatorv1.Managed}, testOut{true, false, false, true}},
		{testIn{true, true, 0, 2, 2, true, 0, 0, 0, operatorv1.Managed}, testOut{true, false, false, true}},
		{testIn{true, true, 0, 2, 2, true, 0, 0, 0, operatorv1.Managed}, testOut{true, false, false, true}},
		// It is Progressing=true and Degraded=false when updatedNR < desireNR or updatedDNS < desireDNS.
		{testIn{true, true, 0, 0, 0, true, 0, 2, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{true, true, 0, 2, 0, true, 0, 0, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{true, true, 0, 2, 0, true, 0, 2, 3, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{true, true, 0, 2, 3, true, 0, 2, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		{testIn{true, true, 1, 2, 1, true, 0, 2, 0, operatorv1.Managed}, testOut{false, true, true, true}},
		{testIn{true, true, 1, 2, 1, true, 1, 2, 1, operatorv1.Managed}, testOut{false, true, true, true}},
		{testIn{true, true, 1, 2, 1, true, 2, 2, 2, operatorv1.Managed}, testOut{false, true, true, true}},
		{testIn{true, true, 2, 2, 2, true, 0, 2, 0, operatorv1.Managed}, testOut{false, true, true, true}},
		{testIn{true, true, 1, 3, 1, true, 3, 3, 3, operatorv1.Managed}, testOut{false, true, true, true}},
		{testIn{true, true, 3, 3, 3, true, 0, 3, 0, operatorv1.Managed}, testOut{false, true, true, true}},
		{testIn{true, true, 2, 3, 2, true, 3, 3, 3, operatorv1.Managed}, testOut{false, true, true, true}},
		{testIn{true, true, 0, 1, 0, true, 0, 1, 0, operatorv1.Managed}, testOut{false, true, false, true}},
		// It is Upgradeable=false whenever managementState=Unmanaged
		{testIn{true, true, 0, 0, 0, true, 0, 2, 0, operatorv1.Unmanaged}, testOut{false, true, false, false}},
		{testIn{true, true, 1, 3, 1, true, 3, 3, 3, operatorv1.Unmanaged}, testOut{false, true, true, false}},
		{testIn{true, true, 2, 2, 2, true, 0, 2, 0, operatorv1.Unmanaged}, testOut{false, true, true, false}},
		// It is Available=false whenever availDNS=0
		{testIn{true, true, 0, 0, 0, true, 0, 2, 0, operatorv1.ManagementState("")}, testOut{false, true, false, true}},
		{testIn{true, true, 1, 1, 0, true, 0, 2, 0, operatorv1.ManagementState("")}, testOut{false, true, true, true}},
		{testIn{true, true, 0, 5, 1, true, 3, 3, 3, operatorv1.ManagementState("")}, testOut{false, true, false, true}},
		{testIn{true, true, 2, 2, 2, true, 0, 2, 0, operatorv1.ManagementState("")}, testOut{false, true, true, true}},
		{testIn{true, true, 2, 2, 1, true, 2, 2, 2, operatorv1.ManagementState("")}, testOut{false, true, true, true}},
		// It is Degraded=false, Progressing=false whenever avail=desired=updated.
		{testIn{true, true, 2, 2, 2, true, 2, 2, 2, operatorv1.Managed}, testOut{false, false, true, true}},
		{testIn{true, true, 2, 2, 2, true, 2, 2, 2, operatorv1.Unmanaged}, testOut{false, false, true, false}},
		{testIn{true, true, 2, 2, 2, true, 2, 2, 2, operatorv1.ManagementState("")}, testOut{false, false, true, true}},
		// We should never have a situation where Degraded=true and Progressing=true
	}

	for i, tc := range testCases {
		var (
			clusterIP string

			degraded, progressing, available, upgradeable operatorv1.ConditionStatus
		)
		if tc.inputs.haveClusterIP {
			clusterIP = "1.2.3.4"
		}
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
					UpdatedNumberScheduled: tc.inputs.updatedDNS,
				},
			}
			dnsDaemonset.Spec.Template.Spec.NodeSelector = nodeSelectorForDNS(&operatorv1.DNS{})
			dnsDaemonset.Spec.Template.Spec.Tolerations = tolerationsForDNS(&operatorv1.DNS{})
		}
		if tc.inputs.haveNR {
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
					UpdatedNumberScheduled: tc.inputs.updatedNR,
				},
			}
		}
		dns := operatorv1.DNS{}
		dns.Spec.ManagementState = tc.inputs.managementState
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
		if tc.outputs.upgradeable {
			upgradeable = operatorv1.ConditionTrue
		} else {
			upgradeable = operatorv1.ConditionFalse
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
			{
				Type:   operatorv1.OperatorStatusTypeUpgradeable,
				Status: upgradeable,
			},
		}
		actual, _ := computeDNSStatusConditions(&dns, clusterIP, tc.inputs.haveDNS, dnsDaemonset, tc.inputs.haveNR, nodeResolverDaemonset, 0, &reconcile.Result{})
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
		var managementState string
		switch tc.inputs.managementState {
		case operatorv1.Managed:
			managementState = "Managed"
		case operatorv1.Force:
			managementState = "Force"
		case operatorv1.Removed:
			managementState = "Removed"
		case operatorv1.Unmanaged:
			managementState = "Unmanaged"
		}
		description := fmt.Sprintf("%s, %d/%d/%d DNS pods available/updated/desired, %d/%d node-resolver pods available/desired, managementState is %s", haveClusterIP, tc.inputs.availDNS, tc.inputs.updatedDNS, tc.inputs.desireDNS, tc.inputs.availNR, tc.inputs.desireNR, managementState)
		if !gotExpected {
			t.Fatalf("%q:\nexpected %#v\ngot %#v", description, expected, actual)
		}
	}
}

// TestComputeDNSDegradedCondition verifies the computeDNSDegradedCondition has
// the expected behavior.
func TestComputeDNSDegradedCondition(t *testing.T) {
	// Inject a fake clock.
	fakeClock := utilclocktesting.NewFakeClock(time.Time{})
	clock = fakeClock
	defer func() {
		// Don't forget to reset it.
		clock = utilclock.RealClock{}
	}()

	makeDaemonSet := func(desired, available int) *appsv1.DaemonSet {
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

	degradedFalse := &operatorv1.OperatorCondition{
		Type:               operatorv1.OperatorStatusTypeDegraded,
		Status:             operatorv1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(clock.Now()),
	}
	degradedTrue := &operatorv1.OperatorCondition{
		Type:               operatorv1.OperatorStatusTypeDegraded,
		Status:             operatorv1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(clock.Now()),
	}

	progressingTrue := &operatorv1.OperatorCondition{
		Type:               operatorv1.OperatorStatusTypeProgressing,
		Status:             operatorv1.ConditionTrue,
		LastTransitionTime: metav1.NewTime(clock.Now()),
	}
	progressingFalse := &operatorv1.OperatorCondition{
		Type:               operatorv1.OperatorStatusTypeProgressing,
		Status:             operatorv1.ConditionFalse,
		LastTransitionTime: metav1.NewTime(clock.Now()),
	}

	testCases := []struct {
		name                    string
		oldDegradedCondition    *operatorv1.OperatorCondition
		newProgressingCondition *operatorv1.OperatorCondition
		clusterIP               string
		dnsDaemonset            *appsv1.DaemonSet
		nrDaemonset             *appsv1.DaemonSet
		expected                operatorv1.ConditionStatus
		// Expect a requeue when it's Progressing=false, Degraded=true,
		// and has been degraded past the grace period or may become degraded soon; OR
		// it's previously and still Degraded after the grace period.
		expectRequeue bool
		// A degraded condition will give a retry duration based on its grace period.
		expectAfter time.Duration
	}{
		{
			name:                    "0 available, previously Degraded=false and Progressing=false condition",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingFalse,
			clusterIP:               "172.30.0.10",
			dnsDaemonset:            makeDaemonSet(6, 0),
			nrDaemonset:             makeDaemonSet(6, 6),
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "0 available, previously Degraded=false and Progressing=true condition",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingTrue,
			clusterIP:               "172.30.0.10",
			dnsDaemonset:            makeDaemonSet(6, 0),
			nrDaemonset:             makeDaemonSet(6, 6),
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "no cluster ip, previously Degraded=false and Progressing=true condition",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingTrue,
			clusterIP:               "",
			dnsDaemonset:            makeDaemonSet(6, 6),
			nrDaemonset:             makeDaemonSet(6, 6),
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "no cluster ip, previously Degraded=false and Progressing=false condition",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingFalse,
			clusterIP:               "",
			dnsDaemonset:            makeDaemonSet(6, 6),
			nrDaemonset:             makeDaemonSet(6, 6),
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "no cluster ip, previously Degraded=true and Progressing=false condition",
			oldDegradedCondition:    degradedTrue,
			newProgressingCondition: progressingFalse,
			clusterIP:               "",
			dnsDaemonset:            makeDaemonSet(6, 6),
			nrDaemonset:             makeDaemonSet(6, 6),
			expectRequeue:           true,
			expected:                operatorv1.ConditionTrue,
		},
		{
			name:                    "no cluster ip, previously Degraded=true and Progressing=true condition",
			oldDegradedCondition:    degradedTrue,
			newProgressingCondition: progressingTrue,
			clusterIP:               "",
			dnsDaemonset:            makeDaemonSet(6, 6),
			nrDaemonset:             makeDaemonSet(6, 6),
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "0 available, previously Degraded=true condition",
			oldDegradedCondition:    degradedTrue,
			newProgressingCondition: progressingFalse,
			clusterIP:               "172.30.0.10",
			dnsDaemonset:            makeDaemonSet(6, 0),
			nrDaemonset:             makeDaemonSet(6, 6),
			expectRequeue:           true,
			expected:                operatorv1.ConditionTrue,
		},
		{
			name:                    "node-resolver 0 available is ok",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingFalse,
			clusterIP:               "172.30.0.10",
			dnsDaemonset:            makeDaemonSet(10, 10),
			nrDaemonset:             makeDaemonSet(6, 0),
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "both ok",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingFalse,
			clusterIP:               "172.30.0.10",
			dnsDaemonset:            makeDaemonSet(6, 6),
			nrDaemonset:             makeDaemonSet(6, 6),
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "no clusterIP, 0 available, previously Degraded=false",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingFalse,
			clusterIP:               "",
			dnsDaemonset:            makeDaemonSet(6, 0),
			nrDaemonset:             makeDaemonSet(6, 0),
			expectRequeue:           false, // Don't requeue if lastTransitionTime is recent.
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "no clusterIP, 0 available, previously Degraded=true",
			oldDegradedCondition:    degradedTrue,
			newProgressingCondition: progressingFalse,
			clusterIP:               "",
			dnsDaemonset:            makeDaemonSet(6, 0),
			nrDaemonset:             makeDaemonSet(6, 0),
			expectRequeue:           true, // Requeue while it's still degraded.
			expected:                operatorv1.ConditionTrue,
		},
		{
			name:                    "no clusterIP, previously Degraded=false",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingFalse,
			clusterIP:               "",
			dnsDaemonset:            makeDaemonSet(6, 6),
			nrDaemonset:             makeDaemonSet(6, 6),
			expectRequeue:           false, // Don't requeue if lastTransitionTime is recent.
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "no clusterIP, previously Degraded=true",
			oldDegradedCondition:    degradedTrue,
			newProgressingCondition: progressingFalse,
			clusterIP:               "",
			dnsDaemonset:            makeDaemonSet(6, 6),
			nrDaemonset:             makeDaemonSet(6, 6),
			expectRequeue:           true, // Requeue while it's still degraded.
			expected:                operatorv1.ConditionTrue,
		},
		{
			name:                    "0 desired, previously Degraded=False",
			oldDegradedCondition:    degradedFalse,
			newProgressingCondition: progressingFalse,
			clusterIP:               "172.30.0.10",
			dnsDaemonset:            makeDaemonSet(0, 0),
			nrDaemonset:             makeDaemonSet(0, 0),
			expectRequeue:           false, // Don't requeue if lastTransitionTime is recent.
			expected:                operatorv1.ConditionFalse,
		},
		{
			name:                    "0 desired, previously Degraded=true",
			oldDegradedCondition:    degradedTrue,
			newProgressingCondition: progressingFalse,
			clusterIP:               "172.30.0.10",
			dnsDaemonset:            makeDaemonSet(0, 0),
			nrDaemonset:             makeDaemonSet(0, 0),
			expectRequeue:           true, // Requeue while it's still degraded.
			expected:                operatorv1.ConditionTrue,
		},
	}

	for _, tc := range testCases {
		actual, retryErr := computeDNSDegradedCondition(tc.oldDegradedCondition, tc.newProgressingCondition, tc.clusterIP, true, tc.dnsDaemonset, 0, time.Time{})
		switch e := retryErr.(type) {
		case retryable.Error:
			if !tc.expectRequeue {
				t.Errorf("%q: expected not to be told to requeue", tc.name)
			}
			if tc.expectAfter.Seconds() != e.After().Seconds() {
				t.Errorf("%q: expected requeue after %s, got %s", tc.name, tc.expectAfter.String(), e.After().String())
			}
		case nil:
			if tc.expectRequeue {
				t.Errorf("%q: expected to be told to requeue", tc.name)
			}
		default:
			t.Errorf("%q: unexpected error: %v", tc.name, retryErr)
			continue
		}
		if actual.Status != tc.expected {
			t.Errorf("%q: expected status to be %s, got %s: %#v", tc.name, tc.expected, actual.Status, actual)
		}
	}
}

// TestComputeDNSProgressingCondition verifies the
// computeDNSProgressingCondition has the expected behavior.
func TestComputeDNSProgressingCondition(t *testing.T) {
	makeDaemonSet := func(desired, available, updated int, nodeSelector map[string]string, tolerations []corev1.Toleration) *appsv1.DaemonSet {
		return &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						NodeSelector: nodeSelector,
						Tolerations:  tolerations,
					},
				},
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
					RollingUpdate: &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: int32(desired),
				NumberAvailable:        int32(available),
				UpdatedNumberScheduled: int32(updated),
			},
		}
	}
	var (
		emptySelector      map[string]string
		emptyTolerations   []corev1.Toleration
		defaultSelector    = nodeSelectorForDNS(&operatorv1.DNS{})
		defaultTolerations = tolerationsForDNS(&operatorv1.DNS{})
		customSelector     = map[string]string{"foo": "bar"}
		customTolerations  = []corev1.Toleration{{Key: "foo"}}
	)
	testCases := []struct {
		name         string
		clusterIP    string
		dnsDaemonset *appsv1.DaemonSet
		nrDaemonset  *appsv1.DaemonSet
		nodeSelector map[string]string
		tolerations  []corev1.Toleration
		expected     operatorv1.ConditionStatus
	}{
		{
			name:         "no clusterIP",
			clusterIP:    "",
			dnsDaemonset: makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: defaultSelector,
			tolerations:  defaultTolerations,
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "0 desired",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(0, 0, 0, defaultSelector, defaultTolerations),
			nrDaemonset:  makeDaemonSet(0, 0, 0, defaultSelector, defaultTolerations),
			nodeSelector: defaultSelector,
			tolerations:  defaultTolerations,
			expected:     operatorv1.ConditionFalse,
		},
		{
			name:         "0/6 available DNS pods",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 0, 0, defaultSelector, defaultTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: defaultSelector,
			tolerations:  defaultTolerations,
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "6/6 DNS pods and 5/6 node-resolver pods available",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nrDaemonset:  makeDaemonSet(6, 5, 5, defaultSelector, defaultTolerations),
			nodeSelector: defaultSelector,
			tolerations:  defaultTolerations,
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "6/6 DNS pods and 6/6 node-resolver pods available",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: defaultSelector,
			tolerations:  defaultTolerations,
			expected:     operatorv1.ConditionFalse,
		},
		{
			name:         "6/6 DNS pods with custom node selector and tolerations",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, 6, customSelector, customTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: customSelector,
			tolerations:  customTolerations,
			expected:     operatorv1.ConditionFalse,
		},
		{
			name:         "6/6 DNS pods missing default node selector",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, 6, emptySelector, defaultTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: defaultSelector,
			tolerations:  defaultTolerations,
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "6/6 DNS pods missing default tolerations",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, 6, defaultSelector, emptyTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: defaultSelector,
			tolerations:  defaultTolerations,
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "6/6 DNS pods missing custom node selector",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, 6, defaultSelector, customTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: customSelector,
			tolerations:  customTolerations,
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "6/6 DNS pods missing custom tolerations",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, 6, customSelector, defaultTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: customSelector,
			tolerations:  customTolerations,
			expected:     operatorv1.ConditionTrue,
		},
		{
			name:         "6/6 DNS pods with 5 up-to-date",
			clusterIP:    "172.30.0.10",
			dnsDaemonset: makeDaemonSet(6, 6, 5, customSelector, defaultTolerations),
			nrDaemonset:  makeDaemonSet(6, 6, 6, defaultSelector, defaultTolerations),
			nodeSelector: customSelector,
			tolerations:  customTolerations,
			expected:     operatorv1.ConditionTrue,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldCondition := operatorv1.OperatorCondition{
				Type:   operatorv1.OperatorStatusTypeProgressing,
				Status: operatorv1.ConditionUnknown,
			}
			dns := &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					NodePlacement: operatorv1.DNSNodePlacement{
						NodeSelector: tc.nodeSelector,
						Tolerations:  tc.tolerations,
					},
				},
				Status: operatorv1.DNSStatus{
					Conditions: []operatorv1.OperatorCondition{oldCondition},
				},
			}
			var reconcileResult reconcile.Result
			actual := computeDNSProgressingCondition(&oldCondition, dns, tc.clusterIP, true, tc.dnsDaemonset, true, tc.nrDaemonset, 0, time.Time{}, &reconcileResult)
			if actual.Status != tc.expected {
				t.Errorf("%q: expected status to be %s, got %s: %#v", tc.name, tc.expected, actual.Status, actual)
			}
		})
	}
}

func TestSkippingStatusUpdates(t *testing.T) {
	makeDaemonSet := func(desired, available, updated int) *appsv1.DaemonSet {
		return &appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						NodeSelector: nodeSelectorForDNS(&operatorv1.DNS{}),
						Tolerations:  tolerationsForDNS(&operatorv1.DNS{}),
					},
				},
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
					RollingUpdate: &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: &maxUnavailable,
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: int32(desired),
				NumberAvailable:        int32(available),
				UpdatedNumberScheduled: int32(updated),
			},
		}
	}
	testCases := []struct {
		name                 string
		clusterIP            string
		dnsDaemonset         *appsv1.DaemonSet
		nrDaemonset          *appsv1.DaemonSet
		oldCondition         operatorv1.OperatorCondition
		progressingCondition operatorv1.OperatorCondition
		currentTime          time.Time
		toleration           time.Duration
		expected             operatorv1.ConditionStatus
		reconcileResult      reconcile.Result
	}{
		{
			name:         "would return Progressing=ConditionTrue, but Progressing was set to false within tolerated duration, so returns Progressing=ConditionFalse",
			clusterIP:    "1.2.3.4",
			dnsDaemonset: makeDaemonSet(6, 1, 1),
			nrDaemonset:  makeDaemonSet(6, 6, 6),
			oldCondition: operatorv1.OperatorCondition{
				Type:               operatorv1.OperatorStatusTypeProgressing,
				Status:             operatorv1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(time.Date(2022, time.Month(5), 19, 1, 0, 0, 0, time.UTC)),
			},
			currentTime: time.Date(2022, time.Month(5), 19, 1, 10, 0, 0, time.UTC),
			// last-curr = 10m, tolerate 1h, so should prevent the flap.
			toleration: 1 * time.Hour,
			expected:   operatorv1.ConditionFalse,
			reconcileResult: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 1 * time.Hour,
			},
		},
		{
			name:         "there is a clusterIP, and time toleration doesn't matter, should return Degraded=ConditionFalse",
			clusterIP:    "1.2.3.4",
			dnsDaemonset: makeDaemonSet(6, 5, 6),
			nrDaemonset:  makeDaemonSet(6, 6, 6),
			oldCondition: operatorv1.OperatorCondition{
				Type:   operatorv1.OperatorStatusTypeDegraded,
				Status: operatorv1.ConditionFalse,
			},
			progressingCondition: operatorv1.OperatorCondition{
				Type:   operatorv1.OperatorStatusTypeProgressing,
				Status: operatorv1.ConditionFalse,
			},
			expected: operatorv1.ConditionFalse,
		},
		{
			name:         "should return Degraded=ConditionTrue because enough time has elapsed for an update",
			clusterIP:    "1.2.3.4",
			dnsDaemonset: makeDaemonSet(6, 0, 6),
			nrDaemonset:  makeDaemonSet(6, 6, 6),
			oldCondition: operatorv1.OperatorCondition{
				Type:               operatorv1.OperatorStatusTypeDegraded,
				Status:             operatorv1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(time.Date(2022, time.Month(5), 12, 1, 10, 50, 0, time.UTC)),
			},
			progressingCondition: operatorv1.OperatorCondition{
				Type:   operatorv1.OperatorStatusTypeProgressing,
				Status: operatorv1.ConditionFalse,
			},
			currentTime: time.Date(2022, time.Month(5), 19, 1, 10, 50, 0, time.UTC),
			// last - current = 1w, and we tolerate 5s, so this should change from False to True.
			toleration: 5 * time.Second,
			expected:   operatorv1.ConditionTrue,
			reconcileResult: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
			},
		},
		{
			name:         "should return Degraded=ConditionFalse because not enough time has elapsed for an update",
			clusterIP:    "1.2.3.4",
			dnsDaemonset: makeDaemonSet(6, 0, 6),
			nrDaemonset:  makeDaemonSet(6, 6, 6),
			oldCondition: operatorv1.OperatorCondition{
				Type:               operatorv1.OperatorStatusTypeDegraded,
				Status:             operatorv1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(time.Date(2022, time.Month(5), 12, 1, 10, 50, 0, time.UTC)),
			},
			currentTime: time.Date(2022, time.Month(5), 12, 1, 10, 52, 0, time.UTC),
			// last - current = 2s, and we tolerate 5s, so this should not change from False.
			toleration: 5 * time.Second,
			expected:   operatorv1.ConditionFalse,
		},
		{
			name:         "should return Degraded=ConditionFalse because it is Progressing",
			clusterIP:    "1.2.3.4",
			dnsDaemonset: makeDaemonSet(6, 0, 6),
			nrDaemonset:  makeDaemonSet(6, 6, 6),
			oldCondition: operatorv1.OperatorCondition{
				Type:               operatorv1.OperatorStatusTypeDegraded,
				Status:             operatorv1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(time.Date(2022, time.Month(5), 12, 1, 10, 50, 0, time.UTC)),
			},
			progressingCondition: operatorv1.OperatorCondition{
				Type:   operatorv1.OperatorStatusTypeProgressing,
				Status: operatorv1.ConditionTrue,
			},
			currentTime: time.Date(2022, time.Month(5), 19, 1, 10, 50, 0, time.UTC),
			toleration:  5 * time.Second,
			expected:    operatorv1.ConditionFalse,
		},
		{
			name:         "would return Degraded=ConditionTrue, but Degraded was set to false within tolerated duration, so returns Degraded=ConditionFalse",
			clusterIP:    "1.2.3.4",
			dnsDaemonset: makeDaemonSet(6, 1, 6),
			nrDaemonset:  makeDaemonSet(6, 6, 6),
			oldCondition: operatorv1.OperatorCondition{
				Type:               operatorv1.OperatorStatusTypeDegraded,
				Status:             operatorv1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(time.Date(2022, time.Month(5), 19, 1, 9, 50, 0, time.UTC)),
			},
			currentTime: time.Date(2022, time.Month(5), 19, 1, 10, 50, 0, time.UTC),
			// last-curr = 1m, tolerate 2m, so should prevent the flap.
			toleration: 2 * time.Minute,
			expected:   operatorv1.ConditionFalse,
		},
		{
			name:         "should return Degraded=ConditionTrue, because Degraded was set to false before the tolerated interval",
			clusterIP:    "1.2.3.4",
			dnsDaemonset: makeDaemonSet(6, 1, 6),
			nrDaemonset:  makeDaemonSet(6, 6, 6),
			oldCondition: operatorv1.OperatorCondition{
				Type:               operatorv1.OperatorStatusTypeDegraded,
				Status:             operatorv1.ConditionFalse,
				LastTransitionTime: metav1.NewTime(time.Date(2022, time.Month(5), 19, 1, 9, 50, 0, time.UTC)),
			},
			currentTime: time.Date(2022, time.Month(5), 19, 1, 11, 50, 0, time.UTC),
			// last-curr = 2m, so change to Degraded=ConditionTrue is correct.
			toleration: 1 * time.Minute,
			expected:   operatorv1.ConditionTrue,
			reconcileResult: reconcile.Result{
				Requeue:      true,
				RequeueAfter: 1 * time.Minute,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dns := &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					NodePlacement: operatorv1.DNSNodePlacement{
						NodeSelector: nodeSelectorForDNS(&operatorv1.DNS{}),
						Tolerations:  tolerationsForDNS(&operatorv1.DNS{}),
					},
				},
				Status: operatorv1.DNSStatus{
					Conditions: []operatorv1.OperatorCondition{tc.oldCondition},
				},
			}

			var actual operatorv1.OperatorCondition
			var actualReconcileResult reconcile.Result
			var retryErr error
			if tc.oldCondition.Type == operatorv1.OperatorStatusTypeProgressing {
				actual = computeDNSProgressingCondition(&tc.oldCondition, dns, tc.clusterIP, true, tc.dnsDaemonset, true, tc.nrDaemonset, tc.toleration, tc.currentTime, &actualReconcileResult)
				if actualReconcileResult != tc.reconcileResult {
					t.Errorf("%q: expected requeue to be %+v, got %+v", tc.name, tc.reconcileResult, actualReconcileResult)
				}
			} else {
				actual, retryErr = computeDNSDegradedCondition(&tc.oldCondition, &tc.progressingCondition, tc.clusterIP, true, tc.dnsDaemonset, tc.toleration, tc.currentTime)
				switch e := retryErr.(type) {
				case retryable.Error:
					if !tc.reconcileResult.Requeue {
						t.Errorf("%q: expected not to be told to requeue", tc.name)
					}
					if tc.reconcileResult.RequeueAfter.Seconds() != e.After().Seconds() {
						t.Errorf("%q: expected requeue after %s, got %s", tc.name, tc.reconcileResult.RequeueAfter.String(), e.After().String())
					}
				case nil:
					if tc.reconcileResult.Requeue {
						t.Errorf("%q: expected to be told to requeue", tc.name)
					}
				default:
					t.Errorf("%q: unexpected error: %v", tc.name, retryErr)
				}
			}
			if actual.Status != tc.expected {
				t.Errorf("%q: expected status to be %s, got %s: %#v", tc.name, tc.expected, actual.Status, actual)
			}
		})
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

func TestTransitionTimeIsRecent(t *testing.T) {
	testCases := []struct {
		description string
		currTime    time.Time
		prevTime    time.Time
		toleration  time.Duration
		isRecent    bool
	}{
		{
			description: "recent: because elapsed time is exactly 0s",
			currTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 0, time.UTC),
			prevTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 0, time.UTC),
			toleration:  0 * time.Second,
			isRecent:    true,
		},
		{
			description: "not recent: because toleration is negative",
			currTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 0, time.UTC),
			prevTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 0, time.UTC),
			toleration:  -1 * time.Second,
			isRecent:    false,
		},
		{
			description: "not recent: elapsed time 10s but will only tolerate 9s",
			currTime:    time.Date(2022, time.Month(5), 19, 1, 10, 20, 0, time.UTC),
			prevTime:    time.Date(2022, time.Month(5), 19, 1, 10, 10, 0, time.UTC),
			toleration:  9 * time.Second,
			isRecent:    false,
		},
		{
			description: "recent: elapsed time 9s and will tolerate 9s",
			currTime:    time.Date(2022, time.Month(5), 19, 1, 10, 20, 0, time.UTC),
			prevTime:    time.Date(2022, time.Month(5), 19, 1, 10, 11, 0, time.UTC),
			toleration:  9 * time.Second,
			isRecent:    true,
		},
		{
			description: "recent: elapsed time is 0s 2ns and will tolerate 2ns",
			currTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 2, time.UTC),
			prevTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 0, time.UTC),
			toleration:  2 * time.Nanosecond,
			isRecent:    true,
		},
		{
			description: "not recent: elapsed time is 0s 2ns but will only tolerate 1ns",
			currTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 2, time.UTC),
			prevTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 0, time.UTC),
			toleration:  1 * time.Nanosecond,
			isRecent:    false,
		},
		{
			description: "recent: elapsed time is 1h and will tolerate 60 mins",
			currTime:    time.Date(2022, time.Month(5), 19, 2, 10, 00, 0, time.UTC),
			prevTime:    time.Date(2022, time.Month(5), 19, 1, 10, 00, 0, time.UTC),
			toleration:  1 * time.Hour,
			isRecent:    true,
		},
		{
			description: "not recent: currTime is before lastTime",
			currTime:    time.Date(2022, time.Month(4), 19, 1, 10, 42, 0, time.UTC),
			prevTime:    time.Date(2022, time.Month(5), 19, 1, 10, 20, 0, time.UTC),
			toleration:  lameDuckDuration,
			isRecent:    false,
		},
		{
			description: "not recent: a month ago",
			currTime:    time.Date(2022, time.Month(5), 19, 1, 10, 42, 0, time.UTC),
			prevTime:    time.Date(2022, time.Month(4), 19, 1, 10, 20, 0, time.UTC),
			toleration:  lameDuckDuration,
			isRecent:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actual := lastTransitionTimeIsRecent(tc.currTime, tc.prevTime, tc.toleration)
			if actual != tc.isRecent {
				t.Errorf("expected to be %v, got %v", tc.isRecent, actual)
			}
		})
	}
}
