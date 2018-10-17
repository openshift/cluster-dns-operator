package util

import (
	corev1 "k8s.io/api/core/v1"
)

const (
	// emptyDNSConfigYAML is a yaml snippet with an empty dnsConfig.
	emptyDNSConfigYAML = `dnsConfig:`

	// invalidDNSConfigYAML is a yaml snippet with an invalid dnsConfig.
	invalidDNSConfigYAML = `dnsConfig: bad`

	// blankClusterIPYAML is a yaml snippet with a blank dnsConfig clusterIP.
	blankClusterIPYAML = `
dnsConfig:
  clusterIP:
`

	// emptyClusterIPYAML is a yaml snippet with an empty dnsConfig clusterIP (spaces).
	emptyClusterIPYAML = `
dnsConfig:
  clusterIP: "     "
`

	// goodClusterIPYAML is a yaml snippet with a valid dnsConfig clusterIP.
	goodClusterIPYAML = `
dnsConfig:
  clusterIP: "10.3.0.10"
`
)

// ConfigTest is a test case scenario used for testing the config code.
type ConfigTest struct {
	Name             string
	ConfigMap        *corev1.ConfigMap
	ErrorExpectation bool
}

// makeConfig creates a config [map] based on the yaml snippet passed to it.
func makeConfig(snippet string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		Data: map[string]string{
			"kco-config": snippet,
		},
	}
}

// ConfigTestDefaultConfigMap returns the default/best case scenario config map.
func ConfigTestDefaultConfigMap() *corev1.ConfigMap {
	return makeConfig(goodClusterIPYAML)
}

// ConfigTestScenarios returns the all the test scenarios.
func ConfigTestScenarios() []ConfigTest {
	return []ConfigTest{
		{
			Name:             "no config map",
			ErrorExpectation: true,
		},
		{
			Name:             "empty config map",
			ConfigMap:        &corev1.ConfigMap{},
			ErrorExpectation: true,
		},
		{
			Name:             "missing kco-config map",
			ConfigMap:        &corev1.ConfigMap{Data: map[string]string{"install-config": ""}},
			ErrorExpectation: true,
		},
		{
			Name:             "empty kco-config map",
			ConfigMap:        makeConfig(""),
			ErrorExpectation: true,
		},
		{
			Name:             "empty kco-config map dnsConfig",
			ConfigMap:        makeConfig(emptyDNSConfigYAML),
			ErrorExpectation: true,
		},
		{
			Name:             "invalid kco-config map dnsConfig",
			ConfigMap:        makeConfig(invalidDNSConfigYAML),
			ErrorExpectation: true,
		},
		{
			Name:             "empty kco-config map dnsConfig clusterIP",
			ConfigMap:        makeConfig(blankClusterIPYAML),
			ErrorExpectation: true,
		},
		{
			Name:             "empty kco-config map dnsConfig clusterIP with spaces",
			ConfigMap:        makeConfig(emptyClusterIPYAML),
			ErrorExpectation: true,
		},
		{
			Name:             "ok kco-config map dnsConfig clusterIP",
			ConfigMap:        ConfigTestDefaultConfigMap(),
			ErrorExpectation: false,
		},
	}
}
