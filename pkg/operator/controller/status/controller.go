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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilclock "k8s.io/apimachinery/pkg/util/clock"

	podutil "k8s.io/kubectl/pkg/util/podutils"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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
// to compute the operator status.  It also watches the clusteroperators
// resource so that it reconciles the dns clusteroperator in case something else
// updates or deletes it.
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

	if err := c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{OwnerType: &operatorv1.DNS{}}); err != nil {
		return nil, err
	}

	isDNSClusterOperator := func(o client.Object) bool {
		return o.GetName() == operatorcontroller.DefaultOperatorName
	}
	clusteroperatorToDNS := func(_ client.Object) []reconcile.Request {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name: operatorcontroller.DefaultDNSName,
			},
		}}
	}
	if err := c.Watch(
		&source.Kind{Type: &configv1.ClusterOperator{}},
		handler.EnqueueRequestsFromMapFunc(clusteroperatorToDNS),
		predicate.NewPredicateFuncs(isDNSClusterOperator),
	); err != nil {
		return nil, err
	}

	return c, nil
}

// Reconcile computes the operator's current status and therefrom creates or
// updates the ClusterOperator resource for the operator.
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	co := &configv1.ClusterOperator{}
	name := operatorcontroller.DNSClusterOperatorName()
	if err := r.client.Get(ctx, name, co); err != nil {
		if errors.IsNotFound(err) {
			initializeClusterOperator(co)
			if err := r.client.Create(ctx, co); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to create clusteroperator %q: %w", name.Name, err)
			}
		} else {
			return reconcile.Result{}, fmt.Errorf("failed to get clusteroperator %q: %w", name.Name, err)
		}
	}
	oldStatus := co.Status.DeepCopy()

	state, err := r.getOperatorState()
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to get operator state: %w", err)
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

	// oldVersions is what's reported in status now.
	oldVersions := computeOldVersions(co.Status.Versions)
	// newVersions is the versions with which the operator is configured.
	// If these differ from oldVersions, then these are the versions to
	// which the operator is trying to upgrade.
	newVersions := computeNewVersions(
		r.OperatorReleaseVersion,
		r.CoreDNSImage,
		r.OpenshiftCLIImage,
		r.KubeRBACProxyImage,
	)
	// curVersions is the versions that the operator can presently report
	// as "current".  For operands, the current version is set to the new
	// version once the desired number of pods are running the new version.
	// For the operator itself, the current version is the last version of
	// the operator that observed all its operands' current versions to be
	// equal to their respective new versions.
	curVersions := computeCurrentVersions(
		oldVersions,
		newVersions,
		&state.dnsDaemonSet,
		&state.nodeResolverDaemonSet,
		state.dnsPodList.Items,
		state.nodeResolverPodList.Items,
	)

	operatorProgressingCondition := computeOperatorProgressingCondition(
		state.haveDNS,
		&state.dns,
		oldVersions,
		newVersions,
		curVersions,
	)
	co.Status.Conditions = mergeConditions(co.Status.Conditions,
		computeOperatorAvailableCondition(state.haveDNS, &state.dns),
		operatorProgressingCondition,
		computeOperatorDegradedCondition(state.haveDNS, &state.dns),
	)
	co.Status.Versions = computeOperatorStatusVersions(curVersions)

	if !operatorStatusesEqual(*oldStatus, co.Status) {
		if err := r.client.Status().Update(ctx, co); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to update clusteroperator %q: %w", name.Name, err)
		}
	}

	return reconcile.Result{}, nil
}

// Populate versions and conditions in cluster operator status as CVO expects these fields.
func initializeClusterOperator(co *configv1.ClusterOperator) {
	co.Name = operatorcontroller.DNSClusterOperatorName().Name
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
	// haveNamespace indicates whether the operand namespace exists.
	haveNamespace bool

	// namespace is the operand namespace.
	namespace corev1.Namespace

	// haveDNS indicates whether the "default" dnses.operator.openshift.io
	// CR exists.
	haveDNS bool

	// dns is the "default" dnses.operator.openshift.io CR if it exists.
	dns operatorv1.DNS

	// dnsDaemonSet is the "dns-default" daemonset if it exists.  If the
	// daemonset does not exist, dnsDaemonSet will have the zero value.
	dnsDaemonSet appsv1.DaemonSet

	// nodeResolverDaemonSet is the "node-resolver" daemonset if it exists.
	// If the daemonset does not exist, nodeResolverDaemonSet will have the
	// zero value.
	nodeResolverDaemonSet appsv1.DaemonSet

	// dnsPodList is the list of pods belonging to dnsDaemonSet.
	dnsPodList corev1.PodList

	// nodeResolverPodList is the list of pods belonging to
	// nodeResolverDaemonSet.
	nodeResolverPodList corev1.PodList
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

	nodeResolverDaemonSetName := operatorcontroller.NodeResolverDaemonSetName()
	if err := r.cache.Get(context.TODO(), nodeResolverDaemonSetName, &state.nodeResolverDaemonSet); err != nil {
		if !errors.IsNotFound(err) {
			return state, fmt.Errorf("failed to get node-resolver daemonset: %w", err)
		}
	}

	nodeResolverLabelSelector, err := metav1.LabelSelectorAsSelector(operatorcontroller.NodeResolverDaemonSetPodSelector())
	if err != nil {
		return state, err
	}

	nodeResolverPodsListOpts := []client.ListOption{
		client.MatchingLabelsSelector{
			Selector: nodeResolverLabelSelector,
		},
		client.InNamespace(operatorcontroller.DefaultOperandNamespace),
	}
	if err := r.cache.List(context.TODO(), &state.nodeResolverPodList, nodeResolverPodsListOpts...); err != nil {
		return state, fmt.Errorf("failed to list node-resolver pods: %w", err)
	}

	// We cannot look up the daemonset and pods without the dns.
	if !state.haveDNS {
		return state, nil
	}

	dnsDaemonSetName := operatorcontroller.DNSDaemonSetName(&state.dns)
	if err := r.cache.Get(context.TODO(), dnsDaemonSetName, &state.dnsDaemonSet); err != nil {
		if !errors.IsNotFound(err) {
			return state, fmt.Errorf("failed to get dns daemonset: %w", err)
		}
	}

	dnsLabelSelector, err := metav1.LabelSelectorAsSelector(operatorcontroller.DNSDaemonSetPodSelector(&state.dns))
	if err != nil {
		return state, err
	}

	dnsPodsListOpts := []client.ListOption{
		client.MatchingLabelsSelector{
			Selector: dnsLabelSelector,
		},
		client.InNamespace(operatorcontroller.DefaultOperandNamespace),
	}
	if err := r.cache.List(context.TODO(), &state.dnsPodList, dnsPodsListOpts...); err != nil {
		return state, fmt.Errorf("failed to list dns pods: %w", err)
	}

	return state, nil
}

// computeOperatorStatusVersions computes the operator's current versions.
func computeOperatorStatusVersions(newVersions map[string]string) []configv1.OperandVersion {
	versions := make([]configv1.OperandVersion, len(newVersions))
	i := 0
	for n, v := range newVersions {
		versions[i] = configv1.OperandVersion{Name: n, Version: v}
		i++
	}
	return versions
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
func computeOperatorProgressingCondition(haveDNS bool, dns *operatorv1.DNS, oldVersions, newVersions, curVersions map[string]string) configv1.ClusterOperatorStatusCondition {
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

	upgrading := false
	for name, curVersion := range curVersions {
		if oldVersion, ok := oldVersions[name]; ok && oldVersion != curVersion {
			messages = append(messages, fmt.Sprintf("Upgraded %s to %q.", name, curVersion))
		}
		if newVersion, ok := newVersions[name]; ok && curVersion != newVersion {
			upgrading = true
			messages = append(messages, fmt.Sprintf("Upgrading %s to %q.", name, newVersion))
		}
	}
	if upgrading {
		status = configv1.ConditionTrue
		progressingReasons = append(progressingReasons, "Upgrading")
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

// computeOldVersions returns a map of operand name to version computed from the
// given clusteroperator status.
func computeOldVersions(oldVersions []configv1.OperandVersion) map[string]string {
	result := make(map[string]string, 4)
	for _, v := range oldVersions {
		result[v.Name] = v.Version
	}
	return result
}

// computeNewVersions returns a map of operand name to version computed from the
// given reconciler versions.
func computeNewVersions(operatorReleaseVersion, coreDNSImage, openshiftCLIImage, kubeRBACProxyImage string) map[string]string {
	return map[string]string{
		OperatorVersionName:     operatorReleaseVersion,
		CoreDNSVersionName:      coreDNSImage,
		OpenshiftCLIVersionName: openshiftCLIImage,
		KubeRBACProxyName:       kubeRBACProxyImage,
	}
}

// computeCurrentVersions returns a map of operand name to version computed from
// the operand pods and oldVersions and newVersions maps.
func computeCurrentVersions(oldVersions, newVersions map[string]string, dnsDaemonSet, nodeResolverDaemonSet *appsv1.DaemonSet, dnsPods, nodeResolverPods []corev1.Pod) map[string]string {
	var (
		operatorVersion      = oldVersions[OperatorVersionName]
		coreDNSVersion       = oldVersions[CoreDNSVersionName]
		openshiftCLIVersion  = oldVersions[OpenshiftCLIVersionName]
		kubeRBACProxyVersion = oldVersions[KubeRBACProxyName]
	)

	// Compute the number of instances of each operand that are at the new
	// version.
	available := map[string]int{}
	versionNameForContainerName := map[string]string{
		"dns":               CoreDNSVersionName,
		"kube-rbac-proxy":   KubeRBACProxyName,
		"dns-node-resolver": OpenshiftCLIVersionName,
	}
	updateAvailableFromPod := func(pod *corev1.Pod) {
		for _, container := range pod.Spec.Containers {
			versionName, ok := versionNameForContainerName[container.Name]
			if !ok {
				continue
			}
			if container.Image == newVersions[versionName] {
				available[versionName]++
			}
		}
	}
	now := metav1.Time{Time: clock.Now()}
	for _, pod := range dnsPods {
		if !podutil.IsPodAvailable(&pod, dnsDaemonSet.Spec.MinReadySeconds, now) {
			continue
		}
		updateAvailableFromPod(&pod)
	}
	for _, pod := range nodeResolverPods {
		if !podutil.IsPodAvailable(&pod, nodeResolverDaemonSet.Spec.MinReadySeconds, now) {
			continue
		}
		updateAvailableFromPod(&pod)
	}
	// If the desired number of pods are at the new version, bump the
	// reported version.  Otherwise, keep the old version.
	desiredDNS := int(dnsDaemonSet.Status.DesiredNumberScheduled)
	coreDNSLevel := available[CoreDNSVersionName] >= desiredDNS
	kubeRBACProxyLevel := available[KubeRBACProxyName] >= desiredDNS
	if coreDNSLevel && kubeRBACProxyLevel {
		coreDNSVersion = newVersions[CoreDNSVersionName]
		kubeRBACProxyVersion = newVersions[KubeRBACProxyName]
	}
	desiredNR := int(nodeResolverDaemonSet.Status.DesiredNumberScheduled)
	openshiftCLILevel := available[OpenshiftCLIVersionName] >= desiredNR
	if openshiftCLILevel {
		openshiftCLIVersion = newVersions[OpenshiftCLIVersionName]
	}
	// Bump the reported operator version if operands are all bumped.
	if coreDNSLevel && kubeRBACProxyLevel && openshiftCLILevel {
		operatorVersion = newVersions[OperatorVersionName]
	}

	return map[string]string{
		OperatorVersionName:     operatorVersion,
		CoreDNSVersionName:      coreDNSVersion,
		OpenshiftCLIVersionName: openshiftCLIVersion,
		KubeRBACProxyName:       kubeRBACProxyVersion,
	}
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
