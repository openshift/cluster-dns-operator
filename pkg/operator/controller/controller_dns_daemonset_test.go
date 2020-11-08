package controller

import (
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDesiredDNSDaemonset(t *testing.T) {
	coreDNSImage := "quay.io/openshift/coredns:test"
	kubeRBACProxyImage := "quay.io/openshift/origin-kube-rbac-proxy:test"

	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultDNSController,
		},
	}

	if ds, err := desiredDNSDaemonSet(dns, coreDNSImage, kubeRBACProxyImage); err != nil {
		t.Errorf("invalid dns daemonset: %v", err)
	} else {
		// Validate the daemonset
		if len(ds.Spec.Template.Spec.Containers) != 2 {
			t.Errorf("expected number of daemonset containers 2, got %d", len(ds.Spec.Template.Spec.Containers))
		}
		for _, c := range ds.Spec.Template.Spec.Containers {
			switch c.Name {
			case "dns":
				if e, a := coreDNSImage, c.Image; e != a {
					t.Errorf("expected daemonset dns image %q, got %q", e, a)
				}
			case "kube-rbac-proxy":
				if e, a := kubeRBACProxyImage, c.Image; e != a {
					t.Errorf("expected daemonset kube rbac proxy image %q, got %q", e, a)
				}
			default:
				t.Errorf("unexpected daemonset container %q", c.Name)
			}
		}
	}
}

var toleration = corev1.Toleration{
	Key:      "foo",
	Value:    "bar",
	Operator: corev1.TolerationOpExists,
	Effect:   corev1.TaintEffectNoExecute,
}

func TestDaemonsetConfigChanged(t *testing.T) {
	pointerTo := func(ios intstr.IntOrString) *intstr.IntOrString { return &ios }
	testCases := []struct {
		description string
		mutate      func(*appsv1.DaemonSet)
		expect      bool
	}{
		{
			description: "if nothing changes",
			mutate:      func(_ *appsv1.DaemonSet) {},
			expect:      false,
		},
		{
			description: "if .uid changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.UID = "2"
			},
			expect: false,
		},
		{
			description: "if .spec.template.spec.nodeSelector changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				ns := map[string]string{"kubernetes.io/os": "linux"}
				daemonset.Spec.Template.Spec.NodeSelector = ns
			},
			expect: true,
		},
		{
			description: "if .spec.template.spec.tolerations changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Tolerations = []corev1.Toleration{toleration}
			},
			expect: true,
		},
		{
			description: "if .spec.template.spec.volumes changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Volumes = []corev1.Volume{
					{
						Name: "test",
					},
				}
			},
			expect: true,
		},
		{
			description: "if the dns container image is changed",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Containers[0].Image = "openshift/origin-coredns:latest"
			},
			expect: true,
		},
		{
			description: "if the kube rbac proxy image is changed",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Containers[1].Image = "openshift/origin-kube-rbac-proxy:latest"
			},
			expect: true,
		},
		{
			description: "if a container command length changed",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Containers[1].Command = append(daemonset.Spec.Template.Spec.Containers[1].Command, "--foo")
			},
			expect: true,
		},
		{
			description: "if a container command contents changed",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Containers[0].Command[1] = "c"
			},
			expect: true,
		},
		{
			description: "if an unexpected additional container is added",
			mutate: func(daemonset *appsv1.DaemonSet) {
				containers := daemonset.Spec.Template.Spec.Containers
				daemonset.Spec.Template.Spec.Containers = append(containers, corev1.Container{
					Name:  "foo",
					Image: "bar",
				})
			},
			expect: true,
		},
		{
			description: "if the config-volume default mode value is defaulted",
			mutate: func(daemonset *appsv1.DaemonSet) {
				newVal := corev1.ConfigMapVolumeSourceDefaultMode
				daemonset.Spec.Template.Spec.Volumes[0].ConfigMap.DefaultMode = &newVal
			},
			expect: false,
		},
		{
			description: "if the config-volume default mode value changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				newVal := int32(0)
				daemonset.Spec.Template.Spec.Volumes[0].ConfigMap.DefaultMode = &newVal
			},
			expect: true,
		},
		{
			description: "if the metrics-tls default mode value is defaulted",
			mutate: func(daemonset *appsv1.DaemonSet) {
				newVal := corev1.SecretVolumeSourceDefaultMode
				daemonset.Spec.Template.Spec.Volumes[1].Secret.DefaultMode = &newVal
			},
			expect: false,
		},
		{
			description: "if the metrics-tls default mode value changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				newVal := int32(0)
				daemonset.Spec.Template.Spec.Volumes[1].Secret.DefaultMode = &newVal
			},
			expect: true,
		},
		{
			description: "if the readiness probe endpoint changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Containers[0].ReadinessProbe.Handler.HTTPGet.Path = "/ready"
			},
			expect: true,
		},
		{
			description: "if the readiness probe period changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Containers[0].ReadinessProbe.PeriodSeconds = 2
			},
			expect: true,
		},
		{
			description: "if the termination grace period is defaulted",
			mutate: func(daemonset *appsv1.DaemonSet) {
				newVal := int64(corev1.DefaultTerminationGracePeriodSeconds)
				daemonset.Spec.Template.Spec.TerminationGracePeriodSeconds = &newVal
			},
			expect: false,
		},
		{
			description: "if the termination grace period changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				sixty := int64(60)
				daemonset.Spec.Template.Spec.TerminationGracePeriodSeconds = &sixty
			},
			expect: true,
		},
		{
			description: "if the update strategy changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
					Type: appsv1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: pointerTo(intstr.FromString("10%")),
					},
				}
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		thirty := int64(30)
		original := appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dns-original",
				Namespace: "openshift-dns",
				UID:       "1",
			},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "dns",
								Image: "openshift/origin-coredns:v4.0",
								Command: []string{
									"a",
									"b",
								},
								ReadinessProbe: &corev1.Probe{
									PeriodSeconds: 10,
									Handler: corev1.Handler{
										HTTPGet: &corev1.HTTPGetAction{
											Path: "/health",
											Port: intstr.IntOrString{
												IntVal: int32(8080),
											},
											Scheme: "HTTP",
										},
									},
								},
							},
							{
								Name:  "kube-rbac-proxy",
								Image: "openshift/origin-kube-rbac-proxy:v4.0",
								Command: []string{
									"e",
									"f",
								},
							},
						},
						NodeSelector: map[string]string{
							"beta.kubernetes.io/os": "linux",
						},
						TerminationGracePeriodSeconds: &thirty,
						Volumes: []corev1.Volume{
							{
								Name: "config-volume",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "dns-default",
										},
									},
								},
							},
							{
								Name: "metrics-tls",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: "dns-default-metrics-tls",
									},
								},
							},
						},
					},
				},
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
					Type: appsv1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: pointerTo(intstr.FromInt(1)),
					},
				},
			},
		}
		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := daemonsetConfigChanged(&original, mutated); changed != tc.expect {
			t.Errorf("%s, expect daemonsetConfigChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if changedAgain, _ := daemonsetConfigChanged(mutated, updated); changedAgain {
				t.Errorf("%s, daemonsetConfigChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
