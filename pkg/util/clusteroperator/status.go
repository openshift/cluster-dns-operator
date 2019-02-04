package clusteroperator

import (
	configv1 "github.com/openshift/api/config/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetStatusCondition returns the result of setting the specified condition in
// the given slice of conditions.
func SetStatusCondition(oldConditions []configv1.ClusterOperatorStatusCondition, condition *configv1.ClusterOperatorStatusCondition) []configv1.ClusterOperatorStatusCondition {
	condition.LastTransitionTime = metav1.Now()

	newConditions := []configv1.ClusterOperatorStatusCondition{}

	found := false
	for _, c := range oldConditions {
		if condition.Type == c.Type {
			if condition.Status == c.Status &&
				condition.Reason == c.Reason &&
				condition.Message == c.Message {
				return oldConditions
			}

			found = true
			newConditions = append(newConditions, *condition)
		} else {
			newConditions = append(newConditions, c)
		}
	}
	if !found {
		newConditions = append(newConditions, *condition)
	}

	return newConditions
}

// ConditionsEqual returns true if and only if the provided slices of conditions
// (ignoring LastTransitionTime) are equal.
func ConditionsEqual(oldConditions, newConditions []configv1.ClusterOperatorStatusCondition) bool {
	if len(newConditions) != len(oldConditions) {
		return false
	}

	for _, conditionA := range oldConditions {
		foundMatchingCondition := false

		for _, conditionB := range newConditions {
			// Compare every field except LastTransitionTime.
			if conditionA.Type == conditionB.Type &&
				conditionA.Status == conditionB.Status &&
				conditionA.Reason == conditionB.Reason &&
				conditionA.Message == conditionB.Message {
				foundMatchingCondition = true
				break
			}
		}

		if !foundMatchingCondition {
			return false
		}
	}

	return true
}

// ObjectReferencesEqual returns true if and only if the provided slices of
// object references are equal.
func ObjectReferencesEqual(oldObjectReferences, newObjectReferences []configv1.ObjectReference) bool {
	if len(newObjectReferences) != len(oldObjectReferences) {
		return false
	}

	for _, refA := range oldObjectReferences {
		foundMatchingRef := false

		for _, refB := range newObjectReferences {
			if refA.Name == refB.Name &&
				refA.Namespace == refB.Namespace &&
				refA.Resource == refB.Resource &&
				refA.Group == refB.Group {
				foundMatchingRef = true
				break
			}
		}

		if !foundMatchingRef {
			return false
		}
	}

	return true
}

// VersionsEqual returns true if and only if the provided slices of operand
// versions are equal.
func VersionsEqual(oldVersions, newVersions []configv1.OperandVersion) bool {
	if len(newVersions) != len(oldVersions) {
		return false
	}

	for _, versionA := range oldVersions {
		foundMatchingVersion := false

		for _, versionB := range newVersions {
			if versionA.Name == versionB.Name &&
				versionA.Version == versionB.Version {
				foundMatchingVersion = true
				break
			}
		}

		if !foundMatchingVersion {
			return false
		}
	}

	return true
}
