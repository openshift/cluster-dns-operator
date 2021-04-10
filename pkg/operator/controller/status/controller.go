package status

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilclock "k8s.io/apimachinery/pkg/util/clock"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	OperatorVersionName     = "operator"
	CoreDNSVersionName      = "coredns"
	OpenshiftCLIVersionName = "openshift-cli"
	KubeRBACProxyName       = "kube-rbac-proxy"

	UnknownVersionValue = "unknown"
	controllerName      = "status_controller"

	dnsEqualConditionMessage = "desired and current number of DNSes are equal"
)

// clock is to enable unit testing
var clock utilclock.Clock = utilclock.RealClock{}

// reconciler handles the actual status reconciliation logic in response to
// events.
type reconciler struct {
	operatorconfig.Config

	client client.Client
	cache  cache.Cache
}

// New creates the status controller. This is the controller that handles all
// the logic for creating the ClusterOperator operator and updating its status.
//
// The controller watches DNS resources in the manager namespace and uses them
// to compute the operator status.
func New(mgr manager.Manager, config operatorconfig.Config) (controller.Controller, error) {
	reconciler := &reconciler{
		Config: config,
		client: mgr.GetClient(),
		cache:  mgr.GetCache(),
	}
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &operatorv1.DNS{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}
	return c, nil
}

// Reconcile computes the operator's current status and therefrom creates or
// updates the ClusterOperator resource for the operator.
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	co := &configv1.ClusterOperator{ObjectMeta: metav1.ObjectMeta{Name: operatorcontroller.DNSClusterOperatorName().Name}}
	if err := r.client.Get(ctx, operatorcontroller.DNSClusterOperatorName(), co); err != nil {
		if errors.IsNotFound(err) {
			initializeClusterOperator(co)
			if err := r.client.Create(ctx, co); err != nil {
				fmt.Printf("failed to create co %s: %v\n", co.Name, err)
				return reconcile.Result{}, fmt.Errorf("failed to create clusteroperator %s: %v", co.Name, err)
			}
		} else {
			return reconcile.Result{}, fmt.Errorf("failed to get clusteroperator %s: %v", co.Name, err)
		}
	}
	oldStatus := co.Status.DeepCopy()

	state, err := r.getOperatorState()
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get operator state: %v", err)
	}

	related := []configv1.ObjectReference{
		{
			Resource: "namespaces",
			Name:     r.Config.OperatorNamespace,
		},
		{
			Group:    operatorv1.GroupName,
			Resource: "dnses",
			Name:     "default",
		},
	}
	if state.haveNamespace {
		related = append(related, configv1.ObjectReference{
			Resource: "namespaces",
			Name:     state.namespace.Name,
		})
	}
	co.Status.RelatedObjects = related

	co.Status.Versions = r.computeOperatorStatusVersions(state.haveDNS, &state.dns, oldStatus.Versions)

	co.Status.Conditions = mergeConditions(co.Status.Conditions,
		computeOperatorAvailableCondition(state.haveDNS, &state.dns),
		computeOperatorProgressingCondition(
			state.haveDNS,
			&state.dns,
			oldStatus.Versions,
			co.Status.Versions,
			r.OperatorReleaseVersion,
			r.CoreDNSImage,
			r.OpenshiftCLIImage,
			r.KubeRBACProxyImage,
		),
		computeOperatorDegradedCondition(state.haveDNS, &state.dns),
	)

	if !operatorStatusesEqual(*oldStatus, co.Status) {
		if err := r.client.Status().Update(ctx, co); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update clusteroperator %s: %v", co.Name, err)
		}
	}

	return reconcile.Result{}, nil
}

// Populate versions and conditions in cluster operator status as CVO expects these fields.
func initializeClusterOperator(co *configv1.ClusterOperator) {
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
		{
			Name:    KubeRBACProxyName,
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
}

type operatorState struct {
	haveNamespace bool
	namespace     corev1.Namespace
	haveDNS       bool
	dns           operatorv1.DNS
}

// getOperatorState gets and returns the resources necessary to compute the
// operator's current state.
func (r *reconciler) getOperatorState() (operatorState, error) {
	var state operatorState

	name := types.NamespacedName{
		Name: operatorcontroller.DefaultOperandNamespace,
	}
	if err := r.client.Get(context.TODO(), name, &state.namespace); err != nil {
		if !errors.IsNotFound(err) {
			fmt.Printf("failed to get ns %s: %v\n", name, err)
			return state, fmt.Errorf("failed to get namespace %q: %w", name, err)
		}
		state.haveNamespace = false
	} else {
		state.haveNamespace = true
	}

	dnsList := operatorv1.DNSList{}
	// TODO: Change to a get call when the following issue is resolved:
	//       https://github.com/kubernetes-sigs/controller-runtime/issues/934
	if err := r.cache.List(context.TODO(), &dnsList); err != nil {
		return state, fmt.Errorf("failed to list dnses: %w", err)
	} else if len(dnsList.Items) > 0 {
		state.haveDNS = true
		state.dns = dnsList.Items[0]
	}

	return state, nil
}

// computeOperatorStatusVersions computes the operator's current versions.
func (r *reconciler) computeOperatorStatusVersions(haveDNS bool, dns *operatorv1.DNS, oldVersions []configv1.OperandVersion) []configv1.OperandVersion {
	// We need to report old version until the operator fully transitions to the new version.
	// https://github.com/openshift/cluster-version-operator/blob/master/docs/dev/clusteroperator.md#version-reporting-during-an-upgrade
	if !haveDNS {
		return oldVersions
	}
	for _, c := range dns.Status.Conditions {
		if c.Type != operatorv1.OperatorStatusTypeProgressing {
			continue
		}
		switch c.Status {
		case operatorv1.ConditionTrue, operatorv1.ConditionUnknown:
			return oldVersions
		}
		break
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
		{
			Name:    KubeRBACProxyName,
			Version: r.KubeRBACProxyImage,
		},
	}
}

// checkDNSAvailable checks if the dns is available.
func checkDNSAvailable(dns *operatorv1.DNS) bool {
	for _, c := range dns.Status.Conditions {
		if c.Type == operatorv1.OperatorStatusTypeAvailable && c.Status == operatorv1.ConditionTrue {
			return true
		}
	}

	return false
}

// computeOperatorDegradedCondition computes the operator's current Degraded status state.
func computeOperatorDegradedCondition(haveDNS bool, dns *operatorv1.DNS) configv1.ClusterOperatorStatusCondition {
	if !haveDNS {
		return configv1.ClusterOperatorStatusCondition{
			Type:    configv1.OperatorDegraded,
			Status:  configv1.ConditionTrue,
			Reason:  "DNSDoesNotExist",
			Message: `DNS "default" does not exist.`,
		}
	}

	var degraded bool
	for _, cond := range dns.Status.Conditions {
		if cond.Type == operatorv1.OperatorStatusTypeDegraded && cond.Status == operatorv1.ConditionTrue {
			degraded = true
		}
	}
	if degraded {
		return configv1.ClusterOperatorStatusCondition{
			Type:    configv1.OperatorDegraded,
			Status:  configv1.ConditionTrue,
			Reason:  "DNSDegraded",
			Message: fmt.Sprintf("DNS %s is degraded", dns.Name),
		}
	}
	return configv1.ClusterOperatorStatusCondition{
		Type:   configv1.OperatorDegraded,
		Status: configv1.ConditionFalse,
		Reason: "DNSNotDegraded",
	}
}

// computeOperatorProgressingCondition computes the operator's current Progressing status state.
func computeOperatorProgressingCondition(haveDNS bool, dns *operatorv1.DNS, oldVersions, curVersions []configv1.OperandVersion, operatorReleaseVersion, coreDNSImage, openshiftCLIImage, kubeRBACProxyImage string) configv1.ClusterOperatorStatusCondition {
	progressingCondition := configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorProgressing,
	}
	status := configv1.ConditionUnknown
	var messages, progressingReasons []string

	if !haveDNS {
		status = configv1.ConditionTrue
		progressingReasons = append(progressingReasons, "DNSDoesNotExist")
		messages = append(messages, `DNS "default" does not exist`)
	} else {
		foundProgressingCondition := false
		for _, cond := range dns.Status.Conditions {
			if cond.Type != operatorv1.OperatorStatusTypeProgressing {
				continue
			}
			foundProgressingCondition = true
			switch cond.Status {
			case operatorv1.ConditionTrue:
				status = configv1.ConditionTrue
				progressingReasons = append(progressingReasons, "DNSReportsProgressingIsTrue")
				messages = append(messages, fmt.Sprintf("DNS %q reports Progressing=True: %q", dns.Name, cond.Message))
			case operatorv1.ConditionUnknown:
				progressingReasons = append(progressingReasons, "DNSReportsProgressingIsUnknown")
				messages = append(messages, fmt.Sprintf("DNS %q reports Progressing=Unknown: %q", dns.Name, cond.Message))
			}
			break
		}
		if !foundProgressingCondition {
			progressingReasons = append(progressingReasons, "DNSDoesNotReportProgressingStatus")
			messages = append(messages, fmt.Sprintf("DNS %q is not reporting a Progressing status condition", dns.Name))
		}
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
			if opv.Version != operatorReleaseVersion {
				status = configv1.ConditionTrue
				progressingReasons = append(progressingReasons, "UpgradingOperator")
				messages = append(messages, fmt.Sprintf("Moving to release version %q.", operatorReleaseVersion))
			}
		case CoreDNSVersionName:
			if opv.Version != coreDNSImage {
				status = configv1.ConditionTrue
				progressingReasons = append(progressingReasons, "UpgradingCoreDNS")
				messages = append(messages, fmt.Sprintf("Moving to coredns image version %q.", coreDNSImage))
			}
		case OpenshiftCLIVersionName:
			if opv.Version != openshiftCLIImage {
				status = configv1.ConditionTrue
				progressingReasons = append(progressingReasons, "UpgradingOpenShiftCLI")
				messages = append(messages, fmt.Sprintf("Moving to openshift-cli image version %q.", openshiftCLIImage))
			}
		case KubeRBACProxyName:
			if opv.Version != kubeRBACProxyImage {
				status = configv1.ConditionTrue
				progressingReasons = append(progressingReasons, "UpgradingKubeRBACProxy")
				messages = append(messages, fmt.Sprintf("Moving to kube-rbac-proxy image version %q.", kubeRBACProxyImage))
			}
		}
	}

	if len(progressingReasons) != 0 {
		progressingCondition.Status = status
		progressingCondition.Reason = strings.Join(progressingReasons, "And")
		progressingCondition.Message = strings.Join(messages, "\n")
	} else {
		progressingCondition.Status = configv1.ConditionFalse
		progressingCondition.Reason = "AsExpected"
		progressingCondition.Message = dnsEqualConditionMessage
	}

	return progressingCondition
}

// computeOperatorAvailableCondition computes the operator's current Available status state.
func computeOperatorAvailableCondition(haveDNS bool, dns *operatorv1.DNS) configv1.ClusterOperatorStatusCondition {
	availableCondition := configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorAvailable,
	}

	switch {
	case !haveDNS:
		availableCondition.Status = configv1.ConditionFalse
		availableCondition.Reason = "DNSDoesNotExist"
		availableCondition.Message = `DNS "default" does not exist.`
	case !checkDNSAvailable(dns):
		availableCondition.Status = configv1.ConditionFalse
		availableCondition.Reason = "DNSUnavailable"
		availableCondition.Message = `DNS "default" is unavailable.`
	default:
		availableCondition.Status = configv1.ConditionTrue
		availableCondition.Reason = "AsExpected"
		availableCondition.Message = `DNS "default" is available.`
	}

	return availableCondition
}

// mergeConditions adds or updates matching conditions, and updates
// the transition time if details of a condition have changed. Returns
// the updated condition array.
func mergeConditions(conditions []configv1.ClusterOperatorStatusCondition, updates ...configv1.ClusterOperatorStatusCondition) []configv1.ClusterOperatorStatusCondition {
	now := metav1.NewTime(clock.Now())
	var additions []configv1.ClusterOperatorStatusCondition
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

func conditionChanged(a, b configv1.ClusterOperatorStatusCondition) bool {
	return a.Status != b.Status || a.Reason != b.Reason || a.Message != b.Message
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
