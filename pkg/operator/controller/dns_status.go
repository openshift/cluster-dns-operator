package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	retry "github.com/openshift/cluster-dns-operator/pkg/util/retryableerror"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// clock is to enable requeueing and unit testing
var clock utilclock.Clock = utilclock.RealClock{}

// expectedCondition contains a condition that is expected to be checked when
// determining the Degraded status of the dns controller
type expectedCondition struct {
	condition   string
	status      operatorv1.ConditionStatus
	gracePeriod time.Duration
}

// syncDNSStatus computes the current status of dns and
// updates status upon any changes since last sync.
// If the elapsed time between time.Now() and
// oldCondition.LastTransitionTime is <= transitionUnchangedToleration
// for progressing and degraded then consider oldCondition to be recent
// and return oldCondition to prevent frequent updates.
func (r *reconciler) syncDNSStatus(dns *operatorv1.DNS, clusterIP, clusterDomain string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet, transitionUnchangedToleration time.Duration) error {
	var retryableErr error

	updated := dns.DeepCopy()
	updated.Status.ClusterIP = clusterIP
	updated.Status.ClusterDomain = clusterDomain

	updated.Status.Conditions, retryableErr = computeDNSStatusConditions(dns, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset, transitionUnchangedToleration)
	if !dnsStatusesEqual(updated.Status, dns.Status) {
		if err := r.client.Status().Update(context.TODO(), updated); err != nil {
			return fmt.Errorf("failed to update dns status: %v", err)
		}
		logrus.Infof("updated DNS %s status: old: %#v, new: %#v", dns.ObjectMeta.Name, dns.Status, updated.Status)
	}

	return retryableErr
}

// computeDNSStatusConditions computes dns status conditions based on
// the status of the dns and node-resolver daemonsets and clusterIP.
// If the elapsed time between time.Now() and
// oldCondition.LastTransitionTime is <= transitionUnchangedToleration
// for progressing and degraded then consider oldCondition to be recent
// and return oldCondition to prevent frequent updates.
func computeDNSStatusConditions(dns *operatorv1.DNS, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet, transitionUnchangedToleration time.Duration) ([]operatorv1.OperatorCondition, error) {
	oldConditions := dns.Status.Conditions
	var oldDegradedCondition, oldDNSMaxUnavailableDNSPodsExceeded, oldProgressingCondition, oldAvailableCondition, oldUpgradeableCondition *operatorv1.OperatorCondition
	for i := range oldConditions {
		switch oldConditions[i].Type {
		case operatorv1.OperatorStatusTypeDegraded:
			oldDegradedCondition = &oldConditions[i]
		case DNSMaxUnavailableDNSPodsExceeded:
			oldDNSMaxUnavailableDNSPodsExceeded = &oldConditions[i]
		case operatorv1.OperatorStatusTypeProgressing:
			oldProgressingCondition = &oldConditions[i]
		case operatorv1.OperatorStatusTypeAvailable:
			oldAvailableCondition = &oldConditions[i]
		case operatorv1.OperatorStatusTypeUpgradeable:
			oldUpgradeableCondition = &oldConditions[i]
		}
	}

	now := time.Now()
	degradedConditions, retryErr := computeDNSDegradedCondition(oldDegradedCondition, oldDNSMaxUnavailableDNSPodsExceeded, clusterIP, haveDNSDaemonset, dnsDaemonset, transitionUnchangedToleration, now)
	degradedConditions = append(degradedConditions, computeDNSProgressingCondition(oldProgressingCondition, dns, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset))
	degradedConditions = append(degradedConditions, computeDNSAvailableCondition(oldAvailableCondition, clusterIP, haveDNSDaemonset, dnsDaemonset))
	degradedConditions = append(degradedConditions, computeDNSUpgradeableCondition(oldUpgradeableCondition, dns))

	return degradedConditions, retryErr
}

var (
	// DNSNoService indicates that no IP address is assigned to the DNS service
	DNSNoService = "NoService"

	// DNSNoDNSDaemonSet indicates that the DNS daemon set doesn't exist
	DNSNoDNSDaemonSet = "NoDNSDaemonSet"

	// DNSNoDNSPodsDesired indicates that no DNS pods are desired; this could mean
	// all nodes are tainted or unschedulable
	DNSNoDNSPodsDesired = "NoDNSPodsDesired"

	// DNSNoDNSPodsAvailable indicates that no DNS pods are available
	DNSNoDNSPodsAvailable = "NoDNSPodsAvailable"

	// DNSInvalidDNSMaxUnavailable indicates that the DNS daemonset has an invalid
	// MaxUnavailable value configured
	DNSInvalidDNSMaxUnavailable = "InvalidDNSMaxUnavailable"

	// DNSMaxUnavailableDNSPodsExceeded indicates that the number of unavailable DNS
	// pods is greater than the configured MaxUnavailable
	DNSMaxUnavailableDNSPodsExceeded = "MaxUnavailableDNSPodsExceeded"
)

// computeDNSDegradedCondition computes the dns Degraded status
// condition based on the status of clusterIP and the DNS daemonset.
// The node-resolver daemonset is not a part of the calculation of
// degraded condition. If the elapsed time between currentTime and
// oldCondition.LastTransitionTime is <= transitionUnchangedToleration
// then consider oldCondition to be recent and return oldCondition to
// prevent frequent updates.
func computeDNSDegradedCondition(oldDegradedCondition, oldGraceCondition *operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, transitionUnchangedToleration time.Duration, currentTime time.Time) ([]operatorv1.OperatorCondition, error) {
	degradedCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeDegraded,
	}
	graceCondition := &operatorv1.OperatorCondition{
		Status: operatorv1.ConditionUnknown,
	}
	var degradedConditions []operatorv1.OperatorCondition

	// These conditions are expected to be false
	expectedConditions := []expectedCondition{
		{
			condition: DNSNoService,
			status:    operatorv1.ConditionFalse,
		},
		{
			condition: DNSNoDNSDaemonSet,
			status:    operatorv1.ConditionFalse,
		},
		{
			condition: DNSNoDNSPodsDesired,
			status:    operatorv1.ConditionFalse,
		},
		{
			condition: DNSNoDNSPodsAvailable,
			status:    operatorv1.ConditionFalse,
		},
		{
			condition: DNSInvalidDNSMaxUnavailable,
			status:    operatorv1.ConditionFalse,
		},
		{
			condition:   DNSMaxUnavailableDNSPodsExceeded,
			status:      operatorv1.ConditionFalse,
			gracePeriod: time.Minute * 5,
		},
	}

	// Determine any degraded conditions
	if len(clusterIP) == 0 {
		degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{
			Type:    DNSNoService,
			Reason:  DNSNoService,
			Status:  operatorv1.ConditionTrue,
			Message: "No IP address is assigned to the DNS service.",
		})
	}
	if !haveDNSDaemonset {
		degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{
			Type:    DNSNoDNSDaemonSet,
			Reason:  DNSNoDNSDaemonSet,
			Status:  operatorv1.ConditionTrue,
			Message: "The DNS daemonset does not exist.",
		})
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
			degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{
				Type:    DNSNoDNSPodsDesired,
				Reason:  DNSNoDNSPodsDesired,
				Status:  operatorv1.ConditionTrue,
				Message: "No DNS pods are desired; this could mean all nodes are tainted or unschedulable.",
			})
		case have == 0:
			degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{
				Type:    DNSNoDNSPodsAvailable,
				Reason:  DNSNoDNSPodsAvailable,
				Status:  operatorv1.ConditionTrue,
				Message: "No DNS pods are available.",
			})
		case intstrErr != nil:
			degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{
				Type:    DNSInvalidDNSMaxUnavailable,
				Reason:  DNSInvalidDNSMaxUnavailable,
				Status:  operatorv1.ConditionTrue,
				Message: fmt.Sprintf("The DNS daemonset has an invalid MaxUnavailable value: %v", intstrErr),
			})
		case int(numberUnavailable) > maxUnavailable:
			graceCondition = &operatorv1.OperatorCondition{
				Type:    DNSMaxUnavailableDNSPodsExceeded,
				Reason:  DNSMaxUnavailableDNSPodsExceeded,
				Status:  operatorv1.ConditionTrue,
				Message: fmt.Sprintf("Too many DNS pods are unavailable (%d > %d max unavailable).", numberUnavailable, maxUnavailable),
			}
		}
	}

	// If there are degraded conditions, we won't requeue
	if len(degradedConditions) != 0 {
		// If the last status was set to false within the last transitionUnchangedToleration, skip the new update
		// to prevent frequent status flaps, and try to keep the long-lasting state (i.e. Degraded=False). See https://bugzilla.redhat.com/show_bug.cgi?id=2037190.
		if oldDegradedCondition != nil && oldDegradedCondition.Status == operatorv1.ConditionFalse && lastTransitionTimeIsRecent(currentTime, oldDegradedCondition.LastTransitionTime.Time, transitionUnchangedToleration) {
			return []operatorv1.OperatorCondition{*oldDegradedCondition}, nil
		}
		// If not, there is still no grace period, and this should not be requeued.
		// Set the final degraded condition as true, and concatenate the reasons and messages
		reasons, messages := formatConditions(degradedConditions)
		degradedCondition.Status = operatorv1.ConditionTrue
		degradedCondition.Reason = reasons
		degradedCondition.Message = messages
		degradedConditions = append(degradedConditions, setDNSLastTransitionTime(degradedCondition, oldDegradedCondition))
		return degradedConditions, nil
	}

	// If the graceCondition occurs, determine if this should be requeued.
	// Set the final degraded condition as false and set it up to requeue the reconcile.
	// We check this last, because the other conditions are non-recoverable as degraded conditions.
	if graceCondition.Status == operatorv1.ConditionTrue {
		requeueTime := requeueAfter(expectedConditions, graceCondition, oldGraceCondition)

		var err error
		if requeueTime != 0 {
			// This should be requeued
			degradedCondition.Status = operatorv1.ConditionFalse
			degradedCondition.Reason = "Waiting"
			degradedCondition.Message = graceCondition.Message
			err = retry.New(errors.New("DNS service is waiting: "+graceCondition.Message), requeueTime)
		} else {
			// This should not be requeued
			degradedCondition.Status = operatorv1.ConditionTrue
			degradedCondition.Reason = "GracePeriodExpired"
			degradedCondition.Message = graceCondition.Message
		}

		degradedConditions = append(degradedConditions, setDNSLastTransitionTime(degradedCondition, oldDegradedCondition))
		return degradedConditions, err
	}

	// nothing is degraded, with or without a grace condition
	degradedCondition.Status = operatorv1.ConditionFalse
	degradedCondition.Reason = "AsExpected"
	degradedCondition.Message = "Enough DNS pods are available, and the DNS service has a cluster IP address."

	return append(degradedConditions, setDNSLastTransitionTime(degradedCondition, oldDegradedCondition)), nil
}

// requeueAfter compares expected operator conditions to the grace
// condition and returns a requeueing wait time.
func requeueAfter(expectedConds []expectedCondition, graceCondition, oldGraceCondition *operatorv1.OperatorCondition) time.Duration {
	var requeueAfter time.Duration
	now := clock.Now()
	init := true
	var t2 time.Time
	if oldGraceCondition != nil {
		t2 = oldGraceCondition.LastTransitionTime.Time
	} else {
		t2 = now
	}
	conditionsMap := make(map[string]*operatorv1.OperatorCondition)
	conditionsMap[graceCondition.Type] = graceCondition

	for _, expected := range expectedConds {
		condition, haveCondition := conditionsMap[expected.condition]
		if !haveCondition {
			continue
		}
		if condition.Status == expected.status {
			continue
		}
		if expected.gracePeriod != 0 {
			t1 := now.Add(-expected.gracePeriod)
			if t2.After(t1) {
				d := t2.Sub(t1)
				if init || d < requeueAfter {
					// Set the requeueAfter value
					// if the grace period hasn't elapsed
					requeueAfter = d
					init = false
				}
				continue
			}
		}
	}
	return requeueAfter
}

func formatConditions(conditions []operatorv1.OperatorCondition) (string, string) {
	var reasons, messages string
	if len(conditions) == 0 {
		return "", ""
	}
	for _, cond := range conditions {
		// Use a CamelCase string for reasons
		reasons = reasons + "And" + cond.Reason
		messages = messages + fmt.Sprintf(", %s=%s (%s)", cond.Type, cond.Status, cond.Message)
	}
	return reasons[3:], messages[2:]
}

func conditionUnChanged(a, b *operatorv1.OperatorCondition) bool {
	return a.Status == b.Status && a.Reason == b.Reason && a.Message == b.Message
}

// computeDNSProgressingCondition computes the dns Progressing status condition
// based on the status of the DNS and node-resolver daemonsets.
func computeDNSProgressingCondition(oldCondition *operatorv1.OperatorCondition, dns *operatorv1.DNS, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet) operatorv1.OperatorCondition {
	progressingCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeProgressing,
	}
	var messages []string
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
// based on the status of clusterIP and the DNS daemonset.
func computeDNSAvailableCondition(oldCondition *operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet) operatorv1.OperatorCondition {
	availableCondition := &operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeAvailable,
	}
	var unavailableReasons []string
	var messages []string

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
		availableCondition.Reason = strings.Join(unavailableReasons, "")
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
	if oldCondition != nil && conditionUnChanged(condition, oldCondition) {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	} else {
		condition.LastTransitionTime = metav1.NewTime(clock.Now())
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

// lastTransitionTimeIsRecent returns true if elapsed time
// (currTime-prevTime) <= toleration. It returns false if currTime is
// before prevTime or if toleration < 0. It returns true if elapsed time
// is 0.
func lastTransitionTimeIsRecent(currTime, prevTime time.Time, toleration time.Duration) bool {
	ret := false
	if toleration < 0 {
		ret = false
	} else {
		switch elapsed := currTime.Sub(prevTime); {
		case elapsed == 0:
			ret = true
		case elapsed < 0:
			ret = false
		default:
			ret = elapsed <= toleration
		}
	}
	return ret
}
