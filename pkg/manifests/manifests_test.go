package manifests

import (
	"testing"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/util"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManifests(t *testing.T) {
	f := NewFactory()

	cm := util.ConfigTestDefaultConfigMap()
	if _, err := f.ClusterDNSDefaultCR(cm); err != nil {
		t.Errorf("invalid ClusterDNSDefaultCR: %v", err)
	}

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

func TestDefaultClusterDNS(t *testing.T) {
	f := &Factory{}
	for _, tc := range util.ConfigTestScenarios() {
		_, err := f.ClusterDNSDefaultCR(tc.ConfigMap)
		if tc.ErrorExpectation {
			if err == nil {
				t.Errorf("test case %s expected an error, got none", tc.Name)
			}
		} else if err != nil {
			t.Errorf("test case %s did not expect an error, got %v", tc.Name, err)
		}
	}
}

func stringPtr(s string) *string {
	return &s
}
