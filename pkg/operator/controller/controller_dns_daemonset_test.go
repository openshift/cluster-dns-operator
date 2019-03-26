package controller

import (
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDesiredDNSDaemonset(t *testing.T) {
	clusterDomain := "cluster.local"
	clusterIP := "172.30.77.10"
	coreDNSImage := "quay.io/openshift/coredns:test"
	openshiftCLIImage := "openshift/origin-cli:test"

	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultDNSController,
		},
	}

	if ds, err := desiredDNSDaemonSet(dns, clusterIP, clusterDomain, coreDNSImage, openshiftCLIImage); err != nil {
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
			default:
				t.Errorf("unexpected daemonset container %q", c.Name)
			}
		}
	}
}
