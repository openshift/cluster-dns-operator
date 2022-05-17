package controller

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// enableDaemonSetEvictionAnnotationKey is the annotation key for the
	// annotation that enables eviction of a daemonset by the cluster
	// autoscaler.
	enableDaemonSetEvictionAnnotationKey = "cluster-autoscaler.kubernetes.io/enable-ds-eviction"
)

var (
	// managedDNSDaemonSetAnnotations is a set of annotation keys for
	// annotations that the operator manages for the DNS daemonset.
	managedDNSDaemonSetAnnotations = sets.NewString(enableDaemonSetEvictionAnnotationKey)
)

// ensureDNSDaemonSet ensures the dns daemonset exists for a given dns.
func (r *reconciler) ensureDNSDaemonSet(dns *operatorv1.DNS) (bool, *appsv1.DaemonSet, error) {
	haveDS, current, err := r.currentDNSDaemonSet(dns)
	if err != nil {
		return false, nil, err
	}
	desired, err := desiredDNSDaemonSet(dns, r.CoreDNSImage, r.KubeRBACProxyImage)
	if err != nil {
		return haveDS, current, fmt.Errorf("failed to build dns daemonset: %v", err)
	}
	switch {
	case !haveDS:
		if err := r.createDNSDaemonSet(desired); err != nil {
			return false, nil, err
		}
		return r.currentDNSDaemonSet(dns)
	case haveDS:
		if updated, err := r.updateDNSDaemonSet(current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentDNSDaemonSet(dns)
		}
	}
	return true, current, nil
}

// ensureDNSDaemonSetDeleted ensures deletion of daemonset and related resources
// associated with the dns.
func (r *reconciler) ensureDNSDaemonSetDeleted(dns *operatorv1.DNS) error {
	daemonset := &appsv1.DaemonSet{}
	name := DNSDaemonSetName(dns)
	daemonset.Name = name.Name
	daemonset.Namespace = name.Namespace
	if err := r.client.Delete(context.TODO(), daemonset); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	} else {
		logrus.Infof("deleted dns daemonset: %s", dns.Name)
	}
	return nil
}

// desiredDNSDaemonSet returns the desired dns daemonset.
func desiredDNSDaemonSet(dns *operatorv1.DNS, coreDNSImage, kubeRBACProxyImage string) (*appsv1.DaemonSet, error) {
	daemonset := manifests.DNSDaemonSet()
	name := DNSDaemonSetName(dns)
	daemonset.Name = name.Name
	daemonset.Namespace = name.Namespace
	daemonset.SetOwnerReferences([]metav1.OwnerReference{dnsOwnerRef(dns)})

	daemonset.Labels = map[string]string{
		// associate the daemonset with the dns
		manifests.OwningDNSLabel: DNSDaemonSetLabel(dns),
	}

	// Enable eviction when cluster-autoscaler removes a node.  This ensures
	// that the cluster DNS service stops forwarding queries to the DNS pod
	// *before* the hosting node shuts down.
	daemonset.Spec.Template.Annotations = map[string]string{
		enableDaemonSetEvictionAnnotationKey: "true",
	}
	// Ensure the daemonset adopts only its own pods.
	daemonset.Spec.Selector = DNSDaemonSetPodSelector(dns)
	daemonset.Spec.Template.Labels = daemonset.Spec.Selector.MatchLabels
	daemonset.Spec.Template.Spec.NodeSelector = nodeSelectorForDNS(dns)
	daemonset.Spec.Template.Spec.Tolerations = tolerationsForDNS(dns)

	coreFileVolumeFound := false
	for i := range daemonset.Spec.Template.Spec.Volumes {
		// TODO: remove hardcoding of volume name
		switch daemonset.Spec.Template.Spec.Volumes[i].Name {
		case "config-volume":
			daemonset.Spec.Template.Spec.Volumes[i].ConfigMap.Name = DNSConfigMapName(dns).Name
			coreFileVolumeFound = true
			break
		case "metrics-tls":
			daemonset.Spec.Template.Spec.Volumes[i].Secret = &corev1.SecretVolumeSource{
				SecretName: DNSMetricsSecretName(dns),
			}
		}
	}
	if !coreFileVolumeFound {
		return nil, fmt.Errorf("volume 'config-volume' is not found")
	}

	for i, c := range daemonset.Spec.Template.Spec.Containers {
		switch c.Name {
		case "dns":
			daemonset.Spec.Template.Spec.Containers[i].Image = coreDNSImage
		case "kube-rbac-proxy":
			daemonset.Spec.Template.Spec.Containers[i].Image = kubeRBACProxyImage
		}
	}
	return daemonset, nil
}

// nodeSelectorForDNS takes a dns and returns the node selector that it
// specifies, or a default node selector if it doesn't specify one.
func nodeSelectorForDNS(dns *operatorv1.DNS) map[string]string {
	if len(dns.Spec.NodePlacement.NodeSelector) != 0 {
		return dns.Spec.NodePlacement.NodeSelector
	}
	return map[string]string{"kubernetes.io/os": "linux"}
}

// tolerationsForDNS takes a dns and returns the tolerations that it specifies,
// or default tolerations if it doesn't specify tolerations.
func tolerationsForDNS(dns *operatorv1.DNS) []corev1.Toleration {
	if len(dns.Spec.NodePlacement.Tolerations) != 0 {
		return dns.Spec.NodePlacement.Tolerations
	}
	return []corev1.Toleration{{
		Key:      "node-role.kubernetes.io/master",
		Operator: corev1.TolerationOpExists,
	}}
}

// currentDNSDaemonSet returns the current dns daemonset.
func (r *reconciler) currentDNSDaemonSet(dns *operatorv1.DNS) (bool, *appsv1.DaemonSet, error) {
	daemonset := &appsv1.DaemonSet{}
	if err := r.client.Get(context.TODO(), DNSDaemonSetName(dns), daemonset); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, daemonset, nil
}

// createDNSDaemonSet creates a dns daemonset.
func (r *reconciler) createDNSDaemonSet(daemonset *appsv1.DaemonSet) error {
	if err := r.client.Create(context.TODO(), daemonset); err != nil {
		return fmt.Errorf("failed to create dns daemonset %s/%s: %v", daemonset.Namespace, daemonset.Name, err)
	}
	logrus.Infof("created dns daemonset: %s/%s", daemonset.Namespace, daemonset.Name)
	return nil
}

// updateDNSDaemonSet updates a dns daemonset.
func (r *reconciler) updateDNSDaemonSet(current, desired *appsv1.DaemonSet) (bool, error) {
	changed, updated := daemonsetConfigChanged(current, desired)
	if !changed {
		return false, nil
	}

	if safe, reason, err := r.daemonsetUpdateIsSafe(current, updated); err != nil {
		return false, err
	} else if !safe {
		ignored := updated.DeepCopy()
		updated.Spec.Template.Spec.Tolerations = current.Spec.Template.Spec.Tolerations
		updated.Spec.Template.Spec.NodeSelector = current.Spec.Template.Spec.NodeSelector
		diff := cmp.Diff(updated, ignored, cmpopts.EquateEmpty())
		logrus.Warnf("skipping unsafe update to the node-placement parameters for dns daemonset %s/%s: %s; ignored: %v", updated.Namespace, updated.Name, reason, diff)
		changed, _ := daemonsetConfigChanged(current, updated)
		if !changed {
			return false, nil
		}
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update dns daemonset %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated dns daemonset %s/%s: %v", updated.Namespace, updated.Name, diff)
	return true, nil
}

// daemonsetUpdateIsSafe takes current and updated daemonsets, checks if the
// update is safe, and returns a Boolean value and a string value indicating
// whether the update is safe or the reason why it is not.
func (r *reconciler) daemonsetUpdateIsSafe(current, updated *appsv1.DaemonSet) (bool, string, error) {
	name := types.NamespacedName{
		Namespace: current.Namespace,
		Name:      current.Name,
	}
	// Allow the update if the current daemonset doesn't even have a
	// valid selector.
	selector, err := metav1.LabelSelectorAsSelector(current.Spec.Selector)
	if err != nil {
		logrus.Warningf("daemonset %q has an invalid spec.selector: %v", name, err)
		return true, "", nil
	}
	listOpts := []client.ListOption{
		client.MatchingLabelsSelector{Selector: selector},
		client.InNamespace(current.Namespace),
	}
	podList := &corev1.PodList{}
	if err := r.cache.List(context.TODO(), podList, listOpts...); err != nil {
		return false, "", fmt.Errorf("failed to list the daemonset's pods: %w", err)
	}
	// Allow the update if the current daemonset has 0 pods as the update
	// cannot reduce the number of pods below 0.
	if len(podList.Items) == 0 {
		logrus.Warningf("daemonset %q has 0 pods", name)
		return true, "", nil
	}
	nodeLabelSelector := &metav1.LabelSelector{
		MatchLabels: updated.Spec.Template.Spec.NodeSelector,
	}
	nodeSelector, err := metav1.LabelSelectorAsSelector(nodeLabelSelector)
	if err != nil {
		return false, fmt.Sprintf("daemonset %q has an invalid spec.template.spec.nodeSelector: %v", name, err), nil
	}
	listOpts = []client.ListOption{
		client.MatchingLabelsSelector{Selector: nodeSelector},
	}
	nodeList := &corev1.NodeList{}
	if err := r.cache.List(context.TODO(), nodeList, listOpts...); err != nil {
		return false, "", fmt.Errorf("failed to list nodes: %w", err)
	}
	for _, node := range nodeList.Items {
		if node.Spec.Unschedulable {
			continue
		}
		taints := node.Spec.Taints
		tolerations := updated.Spec.Template.Spec.Tolerations
		if !tolerationsTolerateTaints(tolerations, taints) {
			continue
		}
		// We have found a node to which it should be possible to
		// schedule pods for the updated daemonset, so the update
		// can be considered safe.
		return true, "", nil
	}
	return false, "updated daemonset's node placement parameters match 0 schedulable nodes", nil
}

func tolerationsTolerateTaints(tolerations []corev1.Toleration, taints []corev1.Taint) bool {
	if len(taints) == 0 {
		return true
	}
	for _, taint := range taints {
		for _, toleration := range tolerations {
			if toleration.ToleratesTaint(&taint) {
				return true
			}
		}
	}
	return false
}

// daemonsetConfigChanged checks if current config matches the expected config
// for the dns daemonset and if not returns the updated config.
func daemonsetConfigChanged(current, expected *appsv1.DaemonSet) (bool, *appsv1.DaemonSet) {
	changed := false
	updated := current.DeepCopy()

	if !cmp.Equal(current.Spec.UpdateStrategy, expected.Spec.UpdateStrategy, cmpopts.EquateEmpty()) {
		updated.Spec.UpdateStrategy = expected.Spec.UpdateStrategy
		changed = true
	}

	for _, name := range []string{"dns", "kube-rbac-proxy"} {
		var curIndex int
		var curImage, expImage string
		var curReady, expReady corev1.Probe

		for i, c := range current.Spec.Template.Spec.Containers {
			if name == c.Name {
				curIndex = i
				curImage = current.Spec.Template.Spec.Containers[i].Image
				if c.ReadinessProbe != nil {
					curReady = *c.ReadinessProbe
				}
				break
			}
		}
		for i, c := range expected.Spec.Template.Spec.Containers {
			if name == c.Name {
				expImage = expected.Spec.Template.Spec.Containers[i].Image
				if c.ReadinessProbe != nil {
					expReady = *c.ReadinessProbe
				}
				break
			}
		}

		if len(curImage) == 0 {
			logrus.Errorf("current daemonset %s/%s did not contain expected %s container", current.Namespace, current.Name, name)
			updated.Spec.Template.Spec.Containers = expected.Spec.Template.Spec.Containers
			changed = true
			break
		} else {
			if curImage != expImage {
				updated.Spec.Template.Spec.Containers[curIndex].Image = expImage
				changed = true
			}
			if !cmp.Equal(curReady, expReady) {
				updated.Spec.Template.Spec.Containers[curIndex].ReadinessProbe = expected.Spec.Template.Spec.Containers[curIndex].ReadinessProbe
				changed = true
			}
		}
	}
	// TODO: Also check Env?

	if updated.Spec.Template.Annotations == nil {
		updated.Spec.Template.Annotations = map[string]string{}
	}
	for k := range managedDNSDaemonSetAnnotations {
		currentVal, have := current.Spec.Template.Annotations[k]
		expectedVal, want := expected.Spec.Template.Annotations[k]
		if want && (!have || currentVal != expectedVal) {
			updated.Spec.Template.Annotations[k] = expected.Spec.Template.Annotations[k]
			changed = true
		} else if have && !want {
			delete(updated.Spec.Template.Annotations, k)
			changed = true
		}
	}
	if !cmp.Equal(current.Spec.Template.Spec.NodeSelector, expected.Spec.Template.Spec.NodeSelector, cmpopts.EquateEmpty()) {
		updated.Spec.Template.Spec.NodeSelector = expected.Spec.Template.Spec.NodeSelector
		changed = true
	}
	if !cmp.Equal(current.Spec.Template.Spec.TerminationGracePeriodSeconds, expected.Spec.Template.Spec.TerminationGracePeriodSeconds, cmpopts.EquateEmpty(), cmp.Comparer(cmpTerminationGracePeriodSeconds)) {
		updated.Spec.Template.Spec.TerminationGracePeriodSeconds = expected.Spec.Template.Spec.TerminationGracePeriodSeconds
		changed = true
	}
	if !cmp.Equal(current.Spec.Template.Spec.Tolerations, expected.Spec.Template.Spec.Tolerations, cmpopts.EquateEmpty(), cmpopts.SortSlices(cmpTolerations)) {
		updated.Spec.Template.Spec.Tolerations = expected.Spec.Template.Spec.Tolerations
		changed = true
	}
	if !cmp.Equal(current.Spec.Template.Spec.Volumes, expected.Spec.Template.Spec.Volumes, cmpopts.EquateEmpty(), cmp.Comparer(cmpConfigMapVolumeSource), cmp.Comparer(cmpSecretVolumeSource)) {
		updated.Spec.Template.Spec.Volumes = expected.Spec.Template.Spec.Volumes
		changed = true
	}

	// Detect changes to container commands
	if len(current.Spec.Template.Spec.Containers) != len(expected.Spec.Template.Spec.Containers) {
		updated.Spec.Template.Spec.Containers = expected.Spec.Template.Spec.Containers
		changed = true
	} else {
		for i, a := range current.Spec.Template.Spec.Containers {
			b := expected.Spec.Template.Spec.Containers[i]
			if !cmp.Equal(a.Command, b.Command, cmpopts.EquateEmpty()) {
				updated.Spec.Template.Spec.Containers = expected.Spec.Template.Spec.Containers
				changed = true
				break
			}
		}
	}

	if !changed {
		return false, nil
	}
	return true, updated
}

// cmpConfigMapVolumeSource compares two configmap volume source values and
// returns a Boolean indicating whether they are equal.
func cmpConfigMapVolumeSource(a, b corev1.ConfigMapVolumeSource) bool {
	if a.LocalObjectReference != b.LocalObjectReference {
		return false
	}
	if !cmp.Equal(a.Items, b.Items, cmpopts.EquateEmpty()) {
		return false
	}
	aDefaultMode := corev1.ConfigMapVolumeSourceDefaultMode
	if a.DefaultMode != nil {
		aDefaultMode = *a.DefaultMode
	}
	bDefaultMode := corev1.ConfigMapVolumeSourceDefaultMode
	if b.DefaultMode != nil {
		bDefaultMode = *b.DefaultMode
	}
	if aDefaultMode != bDefaultMode {
		return false
	}
	if !cmp.Equal(a.Optional, b.Optional, cmpopts.EquateEmpty()) {
		return false
	}
	return true
}

// cmpSecretVolumeSource compares two secret volume source values and returns a
// Boolean indicating whether they are equal.
func cmpSecretVolumeSource(a, b corev1.SecretVolumeSource) bool {
	if a.SecretName != b.SecretName {
		return false
	}
	if !cmp.Equal(a.Items, b.Items, cmpopts.EquateEmpty()) {
		return false
	}
	aDefaultMode := corev1.SecretVolumeSourceDefaultMode
	if a.DefaultMode != nil {
		aDefaultMode = *a.DefaultMode
	}
	bDefaultMode := corev1.SecretVolumeSourceDefaultMode
	if b.DefaultMode != nil {
		bDefaultMode = *b.DefaultMode
	}
	if aDefaultMode != bDefaultMode {
		return false
	}
	if !cmp.Equal(a.Optional, b.Optional, cmpopts.EquateEmpty()) {
		return false
	}
	return true
}

// cmpTolerations compares two Tolerations values and returns a Boolean
// indicating whether they are equal.
func cmpTolerations(a, b corev1.Toleration) bool {
	if a.Key != b.Key {
		return false
	}
	if a.Value != b.Value {
		return false
	}
	if a.Operator != b.Operator {
		return false
	}
	if a.Effect != b.Effect {
		return false
	}
	if a.Effect == corev1.TaintEffectNoExecute {
		if (a.TolerationSeconds == nil) != (b.TolerationSeconds == nil) {
			return false
		}
		// Field is ignored unless effect is NoExecute.
		if a.TolerationSeconds != nil && *a.TolerationSeconds != *b.TolerationSeconds {
			return false
		}
	}
	return true
}

// cmpTerminationGracePeriodSeconds compares two terminationGracePeriodSeconds
// values and returns a Boolean indicating whether they are equal.
func cmpTerminationGracePeriodSeconds(a, b *int64) bool {
	aVal := int64(corev1.DefaultTerminationGracePeriodSeconds)
	if a != nil {
		aVal = *a
	}
	bVal := int64(corev1.DefaultTerminationGracePeriodSeconds)
	if b != nil {
		bVal = *b
	}
	return aVal == bVal
}
