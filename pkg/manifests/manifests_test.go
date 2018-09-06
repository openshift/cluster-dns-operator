package manifests

import (
	"testing"
)

func TestManifests(t *testing.T) {
	f := NewFactory(NewDefaultConfig())

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
	if _, err := f.DNSConfigMap(); err != nil {
		t.Errorf("invalid DNSClusterRoleBinding: %v", err)
	}
	if _, err := f.DNSDaemonSet(); err != nil {
		t.Errorf("invalid DNSDaemonSet: %v", err)
	}
	if _, err := f.DNSService(); err != nil {
		t.Errorf("invalid DNSService: %v", err)
	}
}
