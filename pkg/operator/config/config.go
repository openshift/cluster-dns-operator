package config

// Config is configuration for the operator and should include things like
// operated images, release version, etc.
type Config struct {
	// OperatorReleaseVersion is the current version of the operator.
	OperatorReleaseVersion string

	// CoreDNSImage is the CoreDNS image to manage.
	CoreDNSImage string

	// OpenshiftCLIImage is the openshift client image to manage.
	OpenshiftCLIImage string
}
