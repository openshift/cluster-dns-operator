package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"

	configv1 "github.com/openshift/api/config/v1"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	"github.com/openshift/cluster-dns-operator/pkg/operator"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	statuscontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller/status"

	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

const operatorNamespace = "openshift-dns-operator"

func main() {
	metricsBindAddr := flag.String("metrics-bind-addr", "127.0.0.1:60000", "The TCP address for the metrics endpoint.")
	metricsCertDir := flag.String("metrics-cert-dir", "", "The directory containing tls.crt and tls.key for the metrics endpoint.")

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

	kubeConfig, err := config.GetConfig()
	if err != nil {
		logrus.Fatalf("failed to get kube config: %v", err)
	}

	// Build TLS options for the operator metrics server from the
	// cluster-wide TLS security profile when secure serving is enabled.
	var metricsTLSOpts []func(*tls.Config)
	if *metricsCertDir != "" {
		cfgClient, err := configclient.NewForConfig(kubeConfig)
		if err != nil {
			logrus.Fatalf("failed to create config client: %v", err)
		}
		apiServer, err := cfgClient.ConfigV1().APIServers().Get(context.TODO(), "cluster", metav1.GetOptions{})
		var tlsSecurityProfile *configv1.TLSSecurityProfile
		if err != nil {
			logrus.Warningf("failed to get apiserver config, using Intermediate TLS profile: %v", err)
		} else {
			tlsSecurityProfile = apiServer.Spec.TLSSecurityProfile
		}
		profileSpec := operatorcontroller.TLSProfileSpecForSecurityProfile(tlsSecurityProfile)
		tlsCfg, err := operatorcontroller.TLSConfigFromProfile(profileSpec)
		if err != nil {
			logrus.Fatalf("failed to build TLS config from profile: %v", err)
		}
		metricsTLSOpts = append(metricsTLSOpts, func(cfg *tls.Config) {
			cfg.CipherSuites = tlsCfg.CipherSuites
			cfg.MinVersion = tlsCfg.MinVersion
			if len(tlsCfg.CurvePreferences) > 0 {
				cfg.CurvePreferences = tlsCfg.CurvePreferences
			}
		})
	}

	operatorConfig := operatorconfig.Config{
		OperatorNamespace:      operatorNamespace,
		OperatorReleaseVersion: releaseVersion,
		CoreDNSImage:           coreDNSImage,
		OpenshiftCLIImage:      cliImage,
		KubeRBACProxyImage:     kubeRBACProxyImage,
		MetricsBindAddress:     *metricsBindAddr,
		MetricsCertDir:         *metricsCertDir,
		MetricsTLSOpts:         metricsTLSOpts,
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
