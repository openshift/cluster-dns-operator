package controller

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	operatorv1 "github.com/openshift/api/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// syncDNSStatus computes the current status of dns and
// updates status upon any changes since last sync.
func (r *reconciler) syncDNSStatus(dns *operatorv1.DNS, clusterIP, clusterDomain string, ds *appsv1.DaemonSet) error {
	updated := dns.DeepCopy()
	updated.Status.ClusterIP = clusterIP
	updated.Status.ClusterDomain = clusterDomain
	updated.Status.Conditions = r.computeDNSStatusConditions(dns.Status.Conditions, clusterIP, ds)
	if !dnsStatusesEqual(updated.Status, dns.Status) {
		if err := r.client.Status().Update(context.TODO(), updated); err != nil {
			return fmt.Errorf("failed to update dns status: %v", err)
		}
	}

	return nil
}

// computeDNSStatusConditions computes dns status conditions based on
// the status of ds and clusterIP.
func (r *reconciler) computeDNSStatusConditions(oldConditions []operatorv1.OperatorCondition, clusterIP string,
	ds *appsv1.DaemonSet) []operatorv1.OperatorCondition {
	var oldDegradedCondition, oldProgressingCondition, oldAvailableCondition *operatorv1.OperatorCondition
	for i := range oldConditions {
		switch oldConditions[i].Type {
		case operatorv1.OperatorStatusTypeDegraded:
			oldDegradedCondition = &oldConditions[i]
		case operatorv1.OperatorStatusTypeProgressing:
			oldProgressingCondition = &oldConditions[i]
		case operatorv1.OperatorStatusTypeAvailable:
			oldAvailableCondition = &oldConditions[i]
		}
	}

	conditions := []operatorv1.OperatorCondition{
		computeDNSDegradedCondition(oldDegradedCondition, clusterIP, ds),
		computeDNSProgressingCondition(oldProgressingCondition, ds),
		computeDNSAvailableCondition(oldAvailableCondition, clusterIP, ds),
	}

	return conditions
}

// computeDNSDegradedCondition computes the dns Degraded status condition
// based on the status of clusterIP and ds.
func computeDNSDegradedCondition(oldCondition *operatorv1.OperatorCondition, clusterIP string,
	ds *appsv1.DaemonSet) operatorv1.OperatorCondition {
	degradedCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeDegraded,
	}
	switch {
	case len(clusterIP) == 0 && ds.Status.NumberAvailable == 0:
		degradedCondition.Status = operatorv1.ConditionTrue
		degradedCondition.Reason = "NoClusterIPAndDaemonSet"
		degradedCondition.Message = "No ClusterIP assigned to DNS Service and no DaemonSet pods running"
	case len(clusterIP) == 0:
		degradedCondition.Status = operatorv1.ConditionTrue
		degradedCondition.Reason = "NoClusterIP"
		degradedCondition.Message = "No ClusterIP assigned to DNS Service"
	case ds.Status.NumberAvailable == 0:
		degradedCondition.Status = operatorv1.ConditionTrue
		degradedCondition.Reason = "DaemonSetUnavailable"
		degradedCondition.Message = "DaemonSet pod not running on any Nodes"
	case ds.Status.NumberAvailable != ds.Status.DesiredNumberScheduled:
		degradedCondition.Status = operatorv1.ConditionTrue
		degradedCondition.Reason = "DaemonSetDegraded"
		degradedCondition.Message = "Not all Nodes running DaemonSet pod"
	default:
		degradedCondition.Status = operatorv1.ConditionFalse
		degradedCondition.Reason = "AsExpected"
		degradedCondition.Message = "ClusterIP assigned to DNS Service and minimum DaemonSet pods running"
	}

	return setDNSLastTransitionTime(degradedCondition, oldCondition)
}

// computeDNSProgressingCondition computes the dns Progressing status condition
// based on the status of ds.
func computeDNSProgressingCondition(oldCondition *operatorv1.OperatorCondition, ds *appsv1.DaemonSet) operatorv1.OperatorCondition {
	progressingCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeProgressing,
	}
	if ds.Status.NumberAvailable == ds.Status.DesiredNumberScheduled {
		progressingCondition.Status = operatorv1.ConditionFalse
		progressingCondition.Reason = "AsExpected"
		progressingCondition.Message = "All expected Nodes running DaemonSet pod"
	} else {
		progressingCondition.Status = operatorv1.ConditionTrue
		progressingCondition.Reason = "Reconciling"
		progressingCondition.Message = fmt.Sprintf("%d Nodes running a DaemonSet pod, want %d",
			ds.Status.NumberAvailable, ds.Status.DesiredNumberScheduled)
	}

	return setDNSLastTransitionTime(progressingCondition, oldCondition)
}

// computeDNSAvailableCondition computes the dns Available status condition
// based on the status of clusterIP and ds.
func computeDNSAvailableCondition(oldCondition *operatorv1.OperatorCondition, clusterIP string, ds *appsv1.DaemonSet) operatorv1.OperatorCondition {
	availableCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeAvailable,
	}
	switch {
	case len(clusterIP) == 0 && ds.Status.NumberAvailable == 0:
		availableCondition.Status = operatorv1.ConditionFalse
		availableCondition.Reason = "NoClusterIPAndDaemonSet"
		availableCondition.Message = "No ClusterIP assigned to DNS Service and no running DaemonSet pods"
	case len(clusterIP) == 0:
		availableCondition.Status = operatorv1.ConditionFalse
		availableCondition.Reason = "NoDNSService"
		availableCondition.Message = "No ClusterIP assigned to DNS Service"
	case ds.Status.NumberAvailable == 0:
		availableCondition.Status = operatorv1.ConditionFalse
		availableCondition.Reason = "DaemonSetUnavailable"
		availableCondition.Message = "DaemonSet pod not running on any Nodes"
	default:
		availableCondition.Status = operatorv1.ConditionTrue
		availableCondition.Reason = "AsExpected"
		availableCondition.Message = "Minimum number of Nodes running DaemonSet pod"
	}

	return setDNSLastTransitionTime(availableCondition, oldCondition)
}

// setDNSLastTransitionTime sets LastTransitionTime for the given condition.
// If the condition has changed, it will assign a new timestamp otherwise keeps the old timestamp.
func setDNSLastTransitionTime(condition, oldCondition *operatorv1.OperatorCondition) operatorv1.OperatorCondition {
	if oldCondition != nil && condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason && condition.Message == oldCondition.Message {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	} else {
		condition.LastTransitionTime = metav1.Now()
	}

	return *condition
}

// dnsStatusesEqual compares two DNSStatus values.  Returns true
// if the provided values should be considered equal for the purpose of determining
// whether an update is necessary, false otherwise.
func dnsStatusesEqual(a, b operatorv1.DNSStatus) bool {
	conditionCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b operatorv1.OperatorCondition) bool { return a.Type < b.Type }),
	}
	if !cmp.Equal(a.Conditions, b.Conditions, conditionCmpOpts...) {
		return false
	}
	if a.ClusterIP != b.ClusterIP {
		return false
	}
	if a.ClusterDomain != b.ClusterDomain {
		return false
	}

	return true
}
