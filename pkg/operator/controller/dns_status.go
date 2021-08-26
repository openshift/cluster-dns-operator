package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	operatorv1 "github.com/openshift/api/operator/v1"
	retry "github.com/openshift/cluster-dns-operator/pkg/util/retryableerror"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilclock "k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// clock is to enable requeueing and unit testing
var clock utilclock.Clock = utilclock.RealClock{}

// dnsCondition contains a condition that is expected to be checked when
// determining the Degraded status of the dns controller
type dnsCondition struct {
	condition   string
	status      operatorv1.ConditionStatus
	gracePeriod time.Duration
}

// syncDNSStatus computes the current status of dns and
// updates status upon any changes since last sync.
func (r *reconciler) syncDNSStatus(dns *operatorv1.DNS, clusterIP, clusterDomain string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet) error {
	var retryableErr error
	updated := dns.DeepCopy()
	updated.Status.ClusterIP = clusterIP
	updated.Status.ClusterDomain = clusterDomain
	updated.Status.Conditions, retryableErr = computeDNSStatusConditions(dns, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset)
	if !dnsStatusesEqual(updated.Status, dns.Status) {
		if err := r.client.Status().Update(context.TODO(), updated); err != nil {
			return fmt.Errorf("failed to update dns status: %v", err)
		}
		logrus.Infof("updated DNS %s status: old: %#v, new: %#v", dns.ObjectMeta.Name, dns.Status, updated.Status)
	}

	return retryableErr
}

// computeDNSStatusConditions computes dns status conditions based on
// the status of ds and clusterIP.
func computeDNSStatusConditions(dns *operatorv1.DNS, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet) ([]operatorv1.OperatorCondition, error) {
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

	dnsConditions, retryErr := computeDNSDegradedCondition(oldDegradedCondition, clusterIP, haveDNSDaemonset, dnsDaemonset)

	conditions := []operatorv1.OperatorCondition{}
	conditions = append(conditions, dnsConditions)
	conditions = append(conditions, computeDNSProgressingCondition(oldProgressingCondition, dns, clusterIP, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset))
	conditions = append(conditions, computeDNSAvailableCondition(oldAvailableCondition, clusterIP, haveDNSDaemonset, dnsDaemonset))
	conditions = append(conditions, computeDNSUpgradeableCondition(oldUpgradeableCondition, dns))

	return conditions, retryErr
}

var (
	// Degraded indicates the DNS controller is degraded
	DNSControllerDegradedConditionType = "Degraded"

	// DNSNoService indicates that no IP address is assigned to the DNS service
	DNSNoService = "NoService"

	// DNSNoDNSDaemonSet indicates that the DNS daemon set doesn't exist
	DNSNoDNSDaemonSet = "NoDaemonSet"

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

// computeDNSDegradedCondition computes the dns Degraded status condition
// based on the status of clusterIP and the DNS daemonset.  The node-resolver
// daemonset is not a part of the calculation of degraded condition.
func computeDNSDegradedCondition(oldCondition *operatorv1.OperatorCondition, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet) (operatorv1.OperatorCondition, error) {
	finalDegradedCondition := operatorv1.OperatorCondition{
		Type: operatorv1.OperatorStatusTypeDegraded,
	}
	degradedConditions := []operatorv1.OperatorCondition{}
	messages := []string{}
	var keepConditionUnknown bool
	now := metav1.NewTime(clock.Now())

	// These conditions are expected to be false
	expectedConditions := []dnsCondition{
		{
			condition: DNSNoService,
			status:    operatorv1.ConditionFalse,
		},
		{
			condition: DNSControllerDegradedConditionType,
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
			condition:   DNSNoDNSPodsAvailable,
			status:      operatorv1.ConditionFalse,
			gracePeriod: time.Minute * 5,
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
		degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{Type: DNSNoService, Status: operatorv1.ConditionTrue, LastTransitionTime: now})
		messages = append(messages, "No IP address is assigned to the DNS service.")
	}
	if !haveDNSDaemonset {
		degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{Type: DNSNoDNSDaemonSet, Status: operatorv1.ConditionTrue, LastTransitionTime: now})
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
			degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{Type: DNSNoDNSPodsDesired, Status: operatorv1.ConditionTrue, LastTransitionTime: now})
			messages = append(messages, "No DNS pods are desired; this could mean all nodes are tainted or unschedulable.")
		case have == 0:
			degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{Type: DNSNoDNSPodsAvailable, Status: operatorv1.ConditionTrue, LastTransitionTime: now})
			messages = append(messages, "No DNS pods are available.")
		case intstrErr != nil:
			keepConditionUnknown = true
			degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{Type: DNSInvalidDNSMaxUnavailable, Status: operatorv1.ConditionUnknown, LastTransitionTime: now})
			messages = append(messages, fmt.Sprintf("The DNS daemonset has an invalid MaxUnavailable value: %v", intstrErr))
		case int(numberUnavailable) > maxUnavailable:
			degradedConditions = append(degradedConditions, operatorv1.OperatorCondition{Type: DNSMaxUnavailableDNSPodsExceeded, Status: operatorv1.ConditionTrue, LastTransitionTime: now})
			messages = append(messages, fmt.Sprintf("Too many DNS pods are unavailable (%d > %d max unavailable).", numberUnavailable, maxUnavailable))
		}
	}

	graceConditions, checkedConditions, requeueAfter := checkConditions(expectedConditions, degradedConditions)

	// graceConditions have grace periods configured in the expectedConditions, so if present, this should be requeued
	if len(graceConditions) != 0 {
		finalDegradedCondition.Status = operatorv1.ConditionTrue
		finalDegradedCondition.Reason = DNSControllerDegradedConditionType
		finalDegradedCondition.Message = strings.Join(messages, "\n")
		grace := formatConditions(graceConditions)

		// If the oldCondition changed more recently than requeueAfter period of time, reduce the requeueAfter period of time
		if oldCondition != nil && (!oldCondition.LastTransitionTime.IsZero()) {
			t1 := now.Add(-requeueAfter)
			t2 := oldCondition.LastTransitionTime
			if t2.After(t1) {
				requeueAfter = t2.Sub(t1)
			}
		}

		return setDNSLastTransitionTime(&finalDegradedCondition, oldCondition), retry.New(errors.New("DNS service is degraded: "+grace), requeueAfter)
	}

	// checkedConditions are degradedConditions without gracePeriods, and the degraded condition is final (this should not be requeued)
	if len(checkedConditions) != 0 {
		if keepConditionUnknown {
			finalDegradedCondition.Status = operatorv1.ConditionUnknown
		} else {
			finalDegradedCondition.Status = operatorv1.ConditionTrue
		}
		finalDegradedCondition.Reason = DNSControllerDegradedConditionType
		finalDegradedCondition.Message = strings.Join(messages, "\n")

		return setDNSLastTransitionTime(&finalDegradedCondition, oldCondition), nil
	}

	finalDegradedCondition.Status = operatorv1.ConditionFalse
	finalDegradedCondition.Reason = "AsExpected"
	finalDegradedCondition.Message = "Enough DNS pods are available, and the DNS service has a cluster IP address."
	return setDNSLastTransitionTime(&finalDegradedCondition, oldCondition), nil
}

// checkConditions compares expected operator conditions to existing operator
// conditions and returns a list of graceConditions and a requeueing wait time.
func checkConditions(expectedConds []dnsCondition, conditions []operatorv1.OperatorCondition) ([]operatorv1.OperatorCondition, []operatorv1.OperatorCondition, time.Duration) {
	var graceConditions, degradedConditions []operatorv1.OperatorCondition
	var requeueAfter time.Duration
	conditionsMap := make(map[string]operatorv1.OperatorCondition)

	for i := range conditions {
		conditionsMap[conditions[i].Type] = conditions[i]
	}
	now := clock.Now()
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

func formatConditions(conditions []operatorv1.OperatorCondition) string {
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

// computeDNSProgressingCondition computes the dns Progressing status condition
// based on the status of the DNS and node-resolver daemonsets.
func computeDNSProgressingCondition(oldCondition *operatorv1.OperatorCondition, dns *operatorv1.DNS, clusterIP string, haveDNSDaemonset bool, dnsDaemonset *appsv1.DaemonSet, haveNodeResolverDaemonset bool, nodeResolverDaemonset *appsv1.DaemonSet) operatorv1.OperatorCondition {
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
