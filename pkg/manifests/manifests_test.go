package manifests

import (
	"testing"
)

func TestManifests(t *testing.T) {
	DNSServiceAccount()
	DNSClusterRole()
	DNSClusterRoleBinding()
	DNSNamespace()
	DNSDaemonSet()
	DNSConfigMap()
	DNSService()

	MetricsClusterRole()
	MetricsClusterRoleBinding()
	MetricsRole()
	MetricsRoleBinding()
}
