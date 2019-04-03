package operator

import (
	"context"
	"fmt"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	operatorclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	kconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
func New(config operatorconfig.Config) (*Operator, error) {
	kubeConfig, err := kconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kube config: %v", err)
	}
	scheme := operatorclient.GetScheme()
	operatorManager, err := manager.New(kubeConfig, manager.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create operator manager: %v", err)
	}

	// Create and register the operator controller with the operator manager.
	operatorController, err := operatorcontroller.New(operatorManager, operatorcontroller.Config{
		KubeConfig:             kubeConfig,
		CoreDNSImage:           config.CoreDNSImage,
		OpenshiftCLIImage:      config.OpenshiftCLIImage,
		OperatorReleaseVersion: config.OperatorReleaseVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create operator controller: %v", err)
	}

	// Create additional controller event sources from informers in the managed
	// namespace. Any new managed resources outside the operator's namespace
	// should be added here.
	mapper, err := apiutil.NewDiscoveryRESTMapper(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get API Group-Resources")
	}
	operandCache, err := cache.New(kubeConfig, cache.Options{Namespace: "openshift-dns", Scheme: scheme, Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("failed to create openshift-dns cache: %v", err)
	}
	// Any types added to the list here will only queue a cluster dns if the
	// resource has the expected label associating the resource with a cluster dns.
	for _, o := range []runtime.Object{
		&appsv1.DaemonSet{},
		&corev1.Service{},
		&corev1.ConfigMap{},
	} {
		// TODO: may not be necessary to copy, but erring on the side of caution for
		// now given we're in a loop.
		obj := o.DeepCopyObject()
		informer, err := operandCache.GetInformer(obj)
		if err != nil {
			return nil, fmt.Errorf("failed to get informer for %v: %v", obj, err)
		}
		operatorController.Watch(&source.Informer{Informer: informer}, &handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				labels := a.Meta.GetLabels()
				if dnsName, ok := labels[manifests.OwningClusterDNSLabel]; ok {
					logrus.Infof("queueing cluster dns %s, related: %s", dnsName, a.Meta.GetSelfLink())
					return []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Name: dnsName,
							},
						},
					}
				} else {
					return []reconcile.Request{}
				}
			}),
		})
	}

	kubeClient, err := operatorclient.NewClient(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %v", err)
	}
	return &Operator{
		manager: operatorManager,
		caches:  []cache.Cache{operandCache},

		// TODO: These are only needed for the default cluster dns stuff, which
		// should be refactored away.
		client: kubeClient,
	}, nil
}

// Start creates the default cluster DNS and then starts the operator
// synchronously until a message is received on the stop channel.
// TODO: Move the default cluster DNS logic elsewhere.
func (o *Operator) Start(stop <-chan struct{}) error {
	// Periodicaly ensure the default controller exists.
	go wait.Until(func() {
		if err := o.ensureDefaultDNS(); err != nil {
			logrus.Errorf("failed to ensure default cluster dns: %v", err)
		}
	}, 1*time.Minute, stop)

	errChan := make(chan error)

	// Start secondary caches.
	for _, cache := range o.caches {
		go func() {
			if err := cache.Start(stop); err != nil {
				errChan <- err
			}
		}()
		logrus.Infof("waiting for cache to sync")
		if !cache.WaitForCacheSync(stop) {
			return fmt.Errorf("failed to sync cache")
		}
		logrus.Infof("cache synced")
	}

	// Secondary caches are all synced, so start the manager.
	go func() {
		errChan <- o.manager.Start(stop)
	}()

	// Wait for the manager to exit or a secondary cache to fail.
	select {
	case <-stop:
		return nil
	case err := <-errChan:
		return err
	}
}

// ensureDefaultDNS creates the default cluster dns if it doesn't already exist.
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
			return fmt.Errorf("failed to create default cluster dns: %v", err)
		}
		logrus.Infof("created default cluster dns: %s", dns.Name)
	}
	return nil
}
