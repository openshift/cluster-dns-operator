package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift/cluster-dns-operator/pkg/operator"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	"github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	"github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func NewStartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Starts the operator",
		Run: func(cmd *cobra.Command, args []string) {
			if err := start(); err != nil {
				logrus.Error(err)
				os.Exit(1)
			}
		},
	}

	return cmd
}

func start() error {
	metrics.DefaultBindAddress = ":60000"

	// Collect operator configuration.
	releaseVersion := os.Getenv("RELEASE_VERSION")
	if len(releaseVersion) == 0 {
		releaseVersion = controller.UnknownVersionValue
		logrus.Infof("RELEASE_VERSION environment variable is missing, defaulting to %q", controller.UnknownVersionValue)
	}
	coreDNSImage := os.Getenv("IMAGE")
	if len(coreDNSImage) == 0 {
		return fmt.Errorf("IMAGE environment variable is required")
	}
	operatorImage := os.Getenv("OPERATOR_IMAGE")
	if len(operatorImage) == 0 {
		return fmt.Errorf("OPERATOR_IMAGE environment variable is required")
	}

	operatorConfig := operatorconfig.Config{
		OperatorReleaseVersion: releaseVersion,
		CoreDNSImage:           coreDNSImage,
		OperatorImage:          operatorImage,
	}

	kubeConfig, err := config.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kube config %v", err)
	}
	// Set up and start the operator.
	op, err := operator.New(operatorConfig, kubeConfig)
	if err != nil {
		return fmt.Errorf("failed to create operator: %v", err)
	}
	return op.Start(signals.SetupSignalHandler())
}
