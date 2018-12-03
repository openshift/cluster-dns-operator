package operator

// Config is configuration for the operator and should include things like
// operated images, scheduling configuration, etc.
type Config struct {
	// CoreDNSImage is the CoreDNS image to manage.
	CoreDNSImage string
	// OpenshiftCLIImage is the openshift client image to manage.
	OpenshiftCLIImage string
}
