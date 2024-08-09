package dnsnameresolver

import (
	"context"
	"strings"
	"sync"
	"time"

	ocpnetworkv1alpha1 "github.com/openshift/api/network/v1alpha1"

	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	ipRemovalGracePeriod = time.Second * 5
	controllerName       = "dnsnameresolver_controller"
)

var (
	controllerLog = ctrl.Log.WithName(controllerName)
)

type Config struct {
	OperandNamespace         string
	ServiceName              string
	DNSPort                  string
	DNSNameResolverNamespace string
}

// reconciler handles the actual DNSNameResolver reconciliation logic in response to events.
type reconciler struct {
	dnsNameResolverCache cache.Cache
	client               client.Client
	resolver             *Resolver
	startResolver        sync.Once
}

// NewUnmanaged creates and returns a controller that watches DNSNameResolver
// objects. The controller re-resolves the DNS names which get added to the
// status of the DNSNameResolver objects. It also removes IP addresses of DNS
// names, whose TTLs have expired, from the status.
func New(mgr manager.Manager, config Config) (controller.Controller, error) {
	// Create a new cache for tracking the DNSNameResolver resources in
	// the DNSNameResolverNamespace.
	dnsNameResolverCache, err := cache.New(mgr.GetConfig(), cache.Options{
		Scheme: mgr.GetScheme(),
		DefaultNamespaces: map[string]cache.Config{
			config.DNSNameResolverNamespace: {},
		},
	})
	if err != nil {
		return nil, err
	}

	// Create a new cache to track the EndpointSlices corresponding to the
	// CoreDNS pods.
	corednsEndpointsSliceCache, err := cache.New(mgr.GetConfig(), cache.Options{
		Scheme: mgr.GetScheme(),
		DefaultNamespaces: map[string]cache.Config{
			config.OperandNamespace: {},
		},
		DefaultLabelSelector: labels.SelectorFromSet(labels.Set{
			discoveryv1.LabelServiceName: config.ServiceName,
		}),
	})
	if err != nil {
		return nil, err
	}

	mgr.Add(dnsNameResolverCache)
	mgr.Add(corednsEndpointsSliceCache)

	reconciler := &reconciler{
		dnsNameResolverCache: dnsNameResolverCache,
		client:               mgr.GetClient(),
		resolver:             NewResolver(corednsEndpointsSliceCache, config.DNSPort),
	}

	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}

	// Watch for the DNSNameResolver resources using dnsNameResolverCache.
	if err := c.Watch(source.Kind[client.Object](dnsNameResolverCache, &ocpnetworkv1alpha1.DNSNameResolver{}, &handler.EnqueueRequestForObject{})); err != nil {
		return nil, err
	}

	// Watch for the CoreDNS pod EndpointSlices to keep corednsEndpointsSliceCache synced.
	// No reconcile requests should be generated.
	if err := c.Watch(source.Kind[client.Object](corednsEndpointsSliceCache, &discoveryv1.EndpointSlice{}, handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
		return nil
	}))); err != nil {
		return nil, err
	}

	return c, nil
}

// Reconcile expects request to refer to an DNSNameResolver resource, and will do all the work to
// keep the status of the resource updated.
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	// Ensure that the resolver is started before doing anything else.
	r.startResolver.Do(func() {
		r.resolver.Start()
	})

	controllerLog.Info("reconciling", "request", request)

	// Get the DNSNameResolver resource.
	dnsNameResolverObj := &ocpnetworkv1alpha1.DNSNameResolver{}
	if err := r.dnsNameResolverCache.Get(ctx, request.NamespacedName, dnsNameResolverObj); err != nil {

		// Check if the DNSNameResolver resource is deleted. If so, delete DNS names matching the DNSNameResolver resource
		// from the resolver.
		if errors.IsNotFound(err) {
			r.resolver.DeleteResolvedName(dnsDetails{objName: request.Name})
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	// Get a copy of the DNSNameResolver resource.
	dnsNameResolver := dnsNameResolverObj.DeepCopy()

	// Check if the grace period is over for some of the IP addresses after the expiration of their respective TTLs. If so,
	// remove those IP addresses and update the status of the resource.
	if removalOfIPsRequired(&dnsNameResolver.Status) {
		if err := r.client.Status().Update(ctx, dnsNameResolver); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Check if the TTLs of some of the IP addresses have expired. If so, requeue
	// the reconcile request after the minimum remaining time until the grace
	// period gets over among that of the IP addresses with expired TTLs.
	if ttlExpired, remainingTime := reconcileRequired(&dnsNameResolver.Status); ttlExpired {
		return reconcile.Result{Requeue: true, RequeueAfter: remainingTime}, nil
	}

	dnsName := string(dnsNameResolver.Spec.Name)
	matchesRegular := !isWildcard(dnsName)

	// Call the Add function of the resolver for the DNS name in the spec of the
	// DNSNameResolver resource with empty resolved addresses. If it is a create
	// event then this will ensure that the resolver sends the resolution request
	// for the DNS name, so that the status of the object gets updated with the
	// corresponding IP addresses.
	r.resolver.AddResolvedName(dnsDetails{
		dnsName:           dnsName,
		resolvedAddresses: []ocpnetworkv1alpha1.DNSNameResolverResolvedAddress{},
		matchesRegular:    matchesRegular,
		objName:           request.Name,
	})

	// Iterate through each of the resolved names matching the DNS name and add
	// them to the resolver along with the current IP addresses.
	for _, resolvedName := range dnsNameResolver.Status.ResolvedNames {
		r.resolver.AddResolvedName(dnsDetails{
			dnsName:           string(resolvedName.DNSName),
			resolvedAddresses: resolvedName.ResolvedAddresses,
			matchesRegular:    matchesRegular,
			objName:           request.Name,
		})
	}

	return reconcile.Result{}, nil
}

// isWildcard checks if the domain name is wildcard.
func isWildcard(dnsName string) bool {
	return strings.HasPrefix(dnsName, "*.")
}

// removalOfIPsRequired checks whether the TTL of any IP address, associated to a DNS name
// matching the DNSNameResolver object, has expired and the grace period after the TTL
// expiration is also over. If so, then the DNSNameResolverStatus status object is modified
// to remove those resolved addresses from the resolved name details of the DNS name.
func removalOfIPsRequired(status *ocpnetworkv1alpha1.DNSNameResolverStatus) bool {
	updated := false
	for index, resolvedName := range status.ResolvedNames {
		updateResolvedAddresses := []ocpnetworkv1alpha1.DNSNameResolverResolvedAddress{}
		for _, resolvedAddress := range resolvedName.ResolvedAddresses {
			if time.Now().Before(resolvedAddress.LastLookupTime.Time.Add(
				time.Second*time.Duration(resolvedAddress.TTLSeconds) + ipRemovalGracePeriod)) {
				updateResolvedAddresses = append(updateResolvedAddresses, resolvedAddress)
			} else {
				updated = true
			}
		}
		status.ResolvedNames[index].ResolvedAddresses = updateResolvedAddresses
	}
	return updated
}

// reconcileRequired checks whether the TTL of any IP address, associated to a DNS name
// matching the DNSNameResolver object, has expired. If so, then the minimum remaining
// time until the grace period gets over among that of the IP addresses with expired
// TTLs is returned.
func reconcileRequired(status *ocpnetworkv1alpha1.DNSNameResolverStatus) (bool, time.Duration) {
	var minRemainingTime time.Duration
	ttlExpired := false
	for _, resolvedName := range status.ResolvedNames {
		for _, addr := range resolvedName.ResolvedAddresses {
			if !time.Now().Before(addr.LastLookupTime.Time.Add(time.Second * time.Duration(addr.TTLSeconds))) {
				remainingTime := time.Until(addr.LastLookupTime.Time.Add(
					time.Second*time.Duration(addr.TTLSeconds) + ipRemovalGracePeriod))
				if !ttlExpired {
					ttlExpired = true
					minRemainingTime = remainingTime
				} else if remainingTime < minRemainingTime {
					minRemainingTime = remainingTime
				}
			}
		}
	}
	return ttlExpired, minRemainingTime
}
