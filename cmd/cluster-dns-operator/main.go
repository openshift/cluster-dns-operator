package main

import (
	"context"
	"runtime"
	"time"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	stub "github.com/openshift/cluster-dns-operator/pkg/stub"
	"github.com/openshift/cluster-dns-operator/pkg/util"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"

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
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logrus.Fatalf("failed to get watch namespace: %v", err)
	}
	resyncPeriod := 10 * time.Minute
	logrus.Infof("Watching %s, %s, %s, %d", resource, kind, namespace, resyncPeriod)
	sdk.Watch(resource, kind, namespace, resyncPeriod)

	kubeClient := k8sclient.GetKubeClient()

	ic, err := util.GetInstallConfig(kubeClient)
	if err != nil {
		logrus.Fatalf("could't get installconfig: %v", err)
	}

	handler := &stub.Handler{
		InstallConfig:   ic,
		Namespace:       namespace,
		ManifestFactory: manifests.NewFactory(),
	}

	if err := handler.EnsureDefaultClusterDNS(); err != nil {
		logrus.Fatalf("failed to ensure default clusterdns: %v", err)
	}
	sdk.Handle(handler)
	sdk.Run(context.TODO())
}
