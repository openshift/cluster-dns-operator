package main

import (
	"os"

	"github.com/openshift/cluster-dns-operator/pkg/operator"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func main() {
	metrics.DefaultBindAddress = ":60000"

	// Collect operator configuration.
	coreDNSImage := os.Getenv("IMAGE")
	if len(coreDNSImage) == 0 {
		logrus.Fatalf("IMAGE environment variable is required")
	}
	cliImage := os.Getenv("OPENSHIFT_CLI_IMAGE")
	if len(cliImage) == 0 {
		logrus.Fatalf("OPENSHIFT_CLI_IMAGE environment variable is required")
	}

	operatorConfig := operatorconfig.Config{
		OperatorReleaseVersion: os.Getenv("RELEASE_VERSION"),
		CoreDNSImage:           coreDNSImage,
		OpenshiftCLIImage:      cliImage,
	}

	// Set up and start the operator.
	op, err := operator.New(operatorConfig)
	if err != nil {
		logrus.Fatalf("failed to create operator: %v", err)
	}
	if err := op.Start(signals.SetupSignalHandler()); err != nil {
		logrus.Fatalf("failed to start operator: %v", err)
	}
}
