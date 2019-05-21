package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	DNSOperatorName = "dns"

	OperatorVersionName     = "operator"
	CoreDNSVersionName      = "coredns"
	OpenshiftCLIVersionName = "openshift-cli"
	UnknownVersionValue     = "unknown"
)

type dnsStatusConditionsCounts struct {
	available   int
	progressing int
	degraded    int
	total       int
}

// syncOperatorStatus computes the operator's current status and therefrom
// creates or updates the ClusterOperator resource for the operator.
func (r *reconciler) syncOperatorStatus() error {
	ns := manifests.DNSNamespace()

	co := &configv1.ClusterOperator{ObjectMeta: metav1.ObjectMeta{Name: DNSOperatorName}}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: co.Name}, co); err != nil {
		if errors.IsNotFound(err) {
			initializeClusterOperator(co, ns.Name)
			if err := r.client.Create(context.TODO(), co); err != nil {
				return fmt.Errorf("failed to create clusteroperator %s: %v", co.Name, err)
			}
			logrus.Infof("created clusteroperator %s", co.Name)
		} else {
			return fmt.Errorf("failed to get clusteroperator %s: %v", co.Name, err)
		}
	}
	oldStatus := co.Status.DeepCopy()

	dnses, ns, err := r.getOperatorState(ns.Name)
	if err != nil {
		return fmt.Errorf("failed to get operator state: %v", err)
	}
	dnsStatusConditionsCounts := computeDNSStatusConditionCounts(dnses)
	co.Status.Versions = r.computeOperatorStatusVersions(oldStatus.Versions, dnsStatusConditionsCounts)
	co.Status.Conditions = r.computeOperatorStatusConditions(oldStatus.Conditions, ns, dnsStatusConditionsCounts, oldStatus.Versions, co.Status.Versions)

	if !operatorStatusesEqual(*oldStatus, co.Status) {
		if err := r.client.Status().Update(context.TODO(), co); err != nil {
			return fmt.Errorf("failed to update clusteroperator %s: %v", co.Name, err)
		}
	}

	return nil
}

// Populate versions and conditions in cluster operator status as CVO expects these fields.
func initializeClusterOperator(co *configv1.ClusterOperator, nsName string) {
	co.Status.Versions = []configv1.OperandVersion{
		{
			Name:    OperatorVersionName,
			Version: UnknownVersionValue,
		},
		{
			Name:    CoreDNSVersionName,
			Version: UnknownVersionValue,
		},
		{
			Name:    OpenshiftCLIVersionName,
			Version: UnknownVersionValue,
		},
	}
	co.Status.Conditions = []configv1.ClusterOperatorStatusCondition{
		{
			Type:   configv1.OperatorDegraded,
			Status: configv1.ConditionUnknown,
		},
		{
			Type:   configv1.OperatorProgressing,
			Status: configv1.ConditionUnknown,
		},
		{
			Type:   configv1.OperatorAvailable,
			Status: configv1.ConditionUnknown,
		},
	}
	co.Status.RelatedObjects = []configv1.ObjectReference{
		{
			Resource: "namespaces",
			Name:     "openshift-dns-operator",
		},
		{
			Resource: "namespaces",
			Name:     nsName,
		},
	}
}

// getOperatorState gets and returns the resources necessary to compute the
// operator's current state.
func (r *reconciler) getOperatorState(nsName string) ([]operatorv1.DNS, *corev1.Namespace, error) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: nsName}, ns); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to get namespace %s: %v", nsName, err)
	}

	dnsList := &operatorv1.DNSList{}
	if err := r.client.List(context.TODO(), dnsList); err != nil {
		return nil, nil, fmt.Errorf("failed to list dnses: %v", err)
	}

	return dnsList.Items, ns, nil
}

// computeDNSStatusConditionCounts computes for each status condition how many
// DNSes have that condition.
func computeDNSStatusConditionCounts(dnses []operatorv1.DNS) dnsStatusConditionsCounts {
	dnsStatusConditionsCounts := dnsStatusConditionsCounts{}
	for _, dns := range dnses {
		var (
			// Assume the DNS is unavailable, degraded, and
			// progressing unless its conditions indicate otherwise.
			available   = false
			degraded    = true
			progressing = true
		)
		for _, c := range dns.Status.Conditions {
			switch {
			case c.Type == operatorv1.OperatorStatusTypeAvailable && c.Status == operatorv1.ConditionTrue:
				available = true
			case c.Type == operatorv1.OperatorStatusTypeProgressing && c.Status == operatorv1.ConditionFalse:
				progressing = false
			case c.Type == operatorv1.OperatorStatusTypeDegraded && c.Status == operatorv1.ConditionFalse:
				degraded = false
			}
		}
		dnsStatusConditionsCounts.total++
		if available {
			dnsStatusConditionsCounts.available++
		}
		if degraded {
			dnsStatusConditionsCounts.degraded++
		}
		if progressing {
			dnsStatusConditionsCounts.progressing++
		}
	}
	return dnsStatusConditionsCounts
}

// computeOperatorStatusVersions computes the operator's current versions.
func (r *reconciler) computeOperatorStatusVersions(oldVersions []configv1.OperandVersion, dnses dnsStatusConditionsCounts) []configv1.OperandVersion {
	// We need to report old version until the operator fully transitions to the new version.
	// https://github.com/openshift/cluster-version-operator/blob/master/docs/dev/clusteroperator.md#version-reporting-during-an-upgrade
	if dnses.available != dnses.total {
		return oldVersions
	}

	return []configv1.OperandVersion{
		{
			Name:    OperatorVersionName,
			Version: r.OperatorReleaseVersion,
		},
		{
			Name:    CoreDNSVersionName,
			Version: r.CoreDNSImage,
		},
		{
			Name:    OpenshiftCLIVersionName,
			Version: r.OpenshiftCLIImage,
		},
	}
}

// computeOperatorStatusConditions computes the operator's current state.
func (r *reconciler) computeOperatorStatusConditions(oldConditions []configv1.ClusterOperatorStatusCondition,
	ns *corev1.Namespace, dnses dnsStatusConditionsCounts,
	oldVersions, curVersions []configv1.OperandVersion) []configv1.ClusterOperatorStatusCondition {
	var oldDegradedCondition, oldProgressingCondition, oldAvailableCondition *configv1.ClusterOperatorStatusCondition
	for i := range oldConditions {
		switch oldConditions[i].Type {
		case configv1.OperatorDegraded:
			oldDegradedCondition = &oldConditions[i]
		case configv1.OperatorProgressing:
			oldProgressingCondition = &oldConditions[i]
		case configv1.OperatorAvailable:
			oldAvailableCondition = &oldConditions[i]
		}
	}

	conditions := []configv1.ClusterOperatorStatusCondition{
		computeOperatorDegradedCondition(oldDegradedCondition, dnses, ns),
		r.computeOperatorProgressingCondition(oldProgressingCondition, dnses, oldVersions, curVersions),
		computeOperatorAvailableCondition(oldAvailableCondition, dnses),
	}

	return conditions
}

// computeOperatorDegradedCondition computes the operator's current Degraded status state.
func computeOperatorDegradedCondition(oldCondition *configv1.ClusterOperatorStatusCondition, dnses dnsStatusConditionsCounts, ns *corev1.Namespace) configv1.ClusterOperatorStatusCondition {
	degradedCondition := configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorDegraded,
	}
	switch {
	case ns == nil:
		degradedCondition.Status = configv1.ConditionTrue
		degradedCondition.Reason = "NoNamespace"
		degradedCondition.Message = "Operand Namespace does not exist"
	case dnses.available == 0:
		degradedCondition.Status = configv1.ConditionTrue
		degradedCondition.Reason = "NoDNS"
		degradedCondition.Message = "No DNS resource exists"
	case dnses.degraded > 0:
		degradedCondition.Status = configv1.ConditionTrue
		degradedCondition.Reason = "NotAllDNSesAvailable"
		degradedCondition.Message = "Not all desired DNS DaemonSets available"
	default:
		degradedCondition.Status = configv1.ConditionFalse
		degradedCondition.Reason = "AsExpected"
		degradedCondition.Message = "All desired DNS DaemonSets available and operand Namespace exists"
	}

	setOperatorLastTransitionTime(&degradedCondition, oldCondition)
	return degradedCondition
}

// computeOperatorProgressingCondition computes the operator's current Progressing status state.
func (r *reconciler) computeOperatorProgressingCondition(oldCondition *configv1.ClusterOperatorStatusCondition, dnses dnsStatusConditionsCounts, oldVersions, curVersions []configv1.OperandVersion) configv1.ClusterOperatorStatusCondition {
	progressingCondition := configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorProgressing,
	}

	progressing := false

	messages := []string{}
	if dnses.total == 0 {
		messages = append(messages, "No DNS resource exists.")
		progressing = true
	}
	if dnses.progressing > 0 {
		messages = append(messages, "Not all DNS DaemonSets available.")
		progressing = true
	}

	oldVersionsMap := make(map[string]string)
	for _, opv := range oldVersions {
		oldVersionsMap[opv.Name] = opv.Version
	}

	for _, opv := range curVersions {
		if oldVersion, ok := oldVersionsMap[opv.Name]; ok && oldVersion != opv.Version {
			messages = append(messages, fmt.Sprintf("Upgraded %s to %q.", opv.Name, opv.Version))
		}
		switch opv.Name {
		case OperatorVersionName:
			if opv.Version != r.OperatorReleaseVersion {
				messages = append(messages, fmt.Sprintf("Moving to release version %q.", r.OperatorReleaseVersion))
				progressing = true
			}
		case CoreDNSVersionName:
			if opv.Version != r.CoreDNSImage {
				messages = append(messages, fmt.Sprintf("Moving to coredns image version %q.", r.CoreDNSImage))
				progressing = true
			}
		case OpenshiftCLIVersionName:
			if opv.Version != r.OpenshiftCLIImage {
				messages = append(messages, fmt.Sprintf("Moving to openshift-cli image version %q.", r.OpenshiftCLIImage))
				progressing = true
			}
		}
	}

	if progressing {
		progressingCondition.Status = configv1.ConditionTrue
		progressingCondition.Reason = "Reconciling"
	} else {
		progressingCondition.Status = configv1.ConditionFalse
		progressingCondition.Reason = "AsExpected"
	}
	progressingCondition.Message = "Desired and available number of DNS DaemonSets are equal"
	if len(messages) > 0 {
		progressingCondition.Message = strings.Join(messages, "\n")
	}

	setOperatorLastTransitionTime(&progressingCondition, oldCondition)
	return progressingCondition
}

// computeOperatorAvailableCondition computes the operator's current Available status state.
func computeOperatorAvailableCondition(oldCondition *configv1.ClusterOperatorStatusCondition,
	dnses dnsStatusConditionsCounts) configv1.ClusterOperatorStatusCondition {
	availableCondition := configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorAvailable,
	}
	if dnses.available > 0 {
		availableCondition.Status = configv1.ConditionTrue
		availableCondition.Reason = "AsExpected"
		availableCondition.Message = "At least 1 DNS DaemonSet available"
	} else {
		availableCondition.Status = configv1.ConditionFalse
		availableCondition.Reason = "DNSUnavailable"
		availableCondition.Message = "No DNS DaemonSets available"
	}

	setOperatorLastTransitionTime(&availableCondition, oldCondition)
	return availableCondition
}

// setOperatorLastTransitionTime sets LastTransitionTime for the given condition.
// If the condition has changed, it will assign a new timestamp otherwise keeps the old timestamp.
func setOperatorLastTransitionTime(condition, oldCondition *configv1.ClusterOperatorStatusCondition) {
	if oldCondition != nil && condition.Status == oldCondition.Status &&
		condition.Reason == oldCondition.Reason && condition.Message == oldCondition.Message {
		condition.LastTransitionTime = oldCondition.LastTransitionTime
	} else {
		condition.LastTransitionTime = metav1.Now()
	}
}

// operatorStatusesEqual compares two ClusterOperatorStatus values.  Returns true
// if the provided ClusterOperatorStatus values should be considered equal for the
// purpose of determining whether an update is necessary, false otherwise.
func operatorStatusesEqual(a, b configv1.ClusterOperatorStatus) bool {
	conditionCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b configv1.ClusterOperatorStatusCondition) bool { return a.Type < b.Type }),
	}
	if !cmp.Equal(a.Conditions, b.Conditions, conditionCmpOpts...) {
		return false
	}

	relatedCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b configv1.ObjectReference) bool { return a.Name < b.Name }),
	}
	if !cmp.Equal(a.RelatedObjects, b.RelatedObjects, relatedCmpOpts...) {
		return false
	}

	versionsCmpOpts := []cmp.Option{
		cmpopts.EquateEmpty(),
		cmpopts.SortSlices(func(a, b configv1.OperandVersion) bool { return a.Name < b.Name }),
	}
	if !cmp.Equal(a.Versions, b.Versions, versionsCmpOpts...) {
		return false
	}

	return true
}
