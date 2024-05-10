package main

import (
	"flag"
	"os"

	"github.com/openshift/cluster-dns-operator/pkg/operator"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	statuscontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller/status"

	"github.com/sirupsen/logrus"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const operatorNamespace = "openshift-dns-operator"

func main() {
	metricsserver.DefaultBindAddress = "127.0.0.1:60000"

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Collect operator configuration.
	releaseVersion := os.Getenv("RELEASE_VERSION")
	if len(releaseVersion) == 0 {
		releaseVersion = statuscontroller.UnknownVersionValue
		logrus.Infof("RELEASE_VERSION environment variable is missing, defaulting to %q", statuscontroller.UnknownVersionValue)
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
		OperatorNamespace:      operatorNamespace,
		OperatorReleaseVersion: releaseVersion,
		CoreDNSImage:           coreDNSImage,
		OpenshiftCLIImage:      cliImage,
		KubeRBACProxyImage:     kubeRBACProxyImage,
	}

	kubeConfig, err := config.GetConfig()
	if err != nil {
		logrus.Fatalf("failed to get kube config %v", err)
	}
	ctx := signals.SetupSignalHandler()
	// Set up and start the operator.
	op, err := operator.New(ctx, operatorConfig, kubeConfig)
	if err != nil {
		logrus.Fatalf("failed to create operator: %v", err)
	}
	if err := op.Start(ctx); err != nil {
		logrus.Fatalf("failed to start operator: %v", err)
	}
}
