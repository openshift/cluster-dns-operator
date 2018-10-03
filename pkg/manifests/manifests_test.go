package manifests

import (
	"testing"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"

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

func stringPtr(s string) *string {
	return &s
}
