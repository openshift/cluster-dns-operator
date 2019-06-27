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
)

// ensureDNSDaemonSet ensures the dns daemonset exists for a given dns.
func (r *reconciler) ensureDNSDaemonSet(dns *operatorv1.DNS, clusterIP, clusterDomain string) (*appsv1.DaemonSet, error) {
	desired, err := desiredDNSDaemonSet(dns, clusterIP, clusterDomain, r.CoreDNSImage, r.OpenshiftCLIImage)
	if err != nil {
		return nil, fmt.Errorf("failed to build dns daemonset: %v", err)
	}
	current, err := r.currentDNSDaemonSet(dns)
	if err != nil {
		return nil, err
	}
	switch {
	case desired != nil && current == nil:
		if err := r.createDNSDaemonSet(desired); err != nil {
			return nil, err
		}
	case desired != nil && current != nil:
		if err := r.updateDNSDaemonSet(current, desired); err != nil {
			return nil, err
		}
	}
	return r.currentDNSDaemonSet(dns)
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
func desiredDNSDaemonSet(dns *operatorv1.DNS, clusterIP, clusterDomain, coreDNSImage, openshiftCLIImage string) (*appsv1.DaemonSet, error) {
	daemonset := manifests.DNSDaemonSet()
	name := DNSDaemonSetName(dns)
	daemonset.Name = name.Name
	daemonset.Namespace = name.Namespace
	daemonset.SetOwnerReferences([]metav1.OwnerReference{dnsOwnerRef(dns)})

	daemonset.Labels = map[string]string{
		// associate the daemonset with the dns
		manifests.OwningDNSLabel: DNSDaemonSetLabel(dns),
	}

	// Ensure the daemonset adopts only its own pods.
	daemonset.Spec.Selector = DNSDaemonSetPodSelector(dns)
	daemonset.Spec.Template.Labels = daemonset.Spec.Selector.MatchLabels

	coreFileVolumeFound := false
	for i := range daemonset.Spec.Template.Spec.Volumes {
		// TODO: remove hardcoding of volume name
		if daemonset.Spec.Template.Spec.Volumes[i].Name == "config-volume" {
			daemonset.Spec.Template.Spec.Volumes[i].ConfigMap.Name = DNSConfigMapName(dns).Name
			coreFileVolumeFound = true
			break
		}
	}
	if !coreFileVolumeFound {
		return nil, fmt.Errorf("volume 'config-volume' is not found")
	}

	for i, c := range daemonset.Spec.Template.Spec.Containers {
		switch c.Name {
		case "dns":
			daemonset.Spec.Template.Spec.Containers[i].Image = coreDNSImage
		case "dns-node-resolver":
			daemonset.Spec.Template.Spec.Containers[i].Image = openshiftCLIImage
			envs := []corev1.EnvVar{}
			if len(clusterIP) > 0 {
				envs = append(envs, corev1.EnvVar{
					Name:  "NAMESERVER",
					Value: clusterIP,
				})
			}
			if len(clusterDomain) > 0 {
				envs = append(envs, corev1.EnvVar{
					Name:  "CLUSTER_DOMAIN",
					Value: clusterDomain,
				})
			}

			if daemonset.Spec.Template.Spec.Containers[i].Env == nil {
				daemonset.Spec.Template.Spec.Containers[i].Env = []corev1.EnvVar{}
			}
			daemonset.Spec.Template.Spec.Containers[i].Env = append(daemonset.Spec.Template.Spec.Containers[i].Env, envs...)
		}
	}
	return daemonset, nil
}

// currentDNSDaemonSet returns the current dns daemonset.
func (r *reconciler) currentDNSDaemonSet(dns *operatorv1.DNS) (*appsv1.DaemonSet, error) {
	daemonset := &appsv1.DaemonSet{}
	if err := r.client.Get(context.TODO(), DNSDaemonSetName(dns), daemonset); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return daemonset, nil
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
func (r *reconciler) updateDNSDaemonSet(current, desired *appsv1.DaemonSet) error {
	changed, updated := daemonsetConfigChanged(current, desired)
	if !changed {
		return nil
	}

	if err := r.client.Update(context.TODO(), updated); err != nil {
		return fmt.Errorf("failed to update dns daemonset %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated dns daemonset: %s/%s", updated.Namespace, updated.Name)
	return nil
}

// daemonsetConfigChanged checks if current config matches the expected config
// for the dns daemonset and if not returns the updated config.
func daemonsetConfigChanged(current, expected *appsv1.DaemonSet) (bool, *appsv1.DaemonSet) {
	changed := false
	updated := current.DeepCopy()

	for _, name := range []string{"dns", "dns-node-resolver"} {
		var curIndex int
		var curImage, expImage string

		for i, c := range current.Spec.Template.Spec.Containers {
			if name == c.Name {
				curIndex = i
				curImage = current.Spec.Template.Spec.Containers[i].Image
				break
			}
		}
		for i, c := range expected.Spec.Template.Spec.Containers {
			if name == c.Name {
				expImage = expected.Spec.Template.Spec.Containers[i].Image
				break
			}
		}

		if len(curImage) == 0 {
			logrus.Errorf("current daemonset %s/%s did not contain expected %s container", current.Namespace, current.Name, name)
			updated.Spec.Template.Spec.Containers = expected.Spec.Template.Spec.Containers
			changed = true
			break
		} else if curImage != expImage {
			updated.Spec.Template.Spec.Containers[curIndex].Image = expImage
			changed = true
		}
	}
	// TODO: Also check Env and Volume sources?

	if !cmp.Equal(current.Spec.Template.Spec.NodeSelector, expected.Spec.Template.Spec.NodeSelector, cmpopts.EquateEmpty()) {
		updated.Spec.Template.Spec.NodeSelector = expected.Spec.Template.Spec.NodeSelector
		changed = true
	}

	if !changed {
		return false, nil
	}
	return true, updated
}
