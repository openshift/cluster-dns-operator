// Package conditions is copied in from openshift/cluster-ingress-operator and should be moved to library-go to be shared.
package conditions

import (
	"fmt"

	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

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
