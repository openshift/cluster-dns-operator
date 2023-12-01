package dnsnameresolverfeature

import (
	"context"
	"sync"

	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
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
	controllerName = "dnsnameresolverfeature_controller"
)

// New creates and returns a controller that creates DNSNameResolver CRD when the
// appropriate featuregate is enabled.
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

	FeatureGateName string

	// DependentCaches is a list of caches that are used by Controllers watching DNSNameResolver
	// resources. The dnsnameresolverfeature controller starts these caches once
	// the DNSNameResolver CRD has been created.
	DependentCaches []cache.Cache
	// DependentControllers is a list of controllers that watch DNSNameResolver
	// resources. The dnsnameresolverfeature controller starts these controllers once
	// the DNSNameResolver CRD has been created and the DependentCaches are started.
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

// Reconcile expects request to refer to a FeatureGate and creates or
// reconciles the DNSNameResolver CRD.
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logrus.Info("reconciling", "request", request)

	// Check if the DNSNameResolved feature gate is enabled or not.
	if !r.config.DNSNameResolverEnabled {
		return reconcile.Result{}, nil
	}

	// Check if the name of feature gate matches with the default feature gate.
	if request.Name != r.config.FeatureGateName {
		// Return a nil error value to avoid re-triggering the event.
		logrus.Errorf("skipping unexpected feature gate object %s", request.Name)
		return reconcile.Result{}, nil
	}

	// Get the feature gate object.
	featureGate := configv1.FeatureGate{}
	if err := r.cache.Get(ctx, request.NamespacedName, &featureGate); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Check that the feature set is either TechPreviewNoUpgrade or CustomNoUpgrade.
	if featureGate.Spec.FeatureSet != configv1.TechPreviewNoUpgrade && featureGate.Spec.FeatureSet != configv1.CustomNoUpgrade {
		return reconcile.Result{}, nil
	}

	// Ensure the DNSNameResolver CRD is created for the enabled feature set.
	if err := r.ensureDNSNameResolverCRD(ctx, featureGate.Spec.FeatureSet); err != nil {
		return reconcile.Result{}, err
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
