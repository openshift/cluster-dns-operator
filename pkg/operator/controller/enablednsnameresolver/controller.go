package enablednsnameresolver

import (
	"context"
	"sync"

	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "enable_dnsnameresolver_controller"
)

// New creates and returns a controller that starts the dependent caches
// and controllers when the appropriate featuregate is enabled.
func New(mgr manager.Manager, config Config) (controller.Controller, error) {
	reconciler := &reconciler{
		cache:  mgr.GetCache(),
		client: mgr.GetClient(),
		config: config,
	}
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	clusterNamePredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		expectedName := operatorcontroller.FeatureGateClusterConfigName()
		actualName := types.NamespacedName{
			Namespace: o.GetNamespace(),
			Name:      o.GetName(),
		}
		return expectedName == actualName
	})
	if err := c.Watch(source.Kind(mgr.GetCache(), &configv1.FeatureGate{}), &handler.EnqueueRequestForObject{}, clusterNamePredicate); err != nil {
		return nil, err
	}
	return c, nil
}

// Config holds all the configuration that must be provided when creating the
// controller.
type Config struct {
	// DNSNameResolverEnabled indicates that the "DNSNameResolver" featuregate is enabled.
	DNSNameResolverEnabled bool
	// DependentCaches is a list of caches that are used by Controllers watching DNSNameResolver
	// resources. The enable_dnsnameresolver controller starts these caches when the
	// "DNSNameResolver" featuregate is enabled.
	DependentCaches []cache.Cache
	// DependentControllers is a list of controllers that watch DNSNameResolver
	// resources. The enable_dnsnameresolver controller starts these controllers when the
	// "DNSNameResolver" featuregate is enabled and the DependentCaches are started.
	DependentControllers []controller.Controller
}

// reconciler handles the actual featuregate reconciliation logic in response to
// events.
type reconciler struct {
	config Config

	cache            cache.Cache
	client           client.Client
	startCaches      sync.Once
	startControllers sync.Once
}

// Reconcile expects request to refer to a FeatureGate and starts the dependent
// caches and controllers.
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logrus.Info("reconciling", "request", request)

	// Check if the DNSNameResolved feature gate is enabled or not.
	if !r.config.DNSNameResolverEnabled {
		return reconcile.Result{}, nil
	}

	// Start the dependent caches and wait for the caches to sync.
	r.startCaches.Do(func() {
		var wg sync.WaitGroup
		for i := range r.config.DependentCaches {
			cache := &r.config.DependentCaches[i]

			// Start the dependent cache.
			go func() {
				if err := (*cache).Start(ctx); err != nil {
					logrus.Error(err, "cannot start cache")
				}
			}()

			// Wait for the dependent cache to sync.
			wg.Add(1)
			go func() {
				if started := (*cache).WaitForCacheSync(ctx); !started {
					logrus.Error("failed to sync cache before starting controllers")
				}
				wg.Done()
			}()
		}
		// Wait for all the dependent caches to sync.
		wg.Wait()
	})

	// Start the dependent controllers after the dependent caches have synced.
	r.startControllers.Do(func() {
		for i := range r.config.DependentControllers {
			controller := &r.config.DependentControllers[i]

			// Start the dependent controller.
			go func() {
				if err := (*controller).Start(ctx); err != nil {
					logrus.Error(err, "cannot start controller")
				}
			}()
		}
	})

	return reconcile.Result{}, nil
}
