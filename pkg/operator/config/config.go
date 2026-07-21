package config

import "crypto/tls"

// Config is configuration for the operator and should include things like
// operated images, release version, etc.
type Config struct {
	// OperatorReleaseVersion is the current version of the operator.
	OperatorReleaseVersion string

	// OperatorNamespace is the namespace that the operator runs in.
	OperatorNamespace string

	// CoreDNSImage is the CoreDNS image to manage.
	CoreDNSImage string

	// OpenshiftCLIImage is the openshift client image to manage.
	OpenshiftCLIImage string

	// KubeRBACProxyImage is the kube-rbac-proxy image used by the
	// CoreDNS DaemonSet operand to secure its metrics endpoint.
	KubeRBACProxyImage string

	// MetricsBindAddress is the TCP address for the operator's own
	// metrics endpoint (e.g. ":9393").
	MetricsBindAddress string

	// MetricsCertDir is the directory containing tls.crt and tls.key
	// for the operator metrics server. When empty, the metrics server
	// falls back to plaintext on the loopback address.
	MetricsCertDir string

	// MetricsTLSOpts are functions that mutate the TLS configuration
	// for the operator metrics server (e.g. cipher suites and min
	// version from the cluster TLS security profile).
	MetricsTLSOpts []func(*tls.Config)
}
