// Package conditions is copied in from openshift/cluster-ingress-operator and should be moved to library-go to be shared.
package conditions

import (
	"fmt"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilclock "k8s.io/utils/clock"
)

// ExpectedCondition contains a condition that is expected to be checked when
// determining Available or Degraded status of the ingress controller
type ExpectedCondition struct {
	Condition string
	Status    operatorv1.ConditionStatus
	// ifConditionsTrue is a list of prerequisite conditions that should be true
	// or else the condition is not checked.
	IfConditionsTrue []string
	GracePeriod      time.Duration
}

// mergeConditions adds or updates matching conditions, and updates
// the transition time if details of a condition have changed. Returns
// the updated condition array.
func mergeConditions(conditions []operatorv1.OperatorCondition, clock utilclock.Clock, updates ...operatorv1.OperatorCondition) []operatorv1.OperatorCondition {
	now := metav1.NewTime(clock.Now())
	var additions []operatorv1.OperatorCondition
	for i, update := range updates {
		add := true
		for j, cond := range conditions {
			if cond.Type == update.Type {
				add = false
				if conditionChanged(cond, update) {
					conditions[j].Status = update.Status
					conditions[j].Reason = update.Reason
					conditions[j].Message = update.Message
					conditions[j].LastTransitionTime = now
					break
				}
			}
		}
		if add {
			updates[i].LastTransitionTime = now
			additions = append(additions, updates[i])
		}
	}
	conditions = append(conditions, additions...)
	return conditions
}

// pruneConditions removes any conditions that are not currently supported.
// Returns the updated condition array.
func pruneConditions(conditions []operatorv1.OperatorCondition) []operatorv1.OperatorCondition {
	for i, condition := range conditions {
		// PodsScheduled was removed in 4.13.0.
		// TODO: Remove this fix-up logic in 4.14.
		if condition.Type == "PodsScheduled" {
			conditions = append(conditions[:i], conditions[i+1:]...)
		}
	}
	return conditions
}

// CheckConditions compares expected operator conditions to existing operator
// conditions and returns a list of graceConditions, degradedconditions, and a
// requeueing wait time.
func CheckConditions(expectedConds []ExpectedCondition, conditions []operatorv1.OperatorCondition, clock utilclock.Clock) ([]*operatorv1.OperatorCondition, []*operatorv1.OperatorCondition, time.Duration) {
	var graceConditions, degradedConditions []*operatorv1.OperatorCondition
	var requeueAfter time.Duration
	conditionsMap := make(map[string]*operatorv1.OperatorCondition)

	for i := range conditions {
		conditionsMap[conditions[i].Type] = &conditions[i]
	}
	now := clock.Now()
	for _, expected := range expectedConds {
		condition, haveCondition := conditionsMap[expected.Condition]
		if !haveCondition {
			continue
		}
		if condition.Status == expected.Status {
			continue
		}
		failedPredicates := false
		for _, ifCond := range expected.IfConditionsTrue {
			predicate, havePredicate := conditionsMap[ifCond]
			if !havePredicate || predicate.Status != operatorv1.ConditionTrue {
				failedPredicates = true
				break
			}
		}
		if failedPredicates {
			continue
		}
		if expected.GracePeriod != 0 {
			t1 := now.Add(-expected.GracePeriod)
			t2 := condition.LastTransitionTime
			if t2.After(t1) {
				d := t2.Sub(t1)
				if len(graceConditions) == 0 || d < requeueAfter {
					// Recompute status conditions again
					// after the grace period has elapsed.
					requeueAfter = d
				}
				graceConditions = append(graceConditions, condition)
				continue
			}
		}
		degradedConditions = append(degradedConditions, condition)
	}
	return graceConditions, degradedConditions, requeueAfter
}

func FormatConditions(conditions []*operatorv1.OperatorCondition) string {
	var formatted string
	if len(conditions) == 0 {
		return ""
	}
	for _, cond := range conditions {
		formatted = formatted + fmt.Sprintf(", %s=%s (%s: %s)", cond.Type, cond.Status, cond.Reason, cond.Message)
	}
	formatted = formatted[2:]
	return formatted
}

func ConditionsEqual(a, b []operatorv1.OperatorCondition) bool {
	conditionCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b operatorv1.OperatorCondition) bool { return a.Type < b.Type }),
	}

	return cmp.Equal(a, b, conditionCmpOpts...)
}

func conditionChanged(a, b operatorv1.OperatorCondition) bool {
	return a.Status != b.Status || a.Reason != b.Reason || a.Message != b.Message
}
