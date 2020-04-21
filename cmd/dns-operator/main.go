package main

import (
	"os"

	"github.com/openshift/cluster-dns-operator/pkg/operator"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	"github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	"github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func main() {
	metrics.DefaultBindAddress = ":60000"

	// Collect operator configuration.
	releaseVersion := os.Getenv("RELEASE_VERSION")
	if len(releaseVersion) == 0 {
		releaseVersion = controller.UnknownVersionValue
		logrus.Infof("RELEASE_VERSION environment variable is missing, defaulting to %q", controller.UnknownVersionValue)
	}
	coreDNSImage := os.Getenv("IMAGE")
	if len(coreDNSImage) == 0 {
		logrus.Fatalf("IMAGE environment variable is required")
	}
	cliImage := os.Getenv("OPENSHIFT_CLI_IMAGE")
	if len(cliImage) == 0 {
		logrus.Fatalf("OPENSHIFT_CLI_IMAGE environment variable is required")
	}

	kubeRBACProxyImage := os.Getenv("KUBE_RBAC_PROXY_IMAGE")
	if len(kubeRBACProxyImage) == 0 {
		logrus.Fatalf("KUBE_RBAC_PROXY_IMAGE environment variable is required")
	}

	operatorConfig := operatorconfig.Config{
		OperatorReleaseVersion: releaseVersion,
		CoreDNSImage:           coreDNSImage,
		OpenshiftCLIImage:      cliImage,
		KubeRBACProxyImage:     kubeRBACProxyImage,
	}

	kubeConfig, err := config.GetConfig()
	if err != nil {
		logrus.Fatalf("failed to get kube config %v", err)
	}
	// Set up and start the operator.
	op, err := operator.New(operatorConfig, kubeConfig)
	if err != nil {
		logrus.Fatalf("failed to create operator: %v", err)
	}
	if err := op.Start(signals.SetupSignalHandler()); err != nil {
		logrus.Fatalf("failed to start operator: %v", err)
	}
}
