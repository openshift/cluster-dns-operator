package manifests

import (
	"testing"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/operator"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManifests(t *testing.T) {
	config := operator.Config{
		CoreDNSImage:      "quay.io/openshift/coredns:test",
		OpenshiftCLIImage: "openshift/origin-cli:test",
	}

	f := NewFactory(config)
	clusterDomain := "cluster.local"
	clusterIP := "172.30.77.10"
	dns := &dnsv1alpha1.ClusterDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}

	if _, err := f.ClusterDNSDefaultCR(); err != nil {
		t.Errorf("invalid ClusterDNSDefaultCR: %v", err)
	}
	if _, err := f.DNSNamespace(); err != nil {
		t.Errorf("invalid DNSNamespace: %v", err)
	}
	if _, err := f.DNSServiceAccount(); err != nil {
		t.Errorf("invalid DNSServiceAccount: %v", err)
	}
	if _, err := f.DNSClusterRole(); err != nil {
		t.Errorf("invalid DNSClusterRole: %v", err)
	}
	if _, err := f.DNSClusterRoleBinding(); err != nil {
		t.Errorf("invalid DNSClusterRoleBinding: %v", err)
	}
	if _, err := f.DNSConfigMap(dns, clusterDomain); err != nil {
		t.Errorf("invalid DNSClusterRoleBinding: %v", err)
	}
	if ds, err := f.DNSDaemonSet(dns, clusterIP, clusterDomain); err != nil {
		t.Errorf("invalid DNSDaemonSet: %v", err)
	} else {
		// Validate the daemonset
		if len(ds.Spec.Template.Spec.Containers) != 2 {
			t.Errorf("expected number of daemonset containers 2, got %d", len(ds.Spec.Template.Spec.Containers))
		}
		for _, c := range ds.Spec.Template.Spec.Containers {
			switch c.Name {
			case "dns":
				if e, a := config.CoreDNSImage, c.Image; e != a {
					t.Errorf("expected daemonset dns image %q, got %q", e, a)
				}
			case "dns-node-resolver":
				if e, a := config.OpenshiftCLIImage, c.Image; e != a {
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
	if _, err := f.DNSService(dns, clusterIP); err != nil {
		t.Errorf("invalid DNSService: %v", err)
	}
}
