package operator

import (
	"context"
	"fmt"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	operatorclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"
	statuscontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller/status"

	features "github.com/openshift/api/features"
	configclient "github.com/openshift/client-go/config/clientset/versioned"
	configinformers "github.com/openshift/client-go/config/informers/externalversions"
	"github.com/openshift/library-go/pkg/operator/configobserver/featuregates"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Operator is the scaffolding for the dns operator. It sets up dependencies
// and defines the topology of the operator and its managed components, wiring
// them together.
type Operator struct {
	manager manager.Manager
	caches  []cache.Cache
	client  client.Client
}

// New creates (but does not start) a new operator from configuration.
func New(ctx context.Context, config operatorconfig.Config, kubeConfig *rest.Config) (*Operator, error) {
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %w", err)
	}
	eventRecorder := events.NewKubeRecorder(kubeClient.CoreV1().Events(config.OperatorNamespace), "cluster-dns-operator", &corev1.ObjectReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Namespace:  config.OperatorNamespace,
		Name:       "dns-operator",
	})

	configClient, err := configclient.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create openshift client: %w", err)
	}
	configInformers := configinformers.NewSharedInformerFactory(configClient, 10*time.Minute)
	desiredVersion := config.OperatorReleaseVersion
	missingVersion := "0.0.1-snapshot"

	// By default, this will exit(0) the process if the featuregates ever change to a different set of values.
	featureGateAccessor := featuregates.NewFeatureGateAccess(
		desiredVersion, missingVersion,
		configInformers.Config().V1().ClusterVersions(), configInformers.Config().V1().FeatureGates(),
		eventRecorder,
	)
	go featureGateAccessor.Run(ctx)
	go configInformers.Start(ctx.Done())

	select {
	case <-featureGateAccessor.InitialFeatureGatesObserved():
		featureGates, _ := featureGateAccessor.CurrentFeatureGates()
		logrus.Info("FeatureGates initialized", "knownFeatures", featureGates.KnownFeatures())
	case <-time.After(1 * time.Minute):
		logrus.Error(nil, "timed out waiting for FeatureGate detection")
		return nil, fmt.Errorf("timed out waiting for FeatureGate detection")
	}

	featureGates, err := featureGateAccessor.CurrentFeatureGates()
	if err != nil {
		return nil, fmt.Errorf("failed to get current feature gates: %w", err)
	}

	dnsNameResolverEnabled := featureGates.Enabled(features.FeatureGateDNSNameResolver)

	operatorManager, err := manager.New(kubeConfig, manager.Options{
		Scheme: operatorclient.GetScheme(),
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				config.OperatorNamespace:                              {},
				operatorcontroller.DefaultOperandNamespace:            {},
				operatorcontroller.GlobalUserSpecifiedConfigNamespace: {},
			},
		},
		// Use a non-caching client everywhere. The default split client does not
		// promise to invalidate the cache during writes (nor does it promise
		// sequential create/get coherence), and we have code which (probably
		// incorrectly) assumes a get immediately following a create/update will
		// return the updated resource. All client consumers will need audited to
		// ensure they are tolerant of stale data (or we need a cache or client that
		// makes stronger coherence guarantees).
		NewClient: func(config *rest.Config, options client.Options) (client.Client, error) {
			// Must override cache option, otherwise client will use cache
			options.Cache = nil
			return client.New(config, options)
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create operator manager: %v", err)
	}

	// Create and register the operator controller with the operator manager.
	cfg := operatorconfig.Config{
		OperatorNamespace:      config.OperatorNamespace,
		CoreDNSImage:           config.CoreDNSImage,
		OpenshiftCLIImage:      config.OpenshiftCLIImage,
		KubeRBACProxyImage:     config.KubeRBACProxyImage,
		OperatorReleaseVersion: config.OperatorReleaseVersion,
	}
	if _, err := operatorcontroller.New(operatorManager, operatorcontroller.Config{
		Config:                    cfg,
		DNSNameResolverEnabled:    dnsNameResolverEnabled,
		DNSNameResolverNamespaces: []string{operatorcontroller.DefaultDNSNameResolverNamespace},
	}); err != nil {
		return nil, fmt.Errorf("failed to create operator controller: %v", err)
	}

	// Set up the status controller.
	if _, err := statuscontroller.New(operatorManager, cfg); err != nil {
		return nil, fmt.Errorf("failed to create status controller: %v", err)
	}

	return &Operator{
		manager: operatorManager,

		// TODO: These are only needed for the default dns stuff, which
		// should be refactored away.
		client: operatorManager.GetClient(),
	}, nil
}

// Start creates the default DNS and then starts the operator
// synchronously until a message is received on the stop channel.
// TODO: Move the default DNS logic elsewhere.
func (o *Operator) Start(ctx context.Context) error {
	// Periodicaly ensure the default dns exists.
	go wait.Until(func() {
		if !o.manager.GetCache().WaitForCacheSync(ctx) {
			logrus.Error("failed to sync cache before ensuring default dns")
			return
		}
		err := o.ensureDefaultDNS()
		if err != nil {
			logrus.Errorf("failed to ensure default dns %v", err)
		}
	}, 1*time.Minute, ctx.Done())

	errChan := make(chan error)
	go func() {
		errChan <- o.manager.Start(ctx)
	}()

	// Wait for the manager to exit or an explicit stop.
	select {
	case <-ctx.Done():
		return nil
	case err := <-errChan:
		return err
	}
}

// ensureDefaultDNS creates the default dns if it doesn't already exist.
func (o *Operator) ensureDefaultDNS() error {
	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: operatorcontroller.DefaultDNSController,
		},
	}
	if err := o.client.Get(context.TODO(), types.NamespacedName{Name: dns.Name}, dns); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		if err := o.client.Create(context.TODO(), dns); err != nil {
			return fmt.Errorf("failed to create default dns: %v", err)
		}
		logrus.Infof("created default dns: %s", dns.Name)
	}
	return nil
}
