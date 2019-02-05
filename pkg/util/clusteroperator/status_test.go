package clusteroperator

import (
	"testing"

	configv1 "github.com/openshift/api/config/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetStatusCondition(t *testing.T) {
	testCases := []struct {
		description   string
		oldConditions []configv1.ClusterOperatorStatusCondition
		newCondition  *configv1.ClusterOperatorStatusCondition
		expected      []configv1.ClusterOperatorStatusCondition
	}{
		{
			description: "new condition",
			newCondition: &configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorAvailable,
				Status: configv1.ConditionTrue,
			},
			expected: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
			},
		},
		{
			description: "existing condition, unchanged",
			oldConditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
			},
			newCondition: &configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorAvailable,
				Status: configv1.ConditionTrue,
			},
			expected: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
			},
		},
		{
			description: "existing conditions, one changed",
			oldConditions: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorFailing,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   configv1.OperatorProgressing,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionFalse,
				},
			},
			newCondition: &configv1.ClusterOperatorStatusCondition{
				Type:   configv1.OperatorAvailable,
				Status: configv1.ConditionTrue,
			},
			expected: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorFailing,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   configv1.OperatorProgressing,
					Status: configv1.ConditionFalse,
				},
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
			},
		},
	}

	for _, tc := range testCases {
		actual := SetStatusCondition(tc.oldConditions, tc.newCondition)
		if !ConditionsEqual(actual, tc.expected) {
			t.Fatalf("%q: expected %v, got %v", tc.description,
				tc.expected, actual)
		}
	}
}

func TestConditionsEqual(t *testing.T) {
	testCases := []struct {
		description string
		expected    bool
		a, b        []configv1.ClusterOperatorStatusCondition
	}{
		{
			description: "empty statuses should be equal",
			expected:    true,
		},
		{
			description: "condition LastTransitionTime should be ignored",
			expected:    true,
			a: []configv1.ClusterOperatorStatusCondition{
				{
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionTrue,
					LastTransitionTime: metav1.Unix(0, 0),
				},
			},
			b: []configv1.ClusterOperatorStatusCondition{
				{
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionTrue,
					LastTransitionTime: metav1.Unix(1, 0),
				},
			},
		},
		{
			description: "order of conditions should not matter",
			expected:    true,
			a: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorProgressing,
					Status: configv1.ConditionTrue,
				},
			},
			b: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorProgressing,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
			},
		},
		{
			description: "check missing condition",
			expected:    false,
			a: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorProgressing,
					Status: configv1.ConditionTrue,
				},
			},
			b: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionTrue,
				},
				{
					Type:   configv1.OperatorProgressing,
					Status: configv1.ConditionTrue,
				},
			},
		},
		{
			description: "check condition reason differs",
			expected:    false,
			a: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionFalse,
					Reason: "foo",
				},
			},
			b: []configv1.ClusterOperatorStatusCondition{
				{
					Type:   configv1.OperatorAvailable,
					Status: configv1.ConditionFalse,
					Reason: "bar",
				},
			},
		},
		{
			description: "check condition message differs",
			expected:    false,
			a: []configv1.ClusterOperatorStatusCondition{
				{
					Type:    configv1.OperatorAvailable,
					Status:  configv1.ConditionFalse,
					Message: "foo",
				},
			},
			b: []configv1.ClusterOperatorStatusCondition{
				{
					Type:    configv1.OperatorAvailable,
					Status:  configv1.ConditionFalse,
					Message: "bar",
				},
			},
		},
	}

	for _, tc := range testCases {
		actual := ConditionsEqual(tc.a, tc.b)
		if actual != tc.expected {
			t.Fatalf("%q: expected %v, got %v", tc.description,
				tc.expected, actual)
		}
	}
}

func TestVersionsEqual(t *testing.T) {
	testCases := []struct {
		description string
		expected    bool
		a, b        []configv1.OperandVersion
	}{
		{
			description: "empty slices should be equal",
			expected:    true,
		},
		{
			description: "order should not matter",
			expected:    true,
			a: []configv1.OperandVersion{
				{
					Name:    "x",
					Version: "1",
				},
				{
					Name:    "y",
					Version: "2",
				},
			},
			b: []configv1.OperandVersion{
				{
					Name:    "y",
					Version: "2",
				},
				{
					Name:    "x",
					Version: "1",
				},
			},
		},
		{
			description: "check missing operand",
			expected:    false,
			a: []configv1.OperandVersion{
				{
					Name:    "x",
					Version: "1",
				},
				{
					Name:    "y",
					Version: "2",
				},
			},
			b: []configv1.OperandVersion{
				{
					Name:    "x",
					Version: "1",
				},
			},
		},
		{
			description: "check different operand name",
			expected:    false,
			a: []configv1.OperandVersion{
				{
					Name:    "x",
					Version: "1",
				},
				{
					Name:    "y",
					Version: "2",
				},
			},
			b: []configv1.OperandVersion{
				{
					Name:    "x",
					Version: "1",
				},
				{
					Name:    "z",
					Version: "2",
				},
			},
		},
		{
			description: "check different operand version",
			expected:    false,
			a: []configv1.OperandVersion{
				{
					Name:    "x",
					Version: "1",
				},
				{
					Name:    "y",
					Version: "2",
				},
			},
			b: []configv1.OperandVersion{
				{
					Name:    "x",
					Version: "2",
				},
				{
					Name:    "y",
					Version: "2",
				},
			},
		},
		{
			description: "check extra operand",
			expected:    false,
			a: []configv1.OperandVersion{
				{
					Name:    "x",
					Version: "1",
				},
				{
					Name:    "y",
					Version: "2",
				},
			},
			b: []configv1.OperandVersion{
				{
					Name:    "y",
					Version: "2",
				},
			},
		},
	}

	for _, tc := range testCases {
		actual := VersionsEqual(tc.a, tc.b)
		if actual != tc.expected {
			t.Fatalf("%q: expected %v, got %v", tc.description,
				tc.expected, actual)
		}
	}
}

func TestObjectReferencesEqual(t *testing.T) {
	testCases := []struct {
		description string
		expected    bool
		a, b        []configv1.ObjectReference
	}{
		{
			description: "empty slices should be equal",
			expected:    true,
		},
		{
			description: "order should not matter",
			expected:    true,
			a: []configv1.ObjectReference{
				{
					Name: "x",
				},
				{
					Name: "y",
				},
			},
			b: []configv1.ObjectReference{
				{
					Name: "y",
				},
				{
					Name: "x",
				},
			},
		},
		{
			description: "check reference",
			expected:    false,
			a: []configv1.ObjectReference{
				{
					Name: "x",
				},
				{
					Name: "y",
				},
			},
			b: []configv1.ObjectReference{
				{
					Name: "x",
				},
			},
		},
		{
			description: "check different name",
			expected:    false,
			a: []configv1.ObjectReference{
				{
					Name: "x",
				},
				{
					Name: "y",
				},
			},
			b: []configv1.ObjectReference{
				{
					Name: "x",
				},
				{
					Name: "z",
				},
			},
		},
		{
			description: "check different namespace",
			expected:    false,
			a: []configv1.ObjectReference{
				{
					Name:      "x",
					Namespace: "a",
				},
				{
					Name:      "y",
					Namespace: "b",
				},
			},
			b: []configv1.ObjectReference{
				{
					Name:      "x",
					Namespace: "b",
				},
				{
					Name:      "y",
					Namespace: "b",
				},
			},
		},
		{
			description: "check different group",
			expected:    false,
			a: []configv1.ObjectReference{
				{
					Name:  "y",
					Group: "alpha",
				},
			},
			b: []configv1.ObjectReference{
				{
					Name:  "y",
					Group: "beta",
				},
			},
		},
		{
			description: "check different resource",
			expected:    false,
			a: []configv1.ObjectReference{
				{
					Name:     "x",
					Resource: "widget",
				},
				{
					Name:  "y",
					Group: "widget",
				},
			},
			b: []configv1.ObjectReference{
				{
					Name:     "x",
					Resource: "gadget",
				},
				{
					Name:  "y",
					Group: "widget",
				},
			},
		},
	}

	for _, tc := range testCases {
		actual := ObjectReferencesEqual(tc.a, tc.b)
		if actual != tc.expected {
			t.Fatalf("%q: expected %v, got %v", tc.description,
				tc.expected, actual)
		}
	}
}
