package controller

import (
	"testing"
)

func TestDesiredNodeResolverDaemonset(t *testing.T) {
	clusterDomain := "cluster.local"
	clusterIP := "172.30.77.10"
	openshiftCLIImage := "openshift/origin-cli:test"

	if want, _, err := desiredNodeResolverDaemonSet(clusterIP, clusterDomain, openshiftCLIImage); err != nil {
		t.Errorf("invalid node resolver daemonset: %v", err)
	} else if want {
		t.Error("expected the node resolver daemonset desired to be false, got true")
	}
}
