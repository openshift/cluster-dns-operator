package config

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

	// KubeRBACProxyImage is the kube-rbac-proxy image to to use
	// to secure the metrics endpoint.
	KubeRBACProxyImage string
}
