package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"

	operatorv1 "github.com/openshift/api/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// syncDNSStatus computes the current status of dns and
// updates status upon any changes since last sync.
func (r *reconciler) syncDNSStatus(dns *operatorv1.DNS, clusterIP, clusterDomain string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet) error {
	updated := dns.DeepCopy()
	updated.Status.ClusterIP = clusterIP
	updated.Status.ClusterDomain = clusterDomain
	updated.Status.Conditions = computeDNSStatusConditions(dns.Status.Conditions, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset)
	if !dnsStatusesEqual(updated.Status, dns.Status) {
		if err := r.client.Status().Update(context.TODO(), updated); err != nil {
			return fmt.Errorf("failed to update dns status: %v", err)
		}
		logrus.Infof("updated DNS %s status: old: %#v, new: %#v", dns.ObjectMeta.Name, dns.Status, updated.Status)
	}

	return nil
}

// computeDNSStatusConditions computes dns status conditions based on
// the status of ds and clusterIP.
func computeDNSStatusConditions(oldConditions []operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet) []operatorv1.OperatorCondition {
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
		computeDNSDegradedCondition(oldDegradedCondition, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset),
		computeDNSProgressingCondition(oldProgressingCondition, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset),
		computeDNSAvailableCondition(oldAvailableCondition, clusterIP, haveDNSDaemonset, dnsDaemonset),
	}

	return conditions
}

// computeDNSDegradedCondition computes the dns Degraded status condition
// based on the status of clusterIP and the DNS and node-resolver daemonsets.
func computeDNSDegradedCondition(oldCondition *operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet) operatorv1.OperatorCondition {
	degradedCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeDegraded,
	}
	status := operatorv1.ConditionUnknown
	degradedReasons := []string{}
	messages := []string{}
	if len(clusterIP) == 0 {
		status = operatorv1.ConditionTrue
		degradedReasons = append(degradedReasons, "NoService")
		messages = append(messages, "No IP address is assigned to the DNS service.")
	}
	if !haveDNSDaemonset {
		status = operatorv1.ConditionTrue
		degradedReasons = append(degradedReasons, "NoDNSDaemonSet")
		messages = append(messages, "The DNS daemonset does not exist.")
	} else {
		want := dnsDaemonset.Status.DesiredNumberScheduled
		have := dnsDaemonset.Status.NumberAvailable
		numberUnavailable := want - have
		maxUnavailableIntStr := intstr.FromInt(1)
		if dnsDaemonset.Spec.UpdateStrategy.RollingUpdate != nil && dnsDaemonset.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
			maxUnavailableIntStr = *dnsDaemonset.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable
		}
		maxUnavailable, intstrErr := intstr.GetScaledValueFromIntOrPercent(&maxUnavailableIntStr, int(want), true)
		switch {
		case want == 0:
			status = operatorv1.ConditionTrue
			degradedReasons = append(degradedReasons, "NoDNSPodsDesired")
			messages = append(messages, "No DNS pods are desired; this could mean all nodes are tainted or unschedulable.")
		case have == 0:
			status = operatorv1.ConditionTrue
			degradedReasons = append(degradedReasons, "NoDNSPodsAvailable")
			messages = append(messages, "No DNS pods are available.")
		case intstrErr != nil:
			degradedReasons = append(degradedReasons, "InvalidDNSMaxUnavailable")
			messages = append(messages, fmt.Sprintf("The DNS daemonset has an invalid MaxUnavailable value: %v", intstrErr))
		case int(numberUnavailable) > maxUnavailable:
			status = operatorv1.ConditionTrue
			degradedReasons = append(degradedReasons, "MaxUnavailableDNSPodsExceeded")
			messages = append(messages, fmt.Sprintf("Too many DNS pods are unavailable (%d > %d max unavailable).", numberUnavailable, maxUnavailable))
		}
	}
	if !haveNodeResolverDaemonset {
		status = operatorv1.ConditionTrue
		degradedReasons = append(degradedReasons, "NoNodeResolverDaemonSet")
		messages = append(messages, "The node-resolver daemonset does not exist.")
	} else {
		want := nodeResolverDaemonset.Status.DesiredNumberScheduled
		have := nodeResolverDaemonset.Status.NumberAvailable
		numberUnavailable := want - have
		maxUnavailableIntStr := intstr.FromInt(1)
		if nodeResolverDaemonset.Spec.UpdateStrategy.RollingUpdate != nil && nodeResolverDaemonset.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
			maxUnavailableIntStr = *nodeResolverDaemonset.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable
		}
		maxUnavailable, intstrErr := intstr.GetScaledValueFromIntOrPercent(&maxUnavailableIntStr, int(want), true)
		switch {
		case want == 0:
			status = operatorv1.ConditionTrue
			degradedReasons = append(degradedReasons, "NoNodeResolverPodsDesired")
			messages = append(messages, "No node-resolver pods are desired; this could mean all nodes are tainted or unschedulable.")
		case have == 0:
			status = operatorv1.ConditionTrue
			degradedReasons = append(degradedReasons, "NoNodeResolverPodsAvailable")
			messages = append(messages, "No node-resolver pods are available.")
		case intstrErr != nil:
			degradedReasons = append(degradedReasons, "InvalidNodeResolverMaxUnavailable")
			messages = append(messages, fmt.Sprintf("The node-resolver daemonset has an invalid MaxUnavailable value: %v", intstrErr))
		case int(numberUnavailable) > maxUnavailable:
			status = operatorv1.ConditionTrue
			degradedReasons = append(degradedReasons, "MaxUnavailableNodeResolverPodsExceeded")
			messages = append(messages, fmt.Sprintf("Too many node-resolver pods are unavailable (%d > %d max unavailable).", numberUnavailable, maxUnavailable))
		}
	}
	if len(degradedReasons) != 0 {
		degradedCondition.Status = status
		degradedCondition.Reason = strings.Join(degradedReasons, "")
		degradedCondition.Message = strings.Join(messages, "\n")
	} else {
		degradedCondition.Status = operatorv1.ConditionFalse
		degradedCondition.Reason = "AsExpected"
		degradedCondition.Message = "Enough DNS and node-resolver pods are available, and the DNS service has a cluster IP address."
	}

	return setDNSLastTransitionTime(degradedCondition, oldCondition)
}

// computeDNSProgressingCondition computes the dns Progressing status condition
// based on the status of the DNS and node-resolver daemonsets.
func computeDNSProgressingCondition(oldCondition *operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet) operatorv1.OperatorCondition {
	progressingCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeProgressing,
	}
	messages := []string{}
	if len(clusterIP) == 0 {
		messages = append(messages, "No IP address is assigned to the DNS service.")
	}
	if !haveDNSDaemonset {
		messages = append(messages, "The DNS daemonset does not exist.")
	} else {
		have := dnsDaemonset.Status.NumberAvailable
		want := dnsDaemonset.Status.DesiredNumberScheduled
		if have != want {
			messages = append(messages, fmt.Sprintf("Have %d available DNS pods, want %d.", have, want))
		}
	}
	if !haveNodeResolverDaemonset {
		messages = append(messages, "The node-resolver daemonset does not exist.")
	} else {
		have := nodeResolverDaemonset.Status.NumberAvailable
		want := nodeResolverDaemonset.Status.DesiredNumberScheduled
		if have != want {
			messages = append(messages, fmt.Sprintf("Have %d available node-resolver pods, want %d.", have, want))
		}
	}
	if len(messages) != 0 {
		progressingCondition.Status = operatorv1.ConditionTrue
		progressingCondition.Reason = "Reconciling"
		progressingCondition.Message = strings.Join(messages, "\n")
	} else {
		progressingCondition.Status = operatorv1.ConditionFalse
		progressingCondition.Reason = "AsExpected"
		progressingCondition.Message = "All DNS and node-resolver pods are available, and the DNS service has a cluster IP address."
	}

	return setDNSLastTransitionTime(progressingCondition, oldCondition)
}

// computeDNSAvailableCondition computes the dns Available status condition
// based on the status of clusterIP and the DNS and node-resolver daemonsets.
func computeDNSAvailableCondition(oldCondition *operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet) operatorv1.OperatorCondition {
	availableCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeAvailable,
	}
	unavailableReasons := []string{}
	messages := []string{}
	if !haveDNSDaemonset {
		unavailableReasons = append(unavailableReasons, "NoDaemonSet")
		messages = append(messages, "The DNS daemonset does not exist.")
	} else if dnsDaemonset.Status.NumberAvailable == 0 {
		unavailableReasons = append(unavailableReasons, "NoDaemonSetPods")
		messages = append(messages, "The DNS daemonset has no pods available.")
	}
	if len(clusterIP) == 0 {
		unavailableReasons = append(unavailableReasons, "NoService")
		messages = append(messages, "The DNS service has no cluster IP address.")
	}
	if len(unavailableReasons) != 0 {
		availableCondition.Status = operatorv1.ConditionFalse
		availableCondition.Reason = strings.Join(unavailableReasons, "")
		availableCondition.Message = strings.Join(messages, "\n")
	} else {
		availableCondition.Status = operatorv1.ConditionTrue
		availableCondition.Reason = "AsExpected"
		availableCondition.Message = "The DNS daemonset has available pods, and the DNS service has a cluster IP address."
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
