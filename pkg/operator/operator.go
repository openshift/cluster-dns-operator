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

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

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
func New(config operatorconfig.Config, kubeConfig *rest.Config) (*Operator, error) {
	operatorManager, err := manager.New(kubeConfig, manager.Options{
		Scheme:    operatorclient.GetScheme(),
		Namespace: "openshift-dns",
		NewCache: cache.MultiNamespacedCacheBuilder([]string{
			config.OperatorNamespace,
			operatorcontroller.DefaultOperandNamespace,
			operatorcontroller.GlobalUserSpecifiedConfigNamespace,
		}),
		// Use a non-caching client everywhere. The default split client does not
		// promise to invalidate the cache during writes (nor does it promise
		// sequential create/get coherence), and we have code which (probably
		// incorrectly) assumes a get immediately following a create/update will
		// return the updated resource. All client consumers will need audited to
		// ensure they are tolerant of stale data (or we need a cache or client that
		// makes stronger coherence guarantees).
		NewClient: func(_ cache.Cache, config *rest.Config, options client.Options, uncachedObjects ...client.Object) (client.Client, error) {
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
	if _, err := operatorcontroller.New(operatorManager, cfg); err != nil {
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
