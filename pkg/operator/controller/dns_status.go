package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	operatorv1 "github.com/openshift/api/operator/v1"
	cond "github.com/openshift/cluster-dns-operator/pkg/util/conditions"
	retryable "github.com/openshift/cluster-dns-operator/pkg/util/retryableerror"

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
	updated := dns.DeepCopy()
	updated.Status.ClusterIP = clusterIP
	updated.Status.ClusterDomain = clusterDomain
	statusConds, err := computeDNSStatusConditions(dns, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset, transitionUnchangedToleration, reconcileResult)
	if err != nil {
		logrus.Infof("error computing DNS %s status: %v got %v", dns.ObjectMeta.Name, statusConds, err)
		return err
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

// computeDNSDegradedCondition computes the dns Degraded status
// condition based on the status of clusterIP and the DNS daemonset.
// The node-resolver daemonset is not a part of the calculation of
// degraded condition. If the elapsed time between currentTime and
// oldCondition.LastTransitionTime is <= transitionUnchangedToleration
// then consider oldCondition to be recent and return oldCondition to
// prevent frequent updates.
func computeDNSDegradedCondition(oldDegradedCondition, newProgressingCondition *operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, transitionUnchangedToleration time.Duration, currentTime time.Time) (operatorv1.OperatorCondition, error) {
	degradedCondition := operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeDegraded,
	}
	messages := []string{}
	degradedReasons := []string{}

	// The list of status conditions we might see when the operator is NOT degraded.
	expected := []cond.ExpectedCondition{
		{
			Condition: operatorv1.OperatorStatusTypeDegraded,
			Status:    operatorv1.ConditionFalse,
			// If Degraded=true, it's okay if it's only temporary, e.g. has changed within the toleration.
			GracePeriod: transitionUnchangedToleration,
		},
		{
			Condition: operatorv1.OperatorStatusTypeProgressing,
			// If Progressing=true, then it's not yet stable, and can't be degraded.
			Status: operatorv1.ConditionTrue,
			// TODO - we could enforce a grace period so it can only be Progressing for a limited time
			// TODO - before being marked as Degraded
		},
	}

	if len(clusterIP) == 0 {
		degradedReasons = append(degradedReasons, "NoService")
		messages = append(messages, "No IP address is assigned to the DNS service.")
	}
	if !haveDNSDaemonset {
		degradedReasons = append(degradedReasons, "NoDNSDaemonSet")
		messages = append(messages, "The DNS daemonset does not exist.")
	} else {
		want := dnsDaemonset.Status.DesiredNumberScheduled
		have := dnsDaemonset.Status.NumberAvailable
		numberUnavailable := want - have
		maxUnavailableIntStr := intstr.FromString("10%")
		maxUnavailable, intstrErr := intstr.GetScaledValueFromIntOrPercent(&maxUnavailableIntStr, int(want), true)

		switch {
		case want == 0:
			degradedReasons = append(degradedReasons, "NoDNSPodsDesired")
			messages = append(messages, "No DNS pods are desired; this could mean all nodes are tainted or unschedulable.")
		case have == 0:
			degradedReasons = append(degradedReasons, "NoDNSPodsAvailable")
			messages = append(messages, "No DNS pods are available.")
		case intstrErr != nil:
			// This should not happen, but is included just to safeguard against future changes.
			degradedReasons = append(degradedReasons, "InvalidDNSMaxUnavailable")
			messages = append(messages, fmt.Sprintf("The DNS daemonset has an invalid MaxUnavailable value: %v", intstrErr))
		case int(numberUnavailable) > maxUnavailable:
			degradedReasons = append(degradedReasons, "MaxUnavailableDNSPodsExceeded")
			messages = append(messages, fmt.Sprintf("Too many DNS pods are unavailable (%d > %d max unavailable).", numberUnavailable, maxUnavailable))
		}
	}

	// Record whether the operator is Progressing.
	progressing := newProgressingCondition != nil && newProgressingCondition.Status == operatorv1.ConditionTrue
	var currentConditions []operatorv1.OperatorCondition
	if progressing {
		currentConditions = append(currentConditions, *newProgressingCondition)
	}

	if len(degradedReasons) != 0 {
		// if the last status was set to false within the last transitionUnchangedToleration, skip the new update
		// to prevent frequent status flaps, and try to keep the long-lasting state (i.e. Degraded=False). See https://bugzilla.redhat.com/show_bug.cgi?id=2037190.
		if oldDegradedCondition != nil && oldDegradedCondition.Status == operatorv1.ConditionFalse && lastTransitionTimeIsRecent(currentTime, oldDegradedCondition.LastTransitionTime.Time, transitionUnchangedToleration) {
			return *oldDegradedCondition, nil
		}
		degradedCondition.Status = operatorv1.ConditionTrue
		degradedCondition.Reason = strings.Join(degradedReasons, " ")
		degradedCondition.Message = strings.Join(messages, "\n")
	} else {
		degradedCondition.Status = operatorv1.ConditionFalse
		degradedCondition.Reason = "AsExpected"
		degradedCondition.Message = "Enough DNS pods are available, and the DNS service has a cluster IP address."
	}
	currentConditions = append(currentConditions, degradedCondition)

	// Now that we have information on the current DNS operator status, check the conditions against what we expect.
	graceConditions, degradedConditions, requeueAfter := cond.CheckConditions(expected, currentConditions)

	if !progressing && len(degradedConditions) != 0 {
		// We may want to keep checking conditions while degraded.
		retryAfter := transitionUnchangedToleration

		degraded := cond.FormatConditions(degradedConditions)
		degradedCondition.Status = operatorv1.ConditionTrue
		degradedCondition.Reason = "DegradedConditions"
		degradedCondition.Message = "One or more other status conditions indicate a degraded state: " + degraded

		return degradedCondition, retryable.New(errors.New("DNSController is degraded: "+degraded), retryAfter)
	} else {
		degradedCondition.Status = operatorv1.ConditionFalse
		degradedCondition.Reason = "AsExpected"
		degradedCondition.Message = "Enough DNS pods are available, and the DNS service has a cluster IP address."
		var err error
		if len(graceConditions) != 0 {
			var grace string
			for _, gc := range graceConditions {
				grace = grace + fmt.Sprintf(", %s=%s", gc.Type, gc.Status)
			}
			grace = grace[2:]

			err = retryable.New(errors.New("DNSController may become degraded soon: "+grace), requeueAfter)
		}
		return setDNSLastTransitionTime(&degradedCondition, oldDegradedCondition), err
	}

	// TODO is there any case for;
	// degradedCondition.Message = "DNSController appears to be Progressing, but may become Degraded soon."
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
		want := dnsDaemonset.Status.DesiredNumberScheduled // num of nodes that should be running the pod
		have := dnsDaemonset.Status.UpdatedNumberScheduled // num of nodes running the updated pod
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
		have := nodeResolverDaemonset.Status.NumberAvailable
		want := nodeResolverDaemonset.Status.DesiredNumberScheduled
		// When we check DNS pods we use the less-than comparison, makes sense to do it for node-resolver pods too.
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
		progressingCondition.Message = "All DNS and node-resolver pods are available, and the DNS service has a cluster IP address."
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
