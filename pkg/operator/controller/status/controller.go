package status

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	utilclock "k8s.io/apimachinery/pkg/util/clock"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

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
	nsManifest := manifests.DNSNamespace()

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

	state, err := r.getOperatorState(nsManifest.Name)
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
	if state.Namespace != nil {
		related = append(related, configv1.ObjectReference{
			Resource: "namespaces",
			Name:     state.Namespace.Name,
		})
	}
	co.Status.RelatedObjects = related

	dnsAvailable := checkDNSAvailable(&state.DNS)

	co.Status.Versions = r.computeOperatorStatusVersions(oldStatus.Versions, dnsAvailable)

	co.Status.Conditions = mergeConditions(co.Status.Conditions, computeOperatorAvailableCondition(dnsAvailable))
	co.Status.Conditions = mergeConditions(co.Status.Conditions, computeOperatorProgressingCondition(dnsAvailable,
		oldStatus.Versions, co.Status.Versions, r.OperatorReleaseVersion, r.CoreDNSImage,
		r.OpenshiftCLIImage, r.KubeRBACProxyImage))
	co.Status.Conditions = mergeConditions(co.Status.Conditions, computeOperatorDegradedCondition(&state.DNS))

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
	Namespace *corev1.Namespace
	DNS       operatorv1.DNS
}

// getOperatorState gets and returns the resources necessary to compute the
// operator's current state.
func (r *reconciler) getOperatorState(nsName string) (operatorState, error) {
	state := operatorState{}

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: nsName}, ns); err != nil {
		if !errors.IsNotFound(err) {
			fmt.Printf("failed to get ns %s: %v\n", nsName, err)
			return state, fmt.Errorf("failed to get namespace %q: %v", nsName, err)
		}
	} else {
		state.Namespace = ns
	}

	dnsList := operatorv1.DNSList{}
	// TODO: Change to a get call when the following issue is resolved:
	//       https://github.com/kubernetes-sigs/controller-runtime/issues/934
	if err := r.cache.List(context.TODO(), &dnsList); err != nil {
		return state, fmt.Errorf("failed to list dnses: %v", err)
	} else {
		state.DNS = dnsList.Items[0]
	}

	return state, nil
}

// computeOperatorStatusVersions computes the operator's current versions.
func (r *reconciler) computeOperatorStatusVersions(oldVersions []configv1.OperandVersion, allDNSESAvailable bool) []configv1.OperandVersion {
	// We need to report old version until the operator fully transitions to the new version.
	// https://github.com/openshift/cluster-version-operator/blob/master/docs/dev/clusteroperator.md#version-reporting-during-an-upgrade
	if !allDNSESAvailable {
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
func computeOperatorDegradedCondition(dns *operatorv1.DNS) configv1.ClusterOperatorStatusCondition {
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
func computeOperatorProgressingCondition(dnsAvailable bool, oldVersions, curVersions []configv1.OperandVersion,
	operatorReleaseVersion, coreDNSImage, openshiftCLIImage, kubeRBACProxyImage string) configv1.ClusterOperatorStatusCondition {
	// TODO: Update progressingCondition when an ingresscontroller
	//       progressing condition is created. The Operator's condition
	//       should be derived from the ingresscontroller's condition.
	progressingCondition := configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorProgressing,
	}

	progressing := false

	var messages []string
	if !dnsAvailable {
		messages = append(messages, "DNS default unavailable")
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
			if opv.Version != operatorReleaseVersion {
				messages = append(messages, fmt.Sprintf("Moving to release version %q.", operatorReleaseVersion))
				progressing = true
			}
		case CoreDNSVersionName:
			if opv.Version != coreDNSImage {
				messages = append(messages, fmt.Sprintf("Moving to coredns image version %q.", coreDNSImage))
				progressing = true
			}
		case OpenshiftCLIVersionName:
			if opv.Version != openshiftCLIImage {
				messages = append(messages, fmt.Sprintf("Moving to openshift-cli image version %q.", openshiftCLIImage))
				progressing = true
			}
		case KubeRBACProxyName:
			if opv.Version != kubeRBACProxyImage {
				messages = append(messages, fmt.Sprintf("Moving to kube-rbac-proxy image version %q.", kubeRBACProxyImage))
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
	progressingCondition.Message = dnsEqualConditionMessage
	if len(messages) > 0 {
		progressingCondition.Message = strings.Join(messages, "\n")
	}

	return progressingCondition
}

// computeOperatorAvailableCondition computes the operator's current Available status state.
func computeOperatorAvailableCondition(dnsAvailable bool) configv1.ClusterOperatorStatusCondition {
	availableCondition := configv1.ClusterOperatorStatusCondition{
		Type: configv1.OperatorAvailable,
	}
	if dnsAvailable {
		availableCondition.Status = configv1.ConditionTrue
		availableCondition.Reason = "AsExpected"
		availableCondition.Message = "DNS default is available"
	} else {
		availableCondition.Status = configv1.ConditionFalse
		availableCondition.Reason = "DNSUnavailable"
		availableCondition.Message = "DNS default is unavailable"
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
