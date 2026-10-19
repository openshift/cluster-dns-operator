package controller

import (
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TestBuildServicesList tests the buildServicesList function with various DNS configurations
func TestBuildServicesList(t *testing.T) {
	testCases := []struct {
		name               string
		nodeServices       []operatorv1.DNSNodeService
		expected           string
	}{
		{
			name:               "no additional services",
			nodeServices:       nil,
			expected:           "image-registry.openshift-image-registry.svc",
		},
		{
			name:               "empty additional services slice",
			nodeServices:       []operatorv1.DNSNodeService{},
			expected:           "image-registry.openshift-image-registry.svc",
		},
		{
			name:               "single additional service",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "my-service", Namespace: "my-namespace"}},
			expected:           "image-registry.openshift-image-registry.svc,my-service.my-namespace.svc",
		},
		{
			name:               "multiple additional services",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "service1", Namespace: "namespace1"}, {Name: "service2", Namespace: "namespace2"}, {Name: "service3", Namespace: "namespace3"}},
			expected:           "image-registry.openshift-image-registry.svc,service1.namespace1.svc,service2.namespace2.svc,service3.namespace3.svc",
		},
		{
			name:               "service with empty name is filtered out",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "", Namespace: "my-namespace"}},
			expected:           "image-registry.openshift-image-registry.svc",
		},
		{
			name:               "service with empty namespace is filtered out",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "my-service", Namespace: ""}},
			expected:           "image-registry.openshift-image-registry.svc",
		},
		{
			name:               "mixed valid and invalid services",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "service1", Namespace: "namespace1"}, {Name: "", Namespace: "namespace2"}, {Name: "service3", Namespace: "namespace3"}},
			expected:           "image-registry.openshift-image-registry.svc,service1.namespace1.svc,service3.namespace3.svc",
		},
		{
			name:               "all invalid services filtered out",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "", Namespace: "namespace1"}, {Name: "service2", Namespace: ""}},
			expected:           "image-registry.openshift-image-registry.svc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dns := &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					NodeServices: tc.nodeServices,
				},
			}

			result := buildServicesList(dns)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

// TestDesiredNodeResolverDaemonset verifies that desiredNodeResolverDaemonSet
// returns the expected daemonset.
func TestDesiredNodeResolverDaemonset(t *testing.T) {
	clusterDomain := "cluster.local"
	clusterIP := "172.30.77.10"
	openshiftCLIImage := "openshift/origin-cli:test"

	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultDNSController,
		},
	}

	if want, ds, err := desiredNodeResolverDaemonSet(dns, clusterIP, clusterDomain, openshiftCLIImage); err != nil {
		t.Errorf("invalid node resolver daemonset: %v", err)
	} else if !want {
		t.Error("expected the node resolver daemonset desired to be true, got false")
	} else if len(ds.Spec.Template.Spec.Containers) != 1 {
		t.Errorf("expected number of daemonset containers 1, got %d", len(ds.Spec.Template.Spec.Containers))
	} else {
		c := ds.Spec.Template.Spec.Containers[0]
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
		services, ok := envs["SERVICES"]
		if !ok {
			t.Errorf("SERVICES env for dns node resolver image not found")
		} else if services != "image-registry.openshift-image-registry.svc" {
			t.Errorf("expected SERVICES env for dns node resolver image %q, got %q", "image-registry.openshift-image-registry.svc", services)
		}
	}
}

// TestDesiredNodeResolverDaemonsetWithNodeServices verifies that desiredNodeResolverDaemonSet
// correctly handles node services in the SERVICES environment variable.
func TestDesiredNodeResolverDaemonsetWithNodeServices(t *testing.T) {
	clusterDomain := "cluster.local"
	clusterIP := "172.30.77.10"
	openshiftCLIImage := "openshift/origin-cli:test"

	testCases := []struct {
		name               string
		nodeServices       []operatorv1.DNSNodeService
		expectedServices   string
	}{
		{
			name:               "with single node service",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "my-api", Namespace: "my-namespace"}},
			expectedServices:   "image-registry.openshift-image-registry.svc,my-api.my-namespace.svc",
		},
		{
			name:               "with multiple node services",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "service1", Namespace: "ns1"}, {Name: "service2", Namespace: "ns2"}},
			expectedServices:   "image-registry.openshift-image-registry.svc,service1.ns1.svc,service2.ns2.svc",
		},
		{
			name:               "with valid services only",
			nodeServices:       []operatorv1.DNSNodeService{{Name: "clean-service", Namespace: "namespace"}, {Name: "another-service", Namespace: "ns"}},
			expectedServices:   "image-registry.openshift-image-registry.svc,clean-service.namespace.svc,another-service.ns.svc",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dns := &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					NodeServices: tc.nodeServices,
				},
			}

			if want, ds, err := desiredNodeResolverDaemonSet(dns, clusterIP, clusterDomain, openshiftCLIImage); err != nil {
				t.Errorf("invalid node resolver daemonset: %v", err)
			} else if !want {
				t.Error("expected the node resolver daemonset desired to be true, got false")
			} else if len(ds.Spec.Template.Spec.Containers) != 1 {
				t.Errorf("expected number of daemonset containers 1, got %d", len(ds.Spec.Template.Spec.Containers))
			} else {
				c := ds.Spec.Template.Spec.Containers[0]

				// Check environment variables
				envs := map[string]string{}
				for _, e := range c.Env {
					envs[e.Name] = e.Value
				}

				// Verify SERVICES environment variable contains expected services
				services, ok := envs["SERVICES"]
				if !ok {
					t.Errorf("SERVICES env for dns node resolver image not found")
				} else if services != tc.expectedServices {
					t.Errorf("expected SERVICES env for dns node resolver image %q, got %q", tc.expectedServices, services)
				}
			}
		})
	}
}

// TestNodeResolverDaemonsetConfigChanged verifies the
// nodeResolverDaemonSetConfigChanged correctly detects changes that should
// trigger updates and ignores changes that should be ignored.
func TestNodeResolverDaemonsetConfigChanged(t *testing.T) {
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
				daemonset.Spec.Template.Spec.Containers[0].Image = "openshift/origin-cli:latest"
			},
			expect: true,
		},
		{
			description: "if a container command length changed",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Containers[0].Command = append(daemonset.Spec.Template.Spec.Containers[0].Command, "--foo")
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
			description: "if the hosts-file path value changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.Template.Spec.Volumes[0].HostPath.Path = "/foo"
			},
			expect: true,
		},
		{
			description: "if the update strategy changes",
			mutate: func(daemonset *appsv1.DaemonSet) {
				daemonset.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{
					Type: appsv1.RollingUpdateDaemonSetStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDaemonSet{
						MaxUnavailable: pointerTo(intstr.FromString("33%")),
					},
				}
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		hostPathFile := corev1.HostPathFile
		original := appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dns-original",
				Namespace: "openshift-dns",
				UID:       "1",
			},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "dns-node-resolver",
							Image: "openshift/origin-cli:v4.0",
							Command: []string{
								"c",
								"d",
							},
						}},
						NodeSelector: map[string]string{
							"beta.kubernetes.io/os": "linux",
						},
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
			},
		}
		mutated := original.DeepCopy()
		tc.mutate(mutated)
		if changed, updated := nodeResolverDaemonSetConfigChanged(&original, mutated); changed != tc.expect {
			t.Errorf("%s, expect nodeResolverDaemonsetConfigChanged to be %t, got %t", tc.description, tc.expect, changed)
		} else if changed {
			if changedAgain, _ := nodeResolverDaemonSetConfigChanged(mutated, updated); changedAgain {
				t.Errorf("%s, nodeResolverDaemonsetConfigChanged does not behave as a fixed point function", tc.description)
			}
		}
	}
}
