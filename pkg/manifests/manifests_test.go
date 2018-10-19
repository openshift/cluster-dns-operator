package manifests

import (
	"testing"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/util"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManifests(t *testing.T) {
	f := NewFactory()

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
	if _, err := f.DNSDaemonSet(dns); err != nil {
		t.Errorf("invalid DNSDaemonSet: %v", err)
	}
	if _, err := f.DNSService(dns); err != nil {
		t.Errorf("invalid DNSService: %v", err)
	}
}

func TestDefaultCluserDNS(t *testing.T) {
	ic := &util.InstallConfig{
		Networking: util.NetworkingConfig{
			ServiceCIDR: "10.3.0.0/16",
		},
	}
	def, err := NewFactory().ClusterDNSDefaultCR(ic)
	if err != nil {
		t.Fatal(err)
	}
	if e, a := "10.3.0.10", *def.Spec.ClusterIP; e != a {
		t.Errorf("expected default clusterdns clusterIP=%s, got %s", e, a)
	}
}

func stringPtr(s string) *string { return &s }
