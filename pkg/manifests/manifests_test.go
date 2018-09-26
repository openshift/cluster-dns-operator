package manifests

import (
	"testing"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
)

func TestManifests(t *testing.T) {
	f := NewFactory()

	dns := &dnsv1alpha1.ClusterDNS{
		Spec: dnsv1alpha1.ClusterDNSSpec{
			ClusterDomain: stringPtr("cluster.local"),
			ClusterIP:     stringPtr("172.30.77.10"),
		},
	}

	if _, err := f.OperatorNamespace(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.OperatorCustomResourceDefinition(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.OperatorClusterRole(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.OperatorClusterRoleBinding(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.OperatorDeployment(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.OperatorServiceAccount(); err != nil {
		t.Fatal(err)
	}
	if assetData := f.OperatorAssetContent(); len(assetData) == 0 {
		t.Fatal("expected some valid operator asset content")
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
	if _, err := f.DNSDaemonSet(); err != nil {
		t.Errorf("invalid DNSDaemonSet: %v", err)
	}
	if _, err := f.DNSService(dns); err != nil {
		t.Errorf("invalid DNSService: %v", err)
	}
}

func stringPtr(s string) *string {
	return &s
}
