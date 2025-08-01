package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	operatorv1 "github.com/openshift/api/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// defaultService is the default service that should always be included
	// in the services list for the node resolver
	defaultService = "image-registry.openshift-image-registry.svc"

	// workloadPartitioningManagement contains the management workload annotation
	workloadPartitioningManagement = "target.workload.openshift.io/management"
)

var (
	// nodeResolverScript is a shell script that updates /etc/hosts.
	nodeResolverScript = manifests.NodeResolverScript()
)

// buildServicesList combines the default service with additional services from the DNS spec.
// The default service is always included first, followed by any additional services.
func buildServicesList(dns *operatorv1.DNS) string {
	// Start with the default service
	services := []string{defaultService}

	// Add any additional services from the spec
	if len(dns.Spec.NodeServices) > 0 {
		for _, service := range dns.Spec.NodeServices {
			// Build service name in format: name.namespace.svc
			if service.Name != "" && service.Namespace != "" {
				serviceName := service.Name + "." + service.Namespace + ".svc"
				services = append(services, serviceName)
			}
		}
	}

	// Join all services with commas for the environment variable
	return strings.Join(services, ",")
}

// ensureNodeResolverDaemonset ensures the node resolver daemonset exists if it
// should or does not exist if it should not exist.  Returns a Boolean
// indicating whether the daemonset exists, the daemonset if it does exist, and
// an error value.
func (r *reconciler) ensureNodeResolverDaemonSet(dns *operatorv1.DNS, clusterIP, clusterDomain string) (bool, *appsv1.DaemonSet, error) {
	haveDS, current, err := r.currentNodeResolverDaemonSet()
	if err != nil {
		return false, nil, err
	}
	wantDS, desired, err := desiredNodeResolverDaemonSet(dns, clusterIP, clusterDomain, r.OpenshiftCLIImage)
	if err != nil {
		return haveDS, current, fmt.Errorf("failed to build node resolver daemonset: %v", err)
	}
	switch {
	case !wantDS && !haveDS:
		return false, nil, nil
	case !wantDS && haveDS:
		if err := r.deleteNodeResolverDaemonSet(current); err != nil {
			return true, current, err
		}
		return false, nil, nil
	case wantDS && !haveDS:
		if err := r.createNodeResolverDaemonSet(desired); err != nil {
			return false, nil, err
		}
		return r.currentNodeResolverDaemonSet()
	case wantDS && haveDS:
		if updated, err := r.updateNodeResolverDaemonSet(current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentNodeResolverDaemonSet()
		}
	}
	return true, current, nil
}

// desiredNodeResolverDaemonSet returns the desired node resolver daemonset.
func desiredNodeResolverDaemonSet(dns *operatorv1.DNS, clusterIP, clusterDomain, openshiftCLIImage string) (bool, *appsv1.DaemonSet, error) {
	hostPathFile := corev1.HostPathFile
	// maxSurge must be zero when maxUnavailable is nonzero.
	maxSurge := intstr.FromInt(0)
	maxUnavailable := intstr.FromString("33%")
	envs := []corev1.EnvVar{{
		Name:  "SERVICES",
		Value: buildServicesList(dns),
	}}
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
	trueVal := true
	name := NodeResolverDaemonSetName()
	daemonset := appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				dnsOwnerRef(dns),
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: NodeResolverDaemonSetPodSelector(),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						workloadPartitioningManagement: `{"effect": "PreferredDuringScheduling"}`,
					},
					Labels: NodeResolverDaemonSetPodSelector().MatchLabels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Command: []string{
							"/bin/bash", "-c",
							nodeResolverScript,
						},
						Env:             envs,
						Image:           openshiftCLIImage,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Name:            "dns-node-resolver",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("5m"),
								corev1.ResourceMemory: resource.MustParse("21Mi"),
							},
						},
						SecurityContext: &corev1.SecurityContext{
							Privileged: &trueVal,
						},
						TerminationMessagePolicy: corev1.TerminationMessageFallbackToLogsOnError,
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "hosts-file",
							MountPath: "/etc/hosts",
						}},
					}},
					// The node-resolver pods need to run on
					// every node in the cluster.  On nodes
					// that have Smart NICs, each pod that
					// uses the container network consumes
					// an SR-IOV device.  Using the host
					// network eliminates the need for this
					// scarce resource.
					HostNetwork: true,
					NodeSelector: map[string]string{
						"kubernetes.io/os": "linux",
					},
					PriorityClassName:  "system-node-critical",
					ServiceAccountName: "node-resolver",
					Tolerations: []corev1.Toleration{{
						Operator: corev1.TolerationOpExists,
					}},
					Volumes: []corev1.Volume{{
						Name: "hosts-file",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/etc/hosts",
								Type: &hostPathFile,
							},
						},
					}},
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxSurge:       &maxSurge,
					MaxUnavailable: &maxUnavailable,
				},
			},
		},
	}
	return true, &daemonset, nil
}

// currentNodeResolverDaemonSet returns the current DNS node resolver
// daemonset.
func (r *reconciler) currentNodeResolverDaemonSet() (bool, *appsv1.DaemonSet, error) {
	daemonset := &appsv1.DaemonSet{}
	if err := r.client.Get(context.TODO(), NodeResolverDaemonSetName(), daemonset); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, daemonset, nil
}

// createNodeResolverDaemonSet creates a DNS node resolver daemonset.
func (r *reconciler) createNodeResolverDaemonSet(daemonset *appsv1.DaemonSet) error {
	if err := r.client.Create(context.TODO(), daemonset); err != nil {
		return fmt.Errorf("failed to create node resolver daemonset %s/%s: %v", daemonset.Namespace, daemonset.Name, err)
	}
	logrus.Infof("created node resolver daemonset: %s/%s", daemonset.Namespace, daemonset.Name)
	return nil
}

// deleteNodeResolverDaemonSet deletes a DNS node resolver daemonset.
func (r *reconciler) deleteNodeResolverDaemonSet(daemonset *appsv1.DaemonSet) error {
	if err := r.client.Delete(context.TODO(), daemonset); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete node resolver daemonset %s/%s: %v", daemonset.Namespace, daemonset.Name, err)
	}
	logrus.Infof("deleted node resolver daemonset: %s/%s", daemonset.Namespace, daemonset.Name)
	return nil
}

// updateNodeResolverDaemonSet updates a node resolver daemonset.
func (r *reconciler) updateNodeResolverDaemonSet(current, desired *appsv1.DaemonSet) (bool, error) {
	changed, updated := nodeResolverDaemonSetConfigChanged(current, desired)
	if !changed {
		return false, nil
	}

	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update node resolver daemonset %s/%s: %v", updated.Namespace, updated.Name, err)
	}
	logrus.Infof("updated node resolver daemonset: %s/%s", updated.Namespace, updated.Name)
	return true, nil
}

// nodeResolverDaemonSetConfigChanged checks if current config matches the expected config
// for the node resolver daemonset and if not returns the updated config.
func nodeResolverDaemonSetConfigChanged(current, expected *appsv1.DaemonSet) (bool, *appsv1.DaemonSet) {
	changed := false
	updated := current.DeepCopy()

	if !cmp.Equal(current.Spec.UpdateStrategy, expected.Spec.UpdateStrategy, cmpopts.EquateEmpty()) {
		updated.Spec.UpdateStrategy = expected.Spec.UpdateStrategy
		changed = true
	}

	if len(current.Spec.Template.Spec.Containers) != len(expected.Spec.Template.Spec.Containers) {
		updated.Spec.Template.Spec.Containers = expected.Spec.Template.Spec.Containers
		changed = true
	} else if len(expected.Spec.Template.Spec.Containers) > 0 {
		curCommand := current.Spec.Template.Spec.Containers[0].Command
		expCommand := expected.Spec.Template.Spec.Containers[0].Command
		if !cmp.Equal(curCommand, expCommand, cmpopts.EquateEmpty()) {
			updated.Spec.Template.Spec.Containers[0].Command = expCommand
			changed = true
		}

		curImage := current.Spec.Template.Spec.Containers[0].Image
		expImage := expected.Spec.Template.Spec.Containers[0].Image
		if curImage != expImage {
			updated.Spec.Template.Spec.Containers[0].Image = expImage
			changed = true
		}

		curEnv := current.Spec.Template.Spec.Containers[0].Env
		expEnv := expected.Spec.Template.Spec.Containers[0].Env
		if !cmp.Equal(curEnv, expEnv, cmpopts.EquateEmpty()) {
			updated.Spec.Template.Spec.Containers[0].Env = expEnv
			changed = true
		}
	}
	if !cmp.Equal(current.Spec.Template.Spec.NodeSelector, expected.Spec.Template.Spec.NodeSelector, cmpopts.EquateEmpty()) {
		updated.Spec.Template.Spec.NodeSelector = expected.Spec.Template.Spec.NodeSelector
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

	if !changed {
		return false, nil
	}
	return true, updated
}
