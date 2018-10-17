package util

import (
	"testing"
)

func TestGetDefaultClusterDNSIP(t *testing.T) {
	for _, tc := range ConfigTestScenarios() {
		clusterIP, err := GetDefaultClusterDNSIP(tc.ConfigMap)
		t.Logf("test case: %s", tc.Name)
		if tc.ErrorExpectation {
			t.Logf("    expected error: %v", err)
			if err == nil {
				t.Errorf("test case %s expected an error, got none", tc.Name)
			}
		} else {
			t.Logf("    dns cluster IP: %s", clusterIP)
			if err != nil {
				t.Errorf("test case %s did not expect an error, got %v", tc.Name, err)
			}
		}
	}
}
