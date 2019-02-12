package main

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	"github.com/openshift/cluster-dns-operator/pkg/operator"
	stub "github.com/openshift/cluster-dns-operator/pkg/stub"

	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	sdkVersion "github.com/operator-framework/operator-sdk/version"

	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/sirupsen/logrus"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	printVersion()

	sdk.ExposeMetricsPort()

	resource := "dns.openshift.io/v1alpha1"
	kind := "ClusterDNS"
	resyncPeriod := 10 * time.Minute
	logrus.Infof("Watching %s, %s, %d", resource, kind, resyncPeriod)
	sdk.Watch(resource, kind, corev1.NamespaceAll, resyncPeriod)
	// TODO Use a named constant for the application's namespace or get the
	// namespace from config.
	sdk.Watch("apps/v1", "DaemonSet", "openshift-dns", resyncPeriod)

	coreDNSImage := os.Getenv("IMAGE")
	if len(coreDNSImage) == 0 {
		logrus.Fatalf("IMAGE environment variable is required")
	}
	cliImage := os.Getenv("OPENSHIFT_CLI_IMAGE")
	if len(cliImage) == 0 {
		logrus.Fatalf("OPENSHIFT_CLI_IMAGE environment variable is required")
	}
	operatorImageVersion := os.Getenv("OPERATOR_IMAGE_VERSION")
	if len(operatorImageVersion) == 0 {
		logrus.Fatalf("OPERATOR_IMAGE_VERSION environment variable is required")
	}

	operatorConfig := operator.Config{
		CoreDNSImage:         coreDNSImage,
		OpenshiftCLIImage:    cliImage,
		OperatorImageVersion: operatorImageVersion,
	}

	handler := &stub.Handler{
		ManifestFactory: manifests.NewFactory(operatorConfig),
		Config:          operatorConfig,
	}

	if err := handler.EnsureDefaultClusterDNS(); err != nil {
		logrus.Fatalf("failed to ensure default clusterdns: %v", err)
	}
	sdk.Handle(handler)
	sdk.Run(context.TODO())
}
