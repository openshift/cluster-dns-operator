package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	cond "github.com/openshift/cluster-dns-operator/pkg/util/conditions"
	retryable "github.com/openshift/cluster-dns-operator/pkg/util/retryableerror"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// syncDNSStatus computes the current status of dns and
// updates status upon any changes since last sync.
// If the elapsed time between time.Now() and
// oldCondition.LastTransitionTime is <= transitionUnchangedToleration
// for progressing and degraded then consider oldCondition to be recent
// and return oldCondition to prevent frequent updates.
func (r *reconciler) syncDNSStatus(dns *operatorv1.DNS, clusterIP, clusterDomain string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet, transitionUnchangedToleration time.Duration, reconcileResult *reconcile.Result) error {
	var errs []error
	updated := dns.DeepCopy()
	updated.Status.ClusterIP = clusterIP
	updated.Status.ClusterDomain = clusterDomain
	// This can return a retryable error.
	statusConds, err := computeDNSStatusConditions(dns, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset, transitionUnchangedToleration, reconcileResult)
	if err != nil {
		logrus.Infof("error computing DNS %s status: %v got %v", dns.ObjectMeta.Name, statusConds, err)
		errs = append(errs, err)
		return retryable.NewMaybeRetryableAggregate(errs)
	}

	updated.Status.Conditions = statusConds
	if !dnsStatusesEqual(updated.Status, dns.Status) {
		if err := r.client.Status().Update(context.TODO(), updated); err != nil {
			return fmt.Errorf("failed to update dns status: %v", err)
		}
		logrus.Infof("updated DNS %s status: old: %#v, new: %#v", dns.ObjectMeta.Name, dns.Status, updated.Status)
	}

	return nil
}

// computeDNSStatusConditions computes dns status conditions based on
// the status of the dns and node-resolver daemonsets and clusterIP.
// If the elapsed time between time.Now() and
// oldCondition.LastTransitionTime is <= transitionUnchangedToleration
// for progressing and degraded then consider oldCondition to be recent
// and return oldCondition to prevent frequent updates.
func computeDNSStatusConditions(dns *operatorv1.DNS, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet, transitionUnchangedToleration time.Duration, reconcileResult *reconcile.Result) ([]operatorv1.OperatorCondition, error) {
	oldConditions := dns.Status.Conditions
	var oldDegradedCondition, oldProgressingCondition, oldAvailableCondition, oldUpgradeableCondition *operatorv1.OperatorCondition
	for i := range oldConditions {
		switch oldConditions[i].Type {
		case operatorv1.OperatorStatusTypeDegraded:
			oldDegradedCondition = &oldConditions[i]
		case operatorv1.OperatorStatusTypeProgressing:
			oldProgressingCondition = &oldConditions[i]
		case operatorv1.OperatorStatusTypeAvailable:
			oldAvailableCondition = &oldConditions[i]
		case operatorv1.OperatorStatusTypeUpgradeable:
			oldUpgradeableCondition = &oldConditions[i]
		}
	}

	now := time.Now()
	var conditions []operatorv1.OperatorCondition
	// If the operator is currently Progressing=true, we may not want to mark it Degraded=true.
	newProgressingCondition := computeDNSProgressingCondition(oldProgressingCondition, dns, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset, transitionUnchangedToleration, now, reconcileResult)
	conditions = append(conditions, newProgressingCondition)
	conditions = append(conditions, computeDNSAvailableCondition(oldAvailableCondition, clusterIP, haveDNSDaemonset, dnsDaemonset))
	conditions = append(conditions, computeDNSUpgradeableCondition(oldUpgradeableCondition, dns))
	// Store the error from computeDNSDegradedCondition for use in retries by caller.
	degradedCondition, err := computeDNSDegradedCondition(oldDegradedCondition, &newProgressingCondition, clusterIP, haveDNSDaemonset, dnsDaemonset, transitionUnchangedToleration, now)
	conditions = append(conditions, degradedCondition)

	return conditions, err
}

const (
	// DNSNoService indicates that no IP address is assigned to the DNS service.
	DNSNoService = "NoService"

	// DNSNoDNSDaemonSet indicates that the DNS daemon set doesn't exist.
	DNSNoDNSDaemonSet = "NoDNSDaemonSet"

	// DNSNoDNSPodsDesired indicates that no DNS pods are desired; this could mean
	// all nodes are tainted or unschedulable.
	DNSNoDNSPodsDesired = "NoDNSPodsDesired"

	// DNSNoDNSPodsAvailable indicates that no DNS pods are available.
	DNSNoDNSPodsAvailable = "NoDNSPodsAvailable"

	// DNSInvalidDNSMaxUnavailable indicates that the DNS daemonset has an invalid
	// MaxUnavailable value configured.
	DNSInvalidDNSMaxUnavailable = "InvalidDNSMaxUnavailable"

	// DNSMaxUnavailableDNSPodsExceeded indicates that the number of unavailable DNS
	// pods is greater than the configured MaxUnavailable.
	DNSMaxUnavailableDNSPodsExceeded = "MaxUnavailableDNSPodsExceeded"
)

// computeDNSDegradedCondition computes the dns Degraded status
// condition based on the status of clusterIP and the DNS daemonset.
// The node-resolver daemonset is not a part of the calculation of
// degraded condition. If the elapsed time between currentTime and
// oldCondition.LastTransitionTime is <= transitionUnchangedToleration
// then consider oldCondition to be recent and return oldCondition to
// prevent frequent updates.
func computeDNSDegradedCondition(oldDegradedCondition, newProgressingCondition *operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, transitionUnchangedToleration time.Duration, currentTime time.Time) (operatorv1.OperatorCondition, error) {
	degradedCondition := operatorv1.OperatorCondition{
		Type:   operatorv1.OperatorStatusTypeDegraded,
		Status: operatorv1.ConditionUnknown,
	}
	var degradedConditions []*operatorv1.OperatorCondition

	transitionTime := metav1.NewTime(currentTime)

	if len(clusterIP) == 0 {
		degradedConditions = append(degradedConditions, generateCondition(DNSNoService, DNSNoService, "No IP address is assigned to the DNS service.", operatorv1.ConditionTrue, transitionTime))
	}
	if !haveDNSDaemonset {
		degradedConditions = append(degradedConditions, generateCondition(DNSNoDNSDaemonSet, DNSNoDNSDaemonSet, "The DNS daemonset does not exist.", operatorv1.ConditionTrue, transitionTime))
	} else {
		want := dnsDaemonset.Status.DesiredNumberScheduled
		have := dnsDaemonset.Status.NumberAvailable
		numberUnavailable := want - have
		maxUnavailableIntStr := intstr.FromString("10%")
		maxUnavailable, intstrErr := intstr.GetScaledValueFromIntOrPercent(&maxUnavailableIntStr, int(want), true)

		switch {
		case want == 0:
			degradedConditions = append(degradedConditions, generateCondition(DNSNoDNSPodsDesired, DNSNoDNSPodsDesired, "No DNS pods are desired; this could mean all nodes are tainted or unschedulable.", operatorv1.ConditionTrue, transitionTime))
		case have == 0:
			degradedConditions = append(degradedConditions, generateCondition(DNSNoDNSPodsAvailable, DNSNoDNSPodsAvailable, "No DNS pods are available.", operatorv1.ConditionTrue, transitionTime))
		case intstrErr != nil:
			// This should not happen, but is included just to safeguard against future changes.
			degradedConditions = append(degradedConditions, generateCondition(DNSInvalidDNSMaxUnavailable, DNSInvalidDNSMaxUnavailable, fmt.Sprintf("The DNS daemonset has an invalid MaxUnavailable value: %v", intstrErr), operatorv1.ConditionTrue, transitionTime))
		case int(numberUnavailable) > maxUnavailable:
			degradedConditions = append(degradedConditions, generateCondition(DNSMaxUnavailableDNSPodsExceeded, DNSMaxUnavailableDNSPodsExceeded, fmt.Sprintf("Too many DNS pods are unavailable (%d > %d max unavailable).", numberUnavailable, maxUnavailable), operatorv1.ConditionTrue, transitionTime))
		}
	}

	// Record whether the operator is Progressing.
	progressing := newProgressingCondition != nil && newProgressingCondition.Status == operatorv1.ConditionTrue

	if len(degradedConditions) != 0 {
		// if the last status was set to false within the last transitionUnchangedToleration, skip the new update
		// to prevent frequent status flaps, and try to keep the long-lasting state (i.e. Degraded=False). See https://bugzilla.redhat.com/show_bug.cgi?id=2037190.
		if oldDegradedCondition != nil && oldDegradedCondition.Status == operatorv1.ConditionFalse && lastTransitionTimeIsRecent(currentTime, oldDegradedCondition.LastTransitionTime.Time, transitionUnchangedToleration) {
			return *oldDegradedCondition, nil
		}
	}

	var err error

	if !progressing && len(degradedConditions) != 0 {
		degradedMessages := cond.FormatConditions(degradedConditions)
		degradedCondition.Status = operatorv1.ConditionTrue
		degradedCondition.Reason = "DegradedConditions"
		degradedCondition.Message = "One or more other status conditions indicate a degraded state: " + degradedMessages
		err = retryable.New(errors.New("DNS operator is degraded"), transitionUnchangedToleration)
	} else {
		degradedCondition.Status = operatorv1.ConditionFalse
		degradedCondition.Reason = "AsExpected"
		degradedCondition.Message = "Enough DNS pods are available, and the DNS service has a cluster IP address."
	}
	return setDNSLastTransitionTime(&degradedCondition, oldDegradedCondition), err
}

func generateCondition(condtype, reason, message string, status operatorv1.ConditionStatus, transitionTime metav1.Time) *operatorv1.OperatorCondition {
	return &operatorv1.OperatorCondition{
		Type:               condtype,
		Reason:             reason,
		Status:             status,
		Message:            message,
		LastTransitionTime: transitionTime,
	}
}

// computeDNSProgressingCondition computes the dns Progressing status
// condition based on the status of the DNS and node-resolver
// daemonsets. If the elapsed time between currentTime and
// oldCondition.LastTransitionTime is <= transitionUnchangedToleration then
// consider oldCondition to be recent and return oldCondition to
// prevent frequent updates.
func computeDNSProgressingCondition(oldCondition *operatorv1.OperatorCondition, dns *operatorv1.DNS, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet, transitionUnchangedToleration time.Duration, currentTime time.Time, reconcileResult *reconcile.Result) operatorv1.OperatorCondition {
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
		want := dnsDaemonset.Status.DesiredNumberScheduled // num of nodes that should be running the pod.
		have := dnsDaemonset.Status.UpdatedNumberScheduled // num of nodes running the updated pod.
		// It's progressing when have < want.  If have >= want, that's okay.
		if have < want {
			messages = append(messages, fmt.Sprintf("Have %d up-to-date DNS pods, want %d.", have, want))
		}

		haveSelector := dnsDaemonset.Spec.Template.Spec.NodeSelector
		wantSelector := nodeSelectorForDNS(dns)
		if !reflect.DeepEqual(haveSelector, wantSelector) {
			messages = append(messages, fmt.Sprintf("Have DNS daemonset with node selector %+v, want %+v.", haveSelector, wantSelector))
		}

		haveTolerations := dnsDaemonset.Spec.Template.Spec.Tolerations
		wantTolerations := tolerationsForDNS(dns)
		if !reflect.DeepEqual(haveTolerations, wantTolerations) {
			messages = append(messages, fmt.Sprintf("Have DNS daemonset with tolerations %+v, want %+v.", haveTolerations, wantTolerations))
		}
	}
	if !haveNodeResolverDaemonset {
		messages = append(messages, "The node-resolver daemonset does not exist.")
	} else {
		want := nodeResolverDaemonset.Status.DesiredNumberScheduled // num of nodes that should be running the pod.
		have := nodeResolverDaemonset.Status.UpdatedNumberScheduled // num of nodes running the updated pod.

		// It's progressing when have < want.  If have >= want, that's okay.
		if have < want {
			messages = append(messages, fmt.Sprintf("Have %d available node-resolver pods, want %d.", have, want))
		}
	}
	if len(messages) != 0 {
		// if the last status was set to false within the last transitionUnchangedToleration, skip the new update
		// to prevent frequent status flaps, and try to keep the long-lasting state (i.e. Progressing=False). See https://bugzilla.redhat.com/show_bug.cgi?id=2037190.
		if oldCondition != nil && oldCondition.Status == operatorv1.ConditionFalse && lastTransitionTimeIsRecent(currentTime, oldCondition.LastTransitionTime.Time, transitionUnchangedToleration) {
			reconcileResult.Requeue = true
			reconcileResult.RequeueAfter = transitionUnchangedToleration
			return *oldCondition
		}
		progressingCondition.Status = operatorv1.ConditionTrue
		progressingCondition.Reason = "Reconciling"
		progressingCondition.Message = strings.Join(messages, "\n")
	} else {
		progressingCondition.Status = operatorv1.ConditionFalse
		progressingCondition.Reason = "AsExpected"
		progressingCondition.Message = "All DNS and node-resolver pods are updated, and the DNS service has a cluster IP address."
	}

	return setDNSLastTransitionTime(progressingCondition, oldCondition)
}

// computeDNSAvailableCondition computes the dns Available status condition
// based on the status of clusterIP and the DNS daemonset.
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
		messages = append(messages, "No IP address is assigned to the DNS service.")
	}
	if len(unavailableReasons) != 0 {
		availableCondition.Status = operatorv1.ConditionFalse
		availableCondition.Reason = strings.Join(unavailableReasons, " ")
		availableCondition.Message = strings.Join(messages, "\n")
	} else {
		availableCondition.Status = operatorv1.ConditionTrue
		availableCondition.Reason = "AsExpected"
		availableCondition.Message = "The DNS daemonset has available pods, and the DNS service has a cluster IP address."
	}

	return setDNSLastTransitionTime(availableCondition, oldCondition)
}

func computeDNSUpgradeableCondition(oldCondition *operatorv1.OperatorCondition, dns *operatorv1.DNS) operatorv1.OperatorCondition {
	upgradeableCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeUpgradeable,
	}

	if dns.Spec.ManagementState == operatorv1.Unmanaged {
		upgradeableCondition.Status = operatorv1.ConditionFalse
		upgradeableCondition.Reason = "OperatorUnmanaged"
		upgradeableCondition.Message = "Cannot upgrade while managementState is Unmanaged"
	} else {
		upgradeableCondition.Status = operatorv1.ConditionTrue
		upgradeableCondition.Reason = "AsExpected"
		upgradeableCondition.Message = "DNS Operator can be upgraded"
	}

	return setDNSLastTransitionTime(upgradeableCondition, oldCondition)
}

// setDNSLastTransitionTime sets LastTransitionTime for the given condition.
// If the condition has changed, it will assign a new timestamp otherwise keeps the old timestamp.
func setDNSLastTransitionTime(condition, oldCondition *operatorv1.OperatorCondition) operatorv1.OperatorCondition {
	if oldCondition != nil && condition.Status == oldCondition.Status {
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
	if !cond.ConditionsEqual(a.Conditions, b.Conditions) {
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

// lastTransitionTimeIsRecent returns true if elapsed time
// (currTime-prevTime) <= toleration. It returns false if currTime is
// before prevTime or if toleration < 0. It returns true if elapsed time
// is 0.
func lastTransitionTimeIsRecent(currTime, prevTime time.Time, toleration time.Duration) bool {
	if toleration < 0 {
		return false
	}
	switch elapsed := currTime.Sub(prevTime); {
	case elapsed == 0:
		return true
	case elapsed < 0:
		return false
	default:
		return elapsed <= toleration
	}
}
