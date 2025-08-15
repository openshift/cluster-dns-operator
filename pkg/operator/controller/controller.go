package controller

import (
	"context"
	"fmt"
	"net"

	"k8s.io/apimachinery/pkg/util/sets"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	"github.com/openshift/cluster-dns-operator/pkg/util/slice"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/apparentlymart/go-cidr/cidr"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// DefaultDNSController is the name of the default DNS instance.
	DefaultDNSController = "default"

	// DNSControllerFinalizer is applied to a DNS before being considered for processing;
	// this ensures the operator has a chance to handle all states.
	DNSControllerFinalizer = "dns.operator.openshift.io/dns-controller"

	controllerName = "dns_controller"

	namespaceRunLevelLabel           = "openshift.io/run-level"             // "0"
	namespaceClusterMonitoringLabel  = "openshift.io/cluster-monitoring"    // "true"
	namespacePodSecurityEnforceLabel = "pod-security.kubernetes.io/enforce" // privileged
	namespacePodSecurityAuditLabel   = "pod-security.kubernetes.io/audit"   // privileged
	namespacePodSecurityWarnLabel    = "pod-security.kubernetes.io/warn"    // privileged
)

// managedDNSNamespaceLabels is a set of label keys for labels that the operator manages
// for the operand namespace
var managedDNSNamespaceLabels = sets.NewString(
	namespaceClusterMonitoringLabel,
	namespaceRunLevelLabel,
	namespacePodSecurityAuditLabel,
	namespacePodSecurityEnforceLabel,
	namespacePodSecurityWarnLabel,
)

// New creates the operator controller from configuration. This is the
// controller that handles all the logic for implementing dns based on
// DNS resources.
//
// The controller will be pre-configured to watch for DNS resources.
func New(mgr manager.Manager, config Config) (controller.Controller, error) {
	operatorCache := mgr.GetCache()
	reconciler := &reconciler{
		Config:                    config.Config,
		client:                    mgr.GetClient(),
		cache:                     operatorCache,
		dnsNameResolverEnabled:    config.DNSNameResolverEnabled,
		dnsNameResolverNamespaces: config.DNSNameResolverNamespaces,
	}
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	scheme := mgr.GetClient().Scheme()
	mapper := mgr.GetClient().RESTMapper()
	if err := c.Watch(source.Kind[client.Object](operatorCache, &operatorv1.DNS{}, &handler.EnqueueRequestForObject{})); err != nil {
		return nil, err
	}
	if err := c.Watch(source.Kind[client.Object](operatorCache, &appsv1.DaemonSet{}, handler.EnqueueRequestForOwner(scheme, mapper, &operatorv1.DNS{}))); err != nil {
		return nil, err
	}
	if err := c.Watch(source.Kind[client.Object](operatorCache, &corev1.Service{}, handler.EnqueueRequestForOwner(scheme, mapper, &operatorv1.DNS{}))); err != nil {
		return nil, err
	}
	if err := c.Watch(source.Kind[client.Object](operatorCache, &corev1.ConfigMap{}, handler.EnqueueRequestForOwner(scheme, mapper, &operatorv1.DNS{}))); err != nil {
		return nil, err
	}
	if err := c.Watch(source.Kind[client.Object](operatorCache, &networkingv1.NetworkPolicy{}, handler.EnqueueRequestForOwner(scheme, mapper, &operatorv1.DNS{}))); err != nil {
		return nil, err
	}

	objectToDNS := func(context.Context, client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: DefaultDNSNamespaceName()}}
	}
	isInNS := func(namespace string) func(o client.Object) bool {
		return func(o client.Object) bool {
			return o.GetNamespace() == namespace
		}
	}
	if err := c.Watch(source.Kind[client.Object](operatorCache, &corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(objectToDNS), predicate.NewPredicateFuncs(isInNS(GlobalUserSpecifiedConfigNamespace)))); err != nil {
		return nil, err
	}
	// If a node is created or deleted, then the controller may need to
	// reconcile the DNS service in order to add or remove the
	// service.kubernetes.io/topology-aware-hints annotation, but only if
	// the node isn't ignored for the purpose of determining whether to
	// enable topology-aware hints.
	nodePredicate := func(o client.Object) bool {
		node := o.(*corev1.Node)
		return !ignoreNodeForTopologyAwareHints(node)
	}
	if err := c.Watch(source.Kind[client.Object](operatorCache, &corev1.Node{}, handler.EnqueueRequestsFromMapFunc(objectToDNS), predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return nodePredicate(e.Object) },
		DeleteFunc: func(e event.DeleteEvent) bool { return nodePredicate(e.Object) },
		UpdateFunc: func(e event.UpdateEvent) bool {
			old := e.ObjectOld.(*corev1.Node)
			new := e.ObjectNew.(*corev1.Node)
			if ignoreNodeForTopologyAwareHints(old) != ignoreNodeForTopologyAwareHints(new) {
				return true
			}
			if !ignoreNodeForTopologyAwareHints(new) && nodeIsValidForTopologyAwareHints(old) != nodeIsValidForTopologyAwareHints(new) {
				return true
			}
			return false

		},
		GenericFunc: func(e event.GenericEvent) bool { return nodePredicate(e.Object) },
	})); err != nil {
		return nil, err
	}

	return c, nil
}

// Config holds all the configuration that must be provided when creating the
// controller.
type Config struct {
	// DNSNameResolverEnabled indicates that the "DNSNameResolver" featuregate is enabled.
	DNSNameResolverEnabled bool
	// DNSNameResolverNamespaces are the namespaces which will be watched for the
	// DNSNameResolver resources and updated by the CoreDNS pods if the "DNSNameResolver"
	// featuregate is enabled.
	DNSNameResolverNamespaces []string

	operatorconfig.Config
}

// reconciler handles the actual dns reconciliation logic in response to
// events.
type reconciler struct {
	operatorconfig.Config

	client client.Client
	cache  cache.Cache

	// dnsNameResolverEnabled indicates that the "DNSNameResolver" featuregate is enabled.
	dnsNameResolverEnabled bool
	// dnsNameResolverNamespaces are the namespaces which will be watched for the
	// DNSNameResolver resources and updated by the CoreDNS pods the "DNSNameResolver"
	// featuregate is enabled.
	dnsNameResolverNamespaces []string
}

// Reconcile expects request to refer to a dns and will do all the work
// to ensure the dns is in the desired state.
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	errs := []error{}
	result := reconcile.Result{}

	logrus.Infof("reconciling request: %v", request)

	if request.NamespacedName.Name != DefaultDNSController {
		// Return a nil error value to avoid re-triggering the event.
		logrus.Errorf("skipping unexpected dns %s", request.NamespacedName.Name)
		return result, nil
	}
	// Get the current dns state.
	dns := &operatorv1.DNS{}
	if err := r.client.Get(ctx, request.NamespacedName, dns); err != nil {
		if errors.IsNotFound(err) {
			// This means the dns was already deleted/finalized and there are
			// stale queue entries (or something edge triggering from a related
			// resource that got deleted async).
			logrus.Infof("dns not found; reconciliation will be skipped for request: %v", request)
		} else {
			errs = append(errs, fmt.Errorf("failed to get dns %s: %v", request, err))
		}
		dns = nil
	}

	if dns != nil {
		switch dns.Spec.OperatorLogLevel {
		case operatorv1.DNSLogLevelNormal:
			logrus.SetLevel(logrus.InfoLevel)
		case operatorv1.DNSLogLevelDebug:
			logrus.SetLevel(logrus.DebugLevel)
		case operatorv1.DNSLogLevelTrace:
			logrus.SetLevel(logrus.TraceLevel)
		default:
			logrus.SetLevel(logrus.InfoLevel)
		}
		switch dns.Spec.ManagementState {
		case operatorv1.Unmanaged:
			// When the operator is set to unmanaged, it should not make
			// changes to the DNS or node resolver pods, but it should still
			// update status
			//
			// TODO: fetch clusterDomain from higher level openshift resource
			// when it is exposed
			clusterDomain := "cluster.local"
			clusterIP, err := r.getClusterIPFromNetworkConfig()
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get cluster IP from network config: %v", err))
			}
			haveDNSDaemonset, dnsDaemonset, err := r.currentDNSDaemonSet(dns)
			if err != nil {
				errs = append(errs, err)
			}
			haveNodeResolverDaemonset, nodeResolverDaemonset, err := r.currentNodeResolverDaemonSet()
			if err != nil {
				errs = append(errs, err)
			}
			// 2*lameDuckDuration is used for transitionUnchangedToleration to add some room to cover lameDuckDuration when CoreDNS reports unavailable.
			// This is eventually used to prevent frequent updates.
			if err := r.syncDNSStatus(dns, clusterIP, clusterDomain, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset, 2*lameDuckDuration, &result); err != nil {
				errs = append(errs, fmt.Errorf("failed to sync status of dns %q: %w", dns.Name, err))
			}
		default:
			// Ensure we have all the necessary scaffolding on which to place dns instances.
			if err := r.ensureDNSNamespace(); err != nil {
				errs = append(errs, fmt.Errorf("failed to ensure dns namespace: %v", err))
			}

			if dns.DeletionTimestamp != nil {
				// Handle deletion.
				if err := r.ensureOpenshiftExternalNameServiceDeleted(); err != nil {
					errs = append(errs, fmt.Errorf("failed to delete external name for openshift service: %v", err))
				}
				if err := r.ensureDNSDeleted(dns); err != nil {
					errs = append(errs, fmt.Errorf("failed to ensure deletion for dns %s: %v", dns.Name, err))
				}

				if len(errs) == 0 {
					// Clean up the finalizer to allow the dns to be deleted.
					if slice.ContainsString(dns.Finalizers, DNSControllerFinalizer) {
						updated := dns.DeepCopy()
						updated.Finalizers = slice.RemoveString(updated.Finalizers, DNSControllerFinalizer)
						if err := r.client.Update(ctx, updated); err != nil {
							errs = append(errs, fmt.Errorf("failed to remove finalizer from dns %s: %v", dns.Name, err))
						}
					}
				}
			} else if err := r.enforceDNSFinalizer(dns); err != nil {
				errs = append(errs, fmt.Errorf("failed to enforce finalizer for dns %s: %v", dns.Name, err))
			} else {
				// Handle everything else.
				if err := r.ensureDNS(dns, &result); err != nil {
					errs = append(errs, fmt.Errorf("failed to ensure dns %s: %v", dns.Name, err))
				} else if err := r.ensureExternalNameForOpenshiftService(); err != nil {
					errs = append(errs, fmt.Errorf("failed to ensure external name for openshift service: %v", err))
				}
			}
		}
	}

	// Log in case of errors as the controller's logs get eaten.
	if len(errs) > 0 {
		logrus.Errorf("failed to reconcile request %s: %v", request, utilerrors.NewAggregate(errs))
	}
	return result, utilerrors.NewAggregate(errs)
}

// ensureExternalNameForOpenshiftService ensures 'openshift.default.svc'
// resolves to 'kubernetes.default.svc'.
// This will ensure backward compatibility with openshift 3.x
func (r *reconciler) ensureExternalNameForOpenshiftService() error {
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "kubernetes.default.svc.cluster.local",
		},
	}

	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}, svc); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get external name service %s/%s: %v", svc.Namespace, svc.Name, err)
		}

		if err := r.client.Create(context.TODO(), svc); err != nil {
			return fmt.Errorf("failed to create external name service %s/%s: %v", svc.Namespace, svc.Name, err)
		}
		logrus.Infof("created external name service %s/%s", svc.Namespace, svc.Name)
	}
	return nil
}

// ensureOpenshiftExternalNameServiceDeleted ensures deletion of 'openshift.default.svc'
func (r *reconciler) ensureOpenshiftExternalNameServiceDeleted() error {
	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift",
			Namespace: "default",
		},
	}
	if err := r.client.Delete(context.TODO(), svc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete external name service %s/%s: %v", svc.Namespace, svc.Name, err)
	}
	logrus.Infof("deleted external name service %s/%s", svc.Namespace, svc.Name)
	return nil
}

// enforceDNSFinalizer adds DNSControllerFinalizer to dns if it doesn't exist.
func (r *reconciler) enforceDNSFinalizer(dns *operatorv1.DNS) error {
	if !slice.ContainsString(dns.Finalizers, DNSControllerFinalizer) {
		dns.Finalizers = append(dns.Finalizers, DNSControllerFinalizer)
		if err := r.client.Update(context.TODO(), dns); err != nil {
			return err
		}
		logrus.Infof("enforced finalizer for dns: %s", dns.Name)
	}
	return nil
}

// ensureDNSDeleted tries to delete related dns resources.
func (r *reconciler) ensureDNSDeleted(dns *operatorv1.DNS) error {
	// DNS specific configmap and service has owner reference to daemonset.
	// So deletion of daemonset will trigger garbage collection of corresponding
	// configmap and service resources.
	if err := r.ensureDNSDaemonSetDeleted(dns); err != nil {
		return fmt.Errorf("failed to delete daemonset for dns %s: %v", dns.Name, err)
	}
	return nil
}

// ensureDNSNamespace ensures all the necessary scaffolding exists for
// dns generally, including a namespace and all RBAC setup.
func (r *reconciler) ensureDNSNamespace() error {
	existingNamespace := corev1.Namespace{}
	desiredNamespace := manifests.DNSNamespace()
	if err := r.client.Get(context.TODO(), DefaultDNSOperandNamespaceName(), &existingNamespace); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns namespace %q: %w", desiredNamespace.Name, err)
		}
		if err := r.client.Create(context.TODO(), desiredNamespace); err != nil {
			return fmt.Errorf("failed to create dns namespace %s: %w", desiredNamespace.Name, err)
		}
		logrus.Infof("created dns namespace: %s", desiredNamespace.Name)
	} else {
		// Make sure existing DNS namespace has all the labels from the Namespace manifest
		changed, updatedNamespace := namespaceLabelsChanged(&existingNamespace, desiredNamespace)
		if changed {
			if err := r.client.Update(context.TODO(), updatedNamespace); err != nil {
				return fmt.Errorf("failed to update dns namespace %s: %w", updatedNamespace.Name, err)
			}
			logrus.Infof("updated dns namespace: %s", updatedNamespace.Name)
		}
	}

	if _, _, err := r.ensureDNSClusterRole(); err != nil {
		return fmt.Errorf("failed to ensure dns cluster role for %s: %v", manifests.DNSClusterRole().Name, err)
	}

	crb := manifests.DNSClusterRoleBinding()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: crb.Name}, crb); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns cluster role binding %s: %v", crb.Name, err)
		}
		if err := r.client.Create(context.TODO(), crb); err != nil {
			return fmt.Errorf("failed to create dns cluster role binding %s: %v", crb.Name, err)
		}
		logrus.Infof("created dns cluster role binding: %s", crb.Name)
	}

	sa := manifests.DNSServiceAccount()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: sa.Namespace, Name: sa.Name}, sa); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns service account %s/%s: %v", sa.Namespace, sa.Name, err)
		}
		if err := r.client.Create(context.TODO(), sa); err != nil {
			return fmt.Errorf("failed to create dns service account %s/%s: %v", sa.Namespace, sa.Name, err)
		}
		logrus.Infof("created dns service account: %s/%s", sa.Namespace, sa.Name)
	}

	nodeResolverServiceAccount := manifests.NodeResolverServiceAccount()
	nodeResolverServiceAccountName := types.NamespacedName{
		Namespace: nodeResolverServiceAccount.Namespace,
		Name:      nodeResolverServiceAccount.Name,
	}
	if err := r.client.Get(context.TODO(), nodeResolverServiceAccountName, nodeResolverServiceAccount); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get serviceaccount %s: %w", nodeResolverServiceAccountName, err)
		}
		if err := r.client.Create(context.TODO(), nodeResolverServiceAccount); err != nil {
			return fmt.Errorf("failed to create serviceaccount %s: %w", nodeResolverServiceAccountName, err)
		}
		logrus.Infof("created serviceaccount %s", nodeResolverServiceAccountName)
	}

	// Ensure the deny all network policy is present for the dns namespace
	np := manifests.NetworkPolicyDenyAll()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: np.Namespace, Name: np.Name}, np); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get network policy deny all: %v", err)
		}
		if err := r.client.Create(context.TODO(), np); err != nil {
			return fmt.Errorf("failed to create dns deny all network policy %s/%s: %v", np.Namespace, np.Name, err)
		}
		logrus.Infof("created dns deny all network policy %s/%s", np.Namespace, np.Name)
	}

	return nil
}

// namespaceLabelsChanged generates a new namespace with the desired labels, and reports true if the existing namespace
// labels need to be updated to match the desired state.
func namespaceLabelsChanged(existingNamespace, desiredNamespace *corev1.Namespace) (bool, *corev1.Namespace) {
	changed := false
	updatedNamespace := existingNamespace.DeepCopy()
	if updatedNamespace.Labels == nil {
		updatedNamespace.Labels = map[string]string{}
	}

	for k := range managedDNSNamespaceLabels {
		existingVal, have := existingNamespace.Labels[k]
		desiredVal, want := desiredNamespace.Labels[k]
		if want && (!have || existingVal != desiredVal) {
			updatedNamespace.Labels[k] = desiredNamespace.Labels[k]
			changed = true
		}
	}
	return changed, updatedNamespace
}

// ensureMetricsIntegration ensures that dns prometheus metrics are integrated with openshift-monitoring for the given DNS.
func (r *reconciler) ensureMetricsIntegration(dns *operatorv1.DNS, svc *corev1.Service, daemonsetRef metav1.OwnerReference) error {
	cr := manifests.MetricsClusterRole()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name}, cr); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns metrics cluster role %s: %v", cr.Name, err)
		}
		if err := r.client.Create(context.TODO(), cr); err != nil {
			return fmt.Errorf("failed to create dns metrics cluster role %s: %v", cr.Name, err)
		}
		logrus.Infof("created dns metrics cluster role %s", cr.Name)
	}

	crb := manifests.MetricsClusterRoleBinding()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: crb.Name}, crb); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns metrics cluster role binding %s: %v", crb.Name, err)
		}
		if err := r.client.Create(context.TODO(), crb); err != nil {
			return fmt.Errorf("failed to create dns metrics cluster role binding %s: %v", crb.Name, err)
		}
		logrus.Infof("created dns metrics cluster role binding %s", crb.Name)
	}

	mr := manifests.MetricsRole()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: mr.Namespace, Name: mr.Name}, mr); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns metrics role %s/%s: %v", mr.Namespace, mr.Name, err)
		}
		if err := r.client.Create(context.TODO(), mr); err != nil {
			return fmt.Errorf("failed to create dns metrics role %s/%s: %v", mr.Namespace, mr.Name, err)
		}
		logrus.Infof("created dns metrics role %s/%s", mr.Namespace, mr.Name)
	}

	mrb := manifests.MetricsRoleBinding()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: mrb.Namespace, Name: mrb.Name}, mrb); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns metrics role binding %s/%s: %v", mrb.Namespace, mrb.Name, err)
		}
		if err := r.client.Create(context.TODO(), mrb); err != nil {
			return fmt.Errorf("failed to create dns metrics role binding %s/%s: %v", mrb.Namespace, mrb.Name, err)
		}
		logrus.Infof("created dns metrics role binding %s/%s", mrb.Namespace, mrb.Name)
	}

	if _, _, err := r.ensureServiceMonitor(dns, svc, daemonsetRef); err != nil {
		return fmt.Errorf("failed to ensure servicemonitor for %s: %v", dns.Name, err)
	}

	return nil
}

// ensureDNS ensures all necessary dns resources exist for a given dns.
func (r *reconciler) ensureDNS(dns *operatorv1.DNS, reconcileResult *reconcile.Result) error {
	// TODO: fetch this from higher level openshift resource when it is exposed
	clusterDomain := "cluster.local"
	clusterIP, err := r.getClusterIPFromNetworkConfig()
	if err != nil {
		return fmt.Errorf("failed to get cluster IP from network config: %v", err)
	}

	errs := []error{}

	if err := r.ensureCABundleConfigMaps(dns); err != nil {
		errs = append(errs, fmt.Errorf("failed to create ca bundle configmaps for dns %s: %w", dns.Name, err))
	}

	// cmMap is a map of ca bundle configmaps' names and their resource versions used in
	// the calculation of the path of ca bundles in Corefile and volume mount path in daemonset.
	// When the map is empty or missing a configmap because it is not found on the cluster,
	// the operator will print a warning, and it will not specify a path in Corefile for this configmap
	// which will lead to the default behavior of using the system certificates.
	// Also, the operator will not add a volume to daemonset for this configmap.
	cmMap := r.caBundleRevisionMap(dns)

	haveDNSDaemonset, dnsDaemonset, err := r.ensureDNSDaemonSet(dns, cmMap)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to ensure daemonset for dns %s: %v", dns.Name, err))
	} else if !haveDNSDaemonset {
		errs = append(errs, fmt.Errorf("failed to get daemonset for dns %s", dns.Name))
	} else {
		trueVar := true
		daemonsetRef := metav1.OwnerReference{
			APIVersion: "apps/v1",
			Kind:       "DaemonSet",
			Name:       dnsDaemonset.Name,
			UID:        dnsDaemonset.UID,
			Controller: &trueVar,
		}

		if _, _, err := r.ensureDNSConfigMap(dns, clusterDomain, cmMap); err != nil {
			errs = append(errs, fmt.Errorf("failed to create configmap for dns %s: %v", dns.Name, err))
		}
		if _, _, err := r.ensureDNSNetworkPolicy(dns); err != nil {
			errs = append(errs, fmt.Errorf("failed to ensure networkpolicy for dns %s: %v", dns.Name, err))
		}
		if haveSvc, svc, err := r.ensureDNSService(dns, clusterIP, daemonsetRef); err != nil {
			// Set clusterIP to an empty string to cause ClusterOperator to report
			// Available=False and Degraded=True.
			clusterIP = ""
			errs = append(errs, fmt.Errorf("failed to create service for dns %s: %v", dns.Name, err))
		} else if !haveSvc {
			errs = append(errs, fmt.Errorf("failed to get service for dns %s", dns.Name))
		} else if err := r.ensureMetricsIntegration(dns, svc, daemonsetRef); err != nil {
			errs = append(errs, fmt.Errorf("failed to integrate metrics with openshift-monitoring for dns %s: %v", dns.Name, err))
		}
	}

	haveNodeResolverDaemonset, nodeResolverDaemonset, err := r.ensureNodeResolverDaemonSet(dns, clusterIP, clusterDomain)
	if err != nil {
		errs = append(errs, err)
	}

	// 2*lameDuckDuration is used for transitionUnchangedToleration to add some room to cover lameDuckDuration when CoreDNS reports unavailable.
	// This is eventually used to prevent frequent updates.
	if err := r.syncDNSStatus(dns, clusterIP, clusterDomain, haveDNSDaemonset, dnsDaemonset, haveNodeResolverDaemonset, nodeResolverDaemonset, 2*lameDuckDuration, reconcileResult); err != nil {
		errs = append(errs, fmt.Errorf("failed to sync status of dns %q: %w", dns.Name, err))
	}

	return utilerrors.NewAggregate(errs)
}

// caBundleRevisionMap generates a map of ca bundle configmaps with their resource versions
// to be used in Corefile as the path of ca bundles and in daemonset volume mount path.
// Resource version is appended to enable automatic reload of Corefile when there is a change
// in the source ca bundle configmap (e.g. cert rotation). Apart from resource versions,
// server name is also used in the path of ca bundles in case the same ca bundle configmap
// is specified for two different servers.
func (r *reconciler) caBundleRevisionMap(dns *operatorv1.DNS) map[string]string {
	caBundleRevisions := map[string]string{}
	transportConfig := dns.Spec.UpstreamResolvers.TransportConfig
	if transportConfig.Transport == operatorv1.TLSTransport {
		if transportConfig.TLS != nil && transportConfig.TLS.CABundle.Name != "" {
			name := CABundleConfigMapName(transportConfig.TLS.CABundle.Name)
			cm := &corev1.ConfigMap{}
			err := r.client.Get(context.TODO(), name, cm)
			if err != nil {
				logrus.Warningf("failed to get destination ca bundle configmap %s: %v", name.Name, err)
			} else {
				caBundleRevisions[transportConfig.TLS.CABundle.Name] = fmt.Sprintf("%s-%s", cm.Name, cm.ResourceVersion)
			}
		}
	}

	for _, server := range dns.Spec.Servers {
		transportConfig := server.ForwardPlugin.TransportConfig
		if transportConfig.Transport == operatorv1.TLSTransport {
			if transportConfig.TLS != nil && transportConfig.TLS.CABundle.Name != "" {
				name := CABundleConfigMapName(transportConfig.TLS.CABundle.Name)
				cm := &corev1.ConfigMap{}
				err := r.client.Get(context.TODO(), name, cm)
				if err != nil {
					logrus.Warningf("failed to get destination ca bundle configmap %s: %v", name.Name, err)
				} else {
					caBundleRevisions[transportConfig.TLS.CABundle.Name] = fmt.Sprintf("%s-%s", cm.Name, cm.ResourceVersion)
				}
			}
		}
	}

	return caBundleRevisions
}

// getClusterIPFromNetworkConfig will return 10th IP from the service CIDR range
// defined in the cluster network config.
func (r *reconciler) getClusterIPFromNetworkConfig() (string, error) {
	networkConfig := &configv1.Network{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, networkConfig); err != nil {
		return "", fmt.Errorf("failed to get network 'cluster': %v", err)
	}

	if len(networkConfig.Status.ServiceNetwork) == 0 {
		return "", fmt.Errorf("no service networks found in cluster network config")
	}
	_, serviceCIDR, err := net.ParseCIDR(networkConfig.Status.ServiceNetwork[0])
	if err != nil {
		return "", fmt.Errorf("invalid service cidr %s: %v", networkConfig.Status.ServiceNetwork[0], err)
	}

	dnsClusterIP, err := cidr.Host(serviceCIDR, 10)
	if err != nil {
		return "", fmt.Errorf("invalid service cidr %v: %v", serviceCIDR, err)
	}
	return dnsClusterIP.String(), nil
}

func dnsOwnerRef(dns *operatorv1.DNS) metav1.OwnerReference {
	trueVar := true
	return metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1",
		Kind:       "DNS",
		Name:       dns.Name,
		UID:        dns.UID,
		Controller: &trueVar,
	}
}
