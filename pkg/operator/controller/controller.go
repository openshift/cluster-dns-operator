package controller

import (
	"context"
	"fmt"
	"net"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	"github.com/openshift/cluster-dns-operator/pkg/util/slice"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/apparentlymart/go-cidr/cidr"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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
)

// New creates the operator controller from configuration. This is the
// controller that handles all the logic for implementing dns based on
// DNS resources.
//
// The controller will be pre-configured to watch for DNS resources.
func New(mgr manager.Manager, config operatorconfig.Config) (controller.Controller, error) {
	reconciler := &reconciler{
		Config: config,
		client: mgr.GetClient(),
		cache:  mgr.GetCache(),
	}
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &operatorv1.DNS{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{OwnerType: &operatorv1.DNS{}}); err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{OwnerType: &operatorv1.DNS{}}); err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{OwnerType: &operatorv1.DNS{}}); err != nil {
		return nil, err
	}
	return c, nil
}

// reconciler handles the actual dns reconciliation logic in response to
// events.
type reconciler struct {
	operatorconfig.Config

	client client.Client
	cache  cache.Cache
}

// Reconcile expects request to refer to a dns and will do all the work
// to ensure the dns is in the desired state.
func (r *reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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
	if err := r.client.Get(context.TODO(), request.NamespacedName, dns); err != nil {
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
					if err := r.client.Update(context.TODO(), updated); err != nil {
						errs = append(errs, fmt.Errorf("failed to remove finalizer from dns %s: %v", dns.Name, err))
					}
				}
			}
		} else if err := r.enforceDNSFinalizer(dns); err != nil {
			errs = append(errs, fmt.Errorf("failed to enforce finalizer for dns %s: %v", dns.Name, err))
		} else {
			// Handle everything else.
			if err := r.ensureDNS(dns); err != nil {
				errs = append(errs, fmt.Errorf("failed to ensure dns %s: %v", dns.Name, err))
			} else if err := r.ensureExternalNameForOpenshiftService(); err != nil {
				errs = append(errs, fmt.Errorf("failed to ensure external name for openshift service: %v", err))
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
	ns := manifests.DNSNamespace()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: ns.Name}, ns); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns namespace %q: %v", ns.Name, err)
		}
		if err := r.client.Create(context.TODO(), ns); err != nil {
			return fmt.Errorf("failed to create dns namespace %s: %v", ns.Name, err)
		}
		logrus.Infof("created dns namespace: %s", ns.Name)
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

	return nil
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
func (r *reconciler) ensureDNS(dns *operatorv1.DNS) error {
	// TODO: fetch this from higher level openshift resource when it is exposed
	clusterDomain := "cluster.local"
	clusterIP, err := r.getClusterIPFromNetworkConfig()
	if err != nil {
		return fmt.Errorf("failed to get cluster IP from network config: %v", err)
	}

	errs := []error{}

	// In 4.6 and earlier, the node resolver runs as a container in the
	// daemonset that ensureDNSDaemonSet manages, and if a separate node
	// resolver daemonset exists, then ensureNodeResolverDaemonset deletes
	// it.  We want to delete the node resolver daemonset before updating
	// the DNS daemonset to avoid having both the node resolver running in
	// both daemonsets at the same time.
	if _, _, err := r.ensureNodeResolverDaemonSet(clusterIP, clusterDomain); err != nil {
		errs = append(errs, err)
	}

	if haveDS, daemonset, err := r.ensureDNSDaemonSet(dns, clusterIP, clusterDomain); err != nil {
		errs = append(errs, fmt.Errorf("failed to ensure daemonset for dns %s: %v", dns.Name, err))
	} else if !haveDS {
		errs = append(errs, fmt.Errorf("failed to get daemonset for dns %s", dns.Name))
	} else {
		trueVar := true
		daemonsetRef := metav1.OwnerReference{
			APIVersion: "apps/v1",
			Kind:       "DaemonSet",
			Name:       daemonset.Name,
			UID:        daemonset.UID,
			Controller: &trueVar,
		}

		if _, _, err := r.ensureDNSConfigMap(dns, clusterDomain); err != nil {
			errs = append(errs, fmt.Errorf("failed to create configmap for dns %s: %v", dns.Name, err))
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

		if err := r.syncDNSStatus(dns, clusterIP, clusterDomain, daemonset); err != nil {
			errs = append(errs, fmt.Errorf("failed to sync status of dns %s/%s: %v", daemonset.Namespace, daemonset.Name, err))
		}
	}

	return utilerrors.NewAggregate(errs)
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
