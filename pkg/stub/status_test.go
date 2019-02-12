package stub

import (
	"fmt"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/util/clusteroperator"

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
			t.Fatalf("%q: expected %#v, got %#v", tc.description,
				expected, new)
		}
	}
}

func TestComputeStatusVersions(t *testing.T) {
	type testInput struct {
		operatorVersion string

		daemonsets []appsv1.DaemonSet
	}
	type testOutput struct {
		versions []configv1.OperandVersion
	}
	ds := func(numAvailable int, coreDNSVersion, openshiftCLIVersion string) appsv1.DaemonSet {
		return appsv1.DaemonSet{
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Image: coreDNSVersion,
							},
							{
								Image: openshiftCLIVersion,
							},
						},
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				NumberAvailable: int32(numAvailable),
			},
		}
	}
	versionMap := map[string]string{
		"ps1": "0.0.0_version_coredns",
		"ps2": "0.0.1_version_coredns",
		"ps3": "0.0.0_version_origin-cli",
		"ps4": "0.0.10_version_origin-cli",
	}
	testCases := []struct {
		description string
		inputs      testInput
		output      testOutput
	}{
		{
			description: "nothing",
			inputs: testInput{
				operatorVersion: "",
				daemonsets:      []appsv1.DaemonSet{},
			},
			output: testOutput{[]configv1.OperandVersion{}},
		},
		{
			description: "no daemonsets",
			inputs: testInput{
				operatorVersion: "operatorversion",
				daemonsets:      []appsv1.DaemonSet{},
			},
			output: testOutput{[]configv1.OperandVersion{
				{Name: "operator", Version: "operatorversion"},
			}},
		},
		{
			description: "1 unavailable",
			inputs: testInput{
				operatorVersion: "operatorversion",
				daemonsets: []appsv1.DaemonSet{
					ds(0, "ps1", "ps3"),
				}},
			output: testOutput{[]configv1.OperandVersion{
				{Name: "operator", Version: "operatorversion"},
			}},
		},
		{
			description: "1 available",
			inputs: testInput{
				operatorVersion: "operatorversion",
				daemonsets: []appsv1.DaemonSet{
					ds(1, "ps1", "ps3"),
				},
			},
			output: testOutput{[]configv1.OperandVersion{
				{Name: "operator", Version: "operatorversion"},
				{Name: "coredns", Version: versionMap["ps1"]},
				{Name: "node-resolver", Version: versionMap["ps3"]},
			}},
		},
		{
			description: "1 available, 1 updating",
			inputs: testInput{
				operatorVersion: "operatorversion",
				daemonsets: []appsv1.DaemonSet{
					ds(1, "ps1", "ps3"),
					ds(0, "ps2", "ps4"),
				},
			},
			output: testOutput{[]configv1.OperandVersion{
				{Name: "operator", Version: "operatorversion"},
				{Name: "coredns", Version: versionMap["ps1"]},
				{Name: "node-resolver", Version: versionMap["ps3"]},
			}},
		},
		{
			description: "1 available, 1 updated",
			inputs: testInput{
				operatorVersion: "operatorversion",
				daemonsets: []appsv1.DaemonSet{
					ds(1, "ps1", "ps3"),
					ds(1, "ps2", "ps4"),
				},
			},
			output: testOutput{[]configv1.OperandVersion{
				{Name: "operator", Version: "operatorversion"},
				{Name: "coredns", Version: versionMap["ps1"]},
				{Name: "node-resolver", Version: versionMap["ps3"]},
			}},
		},
		{
			description: "1 unavailable, 2 updated",
			inputs: testInput{
				operatorVersion: "operatorversion",
				daemonsets: []appsv1.DaemonSet{
					ds(0, "ps1", "ps3"),
					ds(1, "ps2", "ps4"),
					ds(1, "ps2", "ps4"),
				},
			},
			output: testOutput{[]configv1.OperandVersion{
				{Name: "operator", Version: "operatorversion"},
				{Name: "coredns", Version: versionMap["ps2"]},
				{Name: "node-resolver", Version: versionMap["ps4"]},
			}},
		},
		{
			description: "2 available and updated",
			inputs: testInput{
				operatorVersion: "operatorversion",
				daemonsets: []appsv1.DaemonSet{
					ds(1, "ps2", "ps4"),
					ds(1, "ps2", "ps4"),
				},
			},
			output: testOutput{[]configv1.OperandVersion{
				{Name: "operator", Version: "operatorversion"},
				{Name: "coredns", Version: versionMap["ps2"]},
				{Name: "node-resolver", Version: versionMap["ps4"]},
			}},
		},
	}

	for _, tc := range testCases {
		versions, err := computeStatusVersions(tc.inputs.operatorVersion, tc.inputs.daemonsets, versionMap)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.description, err)
			continue
		}
		if !clusteroperator.VersionsEqual(versions, tc.output.versions) {
			t.Errorf("%q: expected %#v, got %#v", tc.description, tc.output.versions, versions)
		}
	}
}
