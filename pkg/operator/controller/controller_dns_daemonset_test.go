package controller

import (
	"reflect"
	"testing"

	v1 "github.com/openshift/api/config/v1"
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

	if ds, err := desiredDNSDaemonSet(dns, coreDNSImage, kubeRBACProxyImage, map[string]string{}); err != nil {
		t.Errorf("invalid dns daemonset: %v", err)
	} else {
		// Validate the daemonset
		expectedPodAnnotations := map[string]string{
			"cluster-autoscaler.kubernetes.io/enable-ds-eviction": "true",
			"target.workload.openshift.io/management":             "{\"effect\": \"PreferredDuringScheduling\"}",
		}
		actualPodAnnotations := ds.Spec.Template.Annotations
		expectedPodLabels := map[string]string{
			"dns.operator.openshift.io/daemonset-dns": "default",
		}
		actualPodLabels := ds.Spec.Template.Labels
		expectedNodeSelector := map[string]string{"kubernetes.io/os": "linux"}
		actualNodeSelector := ds.Spec.Template.Spec.NodeSelector
		if !reflect.DeepEqual(actualNodeSelector, expectedNodeSelector) {
			t.Errorf("unexpected node selector: expected %#v, got %#v", expectedNodeSelector, actualNodeSelector)
		}
		actualTolerations := ds.Spec.Template.Spec.Tolerations
		expectedTolerations := []corev1.Toleration{{
			Key:      "node-role.kubernetes.io/master",
			Operator: corev1.TolerationOpExists,
		}}
		if !reflect.DeepEqual(actualPodAnnotations, expectedPodAnnotations) {
			t.Errorf("unexpected pod annotations: expected %#v, got %#v", expectedPodAnnotations, actualPodAnnotations)
		}
		if !reflect.DeepEqual(actualPodLabels, expectedPodLabels) {
			t.Errorf("unexpected pod labels: expected %#v, got %#v", expectedPodLabels, actualPodLabels)
		}
		if !reflect.DeepEqual(actualTolerations, expectedTolerations) {
			t.Errorf("unexpected tolerations: expected %#v, got %#v", expectedTolerations, actualTolerations)
		}
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

func TestDesiredDNSDaemonsetWithCABundleConfigMaps(t *testing.T) {
	coreDNSImage := "quay.io/openshift/coredns:test"
	kubeRBACProxyImage := "quay.io/openshift/origin-kube-rbac-proxy:test"

	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultDNSController,
		},
		Spec: operatorv1.DNSSpec{
			Servers: []operatorv1.Server{
				{
					ForwardPlugin: operatorv1.ForwardPlugin{
						TransportConfig: operatorv1.DNSTransportConfig{
							Transport: operatorv1.TLSTransport,
							TLS: &operatorv1.DNSOverTLSConfig{
								ServerName: "dns.foo.com",
								CABundle: v1.ConfigMapNameReference{
									Name: "caBundle1",
								},
							},
						},
						Upstreams: []string{"1.1.1.1"},
					},
					Name:  "foo.com",
					Zones: []string{"foo.com"},
				},
				{
					ForwardPlugin: operatorv1.ForwardPlugin{
						TransportConfig: operatorv1.DNSTransportConfig{
							Transport: operatorv1.TLSTransport,
							TLS: &operatorv1.DNSOverTLSConfig{
								ServerName: "dns.bar.com",
								CABundle: v1.ConfigMapNameReference{
									Name: "caBundle2",
								},
							},
						},
						Upstreams: []string{"2.2.2.2"},
					},
					Name:  "bar.com",
					Zones: []string{"bar.com"},
				},
				{
					ForwardPlugin: operatorv1.ForwardPlugin{
						TransportConfig: operatorv1.DNSTransportConfig{
							Transport: operatorv1.TLSTransport,
							TLS: &operatorv1.DNSOverTLSConfig{
								ServerName: "dns.foo.com",
								CABundle: v1.ConfigMapNameReference{
									Name: "non_existing_caBundle",
								},
							},
						},
						Upstreams: []string{"3.3.3.3"},
					},
					Name:  "foobar.com",
					Zones: []string{"foobar.com"},
				},
			},
			UpstreamResolvers: operatorv1.UpstreamResolvers{
				TransportConfig: operatorv1.DNSTransportConfig{
					Transport: operatorv1.TLSTransport,
					TLS: &operatorv1.DNSOverTLSConfig{
						ServerName: "example.com",
						CABundle: v1.ConfigMapNameReference{
							Name: "caBundle3",
						},
					},
				},
			},
		},
	}

	cmMap := make(map[string]string)
	cmMap["caBundle1"] = "ca-caBundle1-10"
	cmMap["caBundle2"] = "ca-caBundle2-20"
	cmMap["caBundle3"] = "ca-caBundle3-30"

	if ds, err := desiredDNSDaemonSet(dns, coreDNSImage, kubeRBACProxyImage, cmMap); err != nil {
		t.Errorf("invalid dns daemonset: %v", err)
	} else {
		// Validate the volumes
		expectedVolumes := map[string]corev1.Volume{
			"config-volume": {
				Name: "config-volume",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "dns-default",
						},
						Items: []corev1.KeyToPath{
							{
								Key:  "Corefile",
								Path: "Corefile",
							},
						},
					},
				},
			},
			"metrics-tls": {
				Name: "metrics-tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "dns-default-metrics-tls",
					},
				},
			},
			"ca-caBundle1": {
				Name: "ca-caBundle1",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "ca-caBundle1",
						},
						Items: []corev1.KeyToPath{
							{
								Key:  caBundleFileName,
								Path: caBundleFileName,
							},
						},
					},
				},
			},
			"ca-caBundle2": {
				Name: "ca-caBundle2",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "ca-caBundle2",
						},
						Items: []corev1.KeyToPath{
							{
								Key:  caBundleFileName,
								Path: caBundleFileName,
							},
						},
					},
				},
			},
			"ca-caBundle3": {
				Name: "ca-caBundle3",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "ca-caBundle3",
						},
						Items: []corev1.KeyToPath{
							{
								Key:  caBundleFileName,
								Path: caBundleFileName,
							},
						},
					},
				},
			},
		}
		// Validate the volume mounts
		expectedVolumeMounts := map[string]corev1.VolumeMount{
			"config-volume": {
				Name:      "config-volume",
				MountPath: "/etc/coredns",
				ReadOnly:  true,
			},
			"ca-caBundle1": {
				Name:      "ca-caBundle1",
				MountPath: "/etc/pki/dns.foo.com-ca-caBundle1-10",
				ReadOnly:  true,
			},
			"ca-caBundle2": {
				Name:      "ca-caBundle2",
				MountPath: "/etc/pki/dns.bar.com-ca-caBundle2-20",
				ReadOnly:  true,
			},
			"ca-caBundle3": {
				Name:      "ca-caBundle3",
				MountPath: "/etc/pki/example.com-ca-caBundle3-30",
				ReadOnly:  true,
			},
		}
		actualVolumes := ds.Spec.Template.Spec.Volumes
		if len(actualVolumes) != 5 {
			t.Errorf("unexpected number of volumes: expected 5, got %d", len(actualVolumes))
		}
		for _, actualVolume := range actualVolumes {
			expectedVolume := expectedVolumes[actualVolume.Name]
			if !reflect.DeepEqual(actualVolume, expectedVolume) {
				t.Errorf("unexpected volume: expected %#v, got %#v", expectedVolume, actualVolume)
			}
		}

		actualVolumeMounts := ds.Spec.Template.Spec.Containers[0].VolumeMounts
		if len(actualVolumeMounts) != 4 {
			t.Errorf("unexpected number of volume mounts: expected 4, got %d", len(actualVolumeMounts))
		}
		for _, actualVolumeMount := range actualVolumeMounts {
			expectedVolumeMount := expectedVolumeMounts[actualVolumeMount.Name]
			if !reflect.DeepEqual(actualVolumeMount, expectedVolumeMount) {
				t.Errorf("unexpected volume: expected %#v, got %#v", expectedVolumeMount, actualVolumeMount)
			}
		}
	}
}

// TestDesiredDNSDaemonsetNodePlacement verifies that desiredDNSDaemonSet
// respects the DNS pod placement API.
func TestDesiredDNSDaemonsetNodePlacement(t *testing.T) {
	nodeSelector := map[string]string{
		"xyzzy": "quux",
	}
	tolerations := []corev1.Toleration{{
		Key:      "foo",
		Value:    "bar",
		Operator: corev1.TolerationOpExists,
		Effect:   corev1.TaintEffectNoExecute,
	}}
	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultDNSController,
		},
		Spec: operatorv1.DNSSpec{
			NodePlacement: operatorv1.DNSNodePlacement{
				NodeSelector: nodeSelector,
				Tolerations:  tolerations,
			},
		},
	}
	if ds, err := desiredDNSDaemonSet(dns, "", "", map[string]string{}); err != nil {
		t.Errorf("invalid dns daemonset: %v", err)
	} else {
		actualNodeSelector := ds.Spec.Template.Spec.NodeSelector
		if !reflect.DeepEqual(actualNodeSelector, nodeSelector) {
			t.Errorf("unexpected node selector: expected %#v, got %#v", nodeSelector, actualNodeSelector)
		}
		actualTolerations := ds.Spec.Template.Spec.Tolerations
		if !reflect.DeepEqual(actualTolerations, tolerations) {
			t.Errorf("unexpected tolerations: expected %#v, got %#v", tolerations, actualTolerations)
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
				daemonset.Spec.Template.Spec.Containers[0].ReadinessProbe.ProbeHandler.HTTPGet.Path = "/ready"
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
		{
			description: "if an arbitrary annotation is added",
			mutate: func(daemonset *appsv1.DaemonSet) {
				if daemonset.Spec.Template.Annotations == nil {
					daemonset.Spec.Template.Annotations = map[string]string{}
				}
				daemonset.Spec.Template.Annotations["foo"] = "bar"
			},
			expect: false,
		},
		{
			description: "if the enable-ds-eviction annotation is changed",
			mutate: func(daemonset *appsv1.DaemonSet) {
				if daemonset.Spec.Template.Annotations == nil {
					daemonset.Spec.Template.Annotations = map[string]string{}
				}
				daemonset.Spec.Template.Annotations["cluster-autoscaler.kubernetes.io/enable-ds-eviction"] = ""
			},
			expect: true,
		},
		{
			description: "if the target workload annotation is changed",
			mutate: func(daemonset *appsv1.DaemonSet) {
				if daemonset.Spec.Template.Annotations == nil {
					daemonset.Spec.Template.Annotations = map[string]string{}
				}
				daemonset.Spec.Template.Annotations["target.workload.openshift.io/management"] = ""
			},
			expect: true,
		},
		{
			description: "if the enable-ds-eviction annotation is deleted",
			mutate: func(daemonset *appsv1.DaemonSet) {
				delete(daemonset.Spec.Template.Annotations, "cluster-autoscaler.kubernetes.io/enable-ds-eviction")
			},
			expect: true,
		},
		{
			description: "if the target workload annotation is deleted",
			mutate: func(daemonset *appsv1.DaemonSet) {
				delete(daemonset.Spec.Template.Annotations, "target.workload.openshift.io/management")
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
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"cluster-autoscaler.kubernetes.io/enable-ds-eviction": "true",
							"target.workload.openshift.io/management":             "{\"effect\": \"PreferredDuringScheduling\"}",
						},
					},
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
									ProbeHandler: corev1.ProbeHandler{
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
