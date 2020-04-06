package controller

import (
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	}

	for _, tc := range testCases {
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
							},
							{
								Name:  "dns-node-resolver",
								Image: "openshift/origin-cli:v4.0",
								Command: []string{
									"c",
									"d",
								},
							},
						},
						NodeSelector: map[string]string{
							"beta.kubernetes.io/os": "linux",
						},
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
