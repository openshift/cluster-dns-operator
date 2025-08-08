package manifests

import (
	"testing"
)

func TestManifests(t *testing.T) {
	NetworkPolicyDenyAll()

	DNSServiceAccount()
	DNSClusterRole()
	DNSClusterRoleBinding()
	DNSNamespace()
	DNSDaemonSet()
	DNSService()
	DNSNetworkPolicy()

	MetricsClusterRole()
	MetricsClusterRoleBinding()
	MetricsRole()
	MetricsRoleBinding()

	NodeResolverScript()
	NodeResolverServiceAccount()
}
