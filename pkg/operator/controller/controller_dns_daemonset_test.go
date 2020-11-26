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
	clusterDomain := "cluster.local"
	clusterIP := "172.30.77.10"
	coreDNSImage := "quay.io/openshift/coredns:test"
	openshiftCLIImage := "openshift/origin-cli:test"
	kubeRBACProxyImage := "quay.io/openshift/origin-kube-rbac-proxy:test"

	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultDNSController,
		},
	}

	if ds, err := desiredDNSDaemonSet(dns, clusterIP, clusterDomain, coreDNSImage, openshiftCLIImage, kubeRBACProxyImage); err != nil {
		t.Errorf("invalid dns daemonset: %v", err)
	} else {
		// Validate the daemonset
		if len(ds.Spec.Template.Spec.Containers) != 3 {
			t.Errorf("expected number of daemonset containers 3, got %d", len(ds.Spec.Template.Spec.Containers))
		}
		for _, c := range ds.Spec.Template.Spec.Containers {
			switch c.Name {
			case "dns":
				if e, a := coreDNSImage, c.Image; e != a {
					t.Errorf("expected daemonset dns image %q, got %q", e, a)
				}
			case "dns-node-resolver":
				if e, a := openshiftCLIImage, c.Image; e != a {
					t.Errorf("expected daemonset dns node resolver image %q, got %q", e, a)
				}

				envs := map[string]string{}
				for _, e := range c.Env {
					envs[e.Name] = e.Value
				}
				nameserver, ok := envs["NAMESERVER"]
				if !ok {
					t.Errorf("NAMESERVER env for dns node resolver image not found")
				} else if clusterIP != nameserver {
					t.Errorf("expected NAMESERVER env for dns node resolver image %q, got %q", clusterIP, nameserver)
				}
				domain, ok := envs["CLUSTER_DOMAIN"]
				if !ok {
					t.Errorf("CLUSTER_DOMAIN env for dns node resolver image not found")
				} else if clusterDomain != domain {
					t.Errorf("expected CLUSTER_DOMAIN env for dns node resolver image %q, got %q", clusterDomain, domain)
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
			description: "if the dns-node-resolver container image is changed",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Containers[1].Image = "openshift/origin-cli:latest"
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
				daemonset.Spec.Template.Spec.Containers[2].Image = "openshift/origin-kube-rbac-proxy:latest"
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
				newVal := volumeDefaultMode
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
				newVal := volumeDefaultMode
				daemonset.Spec.Template.Spec.Volumes[2].Secret.DefaultMode = &newVal
			},
			expect: false,
		},
		{
			description: "if the metrics-tls default mode value changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				newVal := int32(0)
				daemonset.Spec.Template.Spec.Volumes[2].Secret.DefaultMode = &newVal
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
		hostPathFile := corev1.HostPathFile
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
								Name:  "dns-node-resolver",
								Image: "openshift/origin-cli:v4.0",
								Command: []string{
									"c",
									"d",
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
								Name: "hosts-file",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/etc/hosts",
										Type: &hostPathFile,
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
