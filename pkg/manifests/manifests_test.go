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
	DNSService()

	MetricsClusterRole()
	MetricsClusterRoleBinding()
	MetricsRole()
	MetricsRoleBinding()

	NodeResolverScript()
	NodeResolverServiceAccount()
}
