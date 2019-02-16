package stub

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	configv1 "github.com/openshift/api/config/v1"
	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/util/clusteroperator"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/coreos/go-semver/semver"
)

// syncOperatorStatus computes the operator's current status and therefrom
// creates or updates the ClusterOperator resource for the operator.
func (h *Handler) syncOperatorStatus() error {
	co := &configv1.ClusterOperator{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterOperator",
			APIVersion: configv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dns",
		},
	}

	mustCreate := false
	if err := sdk.Get(co); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("syncOperatorStatus: failed to get ClusterOperator %q: %v", co.Name, err)
		}
		mustCreate = true
	}

	ns, dnses, daemonsets, err := h.getOperatorState()
	if err != nil {
		return fmt.Errorf("syncOperatorStatus: failed to get operator state: %v", err)
	}

	oldConditions := co.Status.Conditions
	co.Status.Conditions = computeStatusConditions(oldConditions, ns, dnses, daemonsets)

	oldRelatedObjects := co.Status.RelatedObjects
	co.Status.RelatedObjects = []configv1.ObjectReference{
		{
			Resource: "namespaces",
			Name:     "openshift-dns-operator",
		},
		{
			Resource: "namespaces",
			Name:     ns.Name,
		},
	}

	versionMap, err := getVersionMap()
	if err != nil {
		return fmt.Errorf("syncOperatorStatus: failed to get version map: %v", err)
	}

	oldVersions := co.Status.Versions
	if co.Status.Versions, err = computeStatusVersions(h.Config.OperatorImageVersion, daemonsets, versionMap); err != nil {
		return fmt.Errorf("syncOperatorStatus: failed to compute status versions: %v", err)
	}

	if mustCreate {
		if err := sdk.Create(co); err != nil {
			return fmt.Errorf("syncOperatorStatus: failed to create ClusterOperator %q: %v", co.Name, err)
		}
		logrus.Infof("syncOperatorStatus: created ClusterOperator %q (UID %v)", co.Name, co.UID)
		if err := sdk.Get(co); err != nil {
			return fmt.Errorf("syncOperatorStatus: error getting ClusterOperator %q: %v", co.Name, err)
		}
	}

	if clusteroperator.ConditionsEqual(oldConditions, co.Status.Conditions) &&
		clusteroperator.ObjectReferencesEqual(oldRelatedObjects, co.Status.RelatedObjects) &&
		clusteroperator.VersionsEqual(oldVersions, co.Status.Versions) {
		return nil
	}

	unstructObj, err := k8sutil.UnstructuredFromRuntimeObject(co)
	if err != nil {
		return fmt.Errorf("syncOperatorStatus: failed to convert ClusterOperator %q: %v\n%#v", co.Name, err, co)
	}

	resourceClient, _, err := k8sclient.GetResourceClient(co.APIVersion, co.Kind, co.Namespace)
	if err != nil {
		return fmt.Errorf("syncOperatorStatus: failed to get resource client: %v", err)
	}

	if _, err := resourceClient.UpdateStatus(unstructObj); err != nil {
		return fmt.Errorf("syncOperatorStatus: failed to update status of %q: %v", co.Name, err)
	}
	logrus.Infof("syncOperatorStatus: updated status of %q", co.Name)
	return nil
}

// getOperatorState gets and returns the resources necessary to compute the
// operator's current state.
func (h *Handler) getOperatorState() (*corev1.Namespace, []dnsv1alpha1.ClusterDNS, []appsv1.DaemonSet, error) {
	ns, err := h.ManifestFactory.DNSNamespace()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error building Namespace: %v", err)
	}

	if err := sdk.Get(ns); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("error getting Namespace %q: %v", ns.Name, err)
	}

	dnsList := &dnsv1alpha1.ClusterDNSList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDNS",
			APIVersion: dnsv1alpha1.SchemeGroupVersion.String(),
		},
	}
	if err := sdk.List(corev1.NamespaceAll, dnsList, sdk.WithListOptions(&metav1.ListOptions{})); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list ClusterDNSes: %v", err)
	}

	daemonsetList := &appsv1.DaemonSetList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
	}
	if err := sdk.List(ns.Name, daemonsetList, sdk.WithListOptions(&metav1.ListOptions{})); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list DaemonSets: %v", err)
	}

	return ns, dnsList.Items, daemonsetList.Items, nil
}

// computeStatusConditions computes the operator's current state.
func computeStatusConditions(conditions []configv1.ClusterOperatorStatusCondition, ns *corev1.Namespace, dnses []dnsv1alpha1.ClusterDNS, daemonsets []appsv1.DaemonSet) []configv1.ClusterOperatorStatusCondition {
	failingCondition := &configv1.ClusterOperatorStatusCondition{
		Type:   configv1.OperatorFailing,
		Status: configv1.ConditionUnknown,
	}
	if ns == nil {
		failingCondition.Status = configv1.ConditionTrue
		failingCondition.Reason = "NoNamespace"
		failingCondition.Message = "DNS namespace does not exist"
	} else {
		failingCondition.Status = configv1.ConditionFalse
	}
	conditions = clusteroperator.SetStatusCondition(conditions, failingCondition)

	progressingCondition := &configv1.ClusterOperatorStatusCondition{
		Type:   configv1.OperatorProgressing,
		Status: configv1.ConditionUnknown,
	}
	numClusterDNSes := len(dnses)
	numDaemonsets := len(daemonsets)
	if numClusterDNSes == numDaemonsets {
		progressingCondition.Status = configv1.ConditionFalse
	} else {
		progressingCondition.Status = configv1.ConditionTrue
		progressingCondition.Reason = "Reconciling"
		progressingCondition.Message = fmt.Sprintf("have %d DaemonSets, want %d", numDaemonsets, numClusterDNSes)
	}
	conditions = clusteroperator.SetStatusCondition(conditions, progressingCondition)

	availableCondition := &configv1.ClusterOperatorStatusCondition{
		Type:   configv1.OperatorAvailable,
		Status: configv1.ConditionUnknown,
	}
	dsAvailable := map[string]bool{}
	for _, ds := range daemonsets {
		dsAvailable[ds.Name] = ds.Status.NumberAvailable > 0
	}
	unavailable := []string{}
	for _, dns := range dnses {
		// TODO Use the manifest to derive the name, or use labels or
		// owner references.
		name := "dns-" + dns.Name
		if available, exists := dsAvailable[name]; !exists {
			msg := fmt.Sprintf("no DaemonSet for ClusterDNS %q", dns.Name)
			unavailable = append(unavailable, msg)
		} else if !available {
			msg := fmt.Sprintf("DaemonSet %q not available", name)
			unavailable = append(unavailable, msg)
		}
	}
	if len(unavailable) == 0 {
		availableCondition.Status = configv1.ConditionTrue
	} else {
		availableCondition.Status = configv1.ConditionFalse
		availableCondition.Reason = "DaemonSetNotAvailable"
		availableCondition.Message = strings.Join(unavailable, "\n")
	}
	conditions = clusteroperator.SetStatusCondition(conditions, availableCondition)

	return conditions
}

// getVersionMap returns the version map for operator and operands.
func getVersionMap() (map[string]string, error) {
	versionMapping := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "version-mapping",
			Namespace: "openshift-dns-operator",
		},
	}
	if err := sdk.Get(versionMapping); err != nil {
		return nil, fmt.Errorf("getVersionMap: error getting configmap %s/%s: %v", versionMapping.Namespace, versionMapping.Name, err)
	}

	versionMap := map[string]string{}
	for version, image := range versionMapping.Data {
		versionMap[image] = version
	}
	return versionMap, nil
}

// computeStatusVersions computes the operator's and operands' current available
// versions.
func computeStatusVersions(operatorVersion string, daemonsets []appsv1.DaemonSet, versionMap map[string]string) ([]configv1.OperandVersion, error) {
	// Find the oldest available version of each operand.  If some but not
	// all instances of an operand have a newer version, this indicates that
	// the operator is trying to roll out, but has not finished rolling out,
	// the new version.  Versions should reflect the currently available
	// version of each operand even if the operator is trying to roll out a
	// newer version.
	var (
		coreDNSSemVer  *semver.Version
		coreDNSVersion string

		openshiftCLISemVer  *semver.Version
		openshiftCLIVersion string
	)
	for _, ds := range daemonsets {
		if ds.Status.NumberAvailable == 0 {
			continue
		}

		coreDNSPullspec := ds.Spec.Template.Spec.Containers[0].Image
		if sv, v, err := getSemVerForPullspec(coreDNSPullspec, versionMap); err != nil {
			return []configv1.OperandVersion{}, fmt.Errorf("failed to get CoreDNS version for daemonset %s/%s: %v", ds.Namespace, ds.Name, err)
		} else if coreDNSSemVer == nil || sv.LessThan(*coreDNSSemVer) {
			coreDNSSemVer, coreDNSVersion = sv, v
		}

		osCLIPullspec := ds.Spec.Template.Spec.Containers[1].Image
		if sv, v, err := getSemVerForPullspec(osCLIPullspec, versionMap); err != nil {
			return []configv1.OperandVersion{}, fmt.Errorf("failed to get OpenShift client version for daemonset %s/%s: %v", ds.Namespace, ds.Name, err)
		} else if openshiftCLISemVer == nil || sv.LessThan(*openshiftCLISemVer) {
			openshiftCLISemVer, openshiftCLIVersion = sv, v
		}
	}

	versions := []configv1.OperandVersion{}
	if operatorVersion != "" {
		version := configv1.OperandVersion{
			Name:    "operator",
			Version: operatorVersion,
		}
		versions = append(versions, version)
	}
	if coreDNSVersion != "" {
		version := configv1.OperandVersion{
			Name:    "coredns",
			Version: coreDNSVersion,
		}
		versions = append(versions, version)
	}
	if openshiftCLIVersion != "" {
		version := configv1.OperandVersion{
			Name:    "node-resolver",
			Version: openshiftCLIVersion,
		}
		versions = append(versions, version)
	}
	return versions, nil

}

// getSemVerForPullspec gets the version string and semantic version of the given
// pullspec using the provided version map.
func getSemVerForPullspec(pullspec string, versionMap map[string]string) (*semver.Version, string, error) {
	version, ok := versionMap[pullspec]
	if !ok {
		return nil, "", fmt.Errorf("failed to look up pullspec %q", pullspec)
	}

	dottedTri := version
	if i := strings.Index(dottedTri, "_"); i >= 0 {
		dottedTri = dottedTri[:i]
	}

	sv, err := semver.NewVersion(dottedTri)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse semver %q extracted from version %q for pullspec %q: %v", dottedTri, version, pullspec, err)
	}

	return sv, version, nil
}
