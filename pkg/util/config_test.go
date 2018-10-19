package util

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

const (
	validInstallConfig = `
networking:
  podCIDR: "10.2.0.0/16"
  serviceCIDR: "10.3.0.0/16"
  type: flannel
`
)

func TestUnmarshalInstallConfig(t *testing.T) {
	cm := &corev1.ConfigMap{}
	cm.Data = map[string]string{"install-config": validInstallConfig}

	_, err := UnmarshalInstallConfig(cm)
	if err != nil {
		t.Fatal(err)
	}
}
