package manifests

import (
	"testing"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/operator"
	"github.com/openshift/cluster-dns-operator/pkg/util"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManifests(t *testing.T) {
	config := operator.Config{
		CoreDNSImage:      "quay.io/openshift/coredns:test",
		OpenshiftCLIImage: "openshift/origin-cli:test",
	}

	f := NewFactory(config)

	dns := &dnsv1alpha1.ClusterDNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: dnsv1alpha1.ClusterDNSSpec{
			ClusterDomain: stringPtr("cluster.local"),
			ClusterIP:     stringPtr("172.30.77.10"),
		},
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
	if _, err := f.DNSConfigMap(dns); err != nil {
		t.Errorf("invalid DNSClusterRoleBinding: %v", err)
	}
	if ds, err := f.DNSDaemonSet(dns); err != nil {
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
				} else if *dns.Spec.ClusterIP != nameserver {
					t.Errorf("expected NAMESERVER env for dns node resolver image %q, got %q", *dns.Spec.ClusterIP, nameserver)
				}
				clusterDomain, ok := envs["CLUSTER_DOMAIN"]
				if !ok {
					t.Errorf("CLUSTER_DOMAIN env for dns node resolver image not found")
				} else if *dns.Spec.ClusterDomain != clusterDomain {
					t.Errorf("expected CLUSTER_DOMAIN env for dns node resolver image %q, got %q", *dns.Spec.ClusterDomain, clusterDomain)
				}
			default:
				t.Errorf("unexpected daemonset container %q", c.Name)
			}
		}
	}
	if _, err := f.DNSService(dns); err != nil {
		t.Errorf("invalid DNSService: %v", err)
	}
}

func TestDefaultClusterDNS(t *testing.T) {
	ic := &util.InstallConfig{
		Networking: util.NetworkingConfig{
			ServiceCIDR: "10.3.0.0/16",
		},
	}
	config := operator.Config{
		CoreDNSImage:      "quay.io/openshift/coredns:test",
		OpenshiftCLIImage: "openshift/origin-cli:test",
	}

	def, err := NewFactory(config).ClusterDNSDefaultCR(ic)
	if err != nil {
		t.Fatal(err)
	}
	if e, a := "10.3.0.10", *def.Spec.ClusterIP; e != a {
		t.Errorf("expected default clusterdns clusterIP=%s, got %s", e, a)
	}
}

func stringPtr(s string) *string { return &s }
