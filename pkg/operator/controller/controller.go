package controller

import (
	"context"
	"fmt"
	"net"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	operatorclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	"github.com/openshift/cluster-dns-operator/pkg/util/slice"

	"k8s.io/client-go/rest"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	"github.com/apparentlymart/go-cidr/cidr"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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
)

// New creates the operator controller from configuration. This is the
// controller that handles all the logic for implementing dns based on
// cluster DNS resources.
//
// The controller will be pre-configured to watch for DNS resources.
func New(mgr manager.Manager, config Config) (controller.Controller, error) {
	kubeClient, err := operatorclient.NewClient(config.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %v", err)
	}

	reconciler := &reconciler{
		Config: config,
		client: kubeClient,
	}
	c, err := controller.New("operator-controller", mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return nil, err
	}
	if err := c.Watch(&source.Kind{Type: &operatorv1.DNS{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}
	return c, nil
}

// Config holds all the things necessary for the controller to run.
type Config struct {
	KubeConfig             *rest.Config
	CoreDNSImage           string
	OpenshiftCLIImage      string
	OperatorReleaseVersion string
}

// reconciler handles the actual dns reconciliation logic in response to
// events.
type reconciler struct {
	Config

	// client is the kube Client and it will refresh scheme/mapper fields if needed
	// to detect some resources like ServiceMonitor which could get registered after
	// the client creation.
	// Since this controller is running in single threaded mode,
	// we do not need to synchronize when changing rest scheme/mapper fields.
	client kclient.Client
}

// Reconcile expects request to refer to a cluster dns and will do all the work
// to ensure the cluster dns is in the desired state.
func (r *reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	errs := []error{}
	result := reconcile.Result{}

	logrus.Infof("reconciling request: %v", request)

	if request.NamespacedName.Name != DefaultDNSController {
		// Return a nil error value to avoid re-triggering the event.
		logrus.Errorf("skipping unexpected cluster dns %s", request.NamespacedName.Name)
		return result, nil
	}
	// Get the current dns state.
	dns := &operatorv1.DNS{}
	if err := r.client.Get(context.TODO(), request.NamespacedName, dns); err != nil {
		if errors.IsNotFound(err) {
			// This means the dns was already deleted/finalized and there are
			// stale queue entries (or something edge triggering from a related
			// resource that got deleted async).
			logrus.Infof("cluster dns not found; reconciliation will be skipped for request: %v", request)
		} else {
			errs = append(errs, fmt.Errorf("failed to get cluster dns %s: %v", request, err))
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
				errs = append(errs, fmt.Errorf("failed to ensure deletion for cluster dns %s: %v", dns.Name, err))
			}

			if len(errs) == 0 {
				// Clean up the finalizer to allow the cluster dns to be deleted.
				if slice.ContainsString(dns.Finalizers, DNSControllerFinalizer) {
					updated := dns.DeepCopy()
					updated.Finalizers = slice.RemoveString(updated.Finalizers, DNSControllerFinalizer)
					if err := r.client.Update(context.TODO(), updated); err != nil {
						errs = append(errs, fmt.Errorf("failed to remove finalizer from cluster dns %s: %v", dns.Name, err))
					}
				}
			}
		} else if err := r.enforceDNSFinalizer(dns); err != nil {
			errs = append(errs, fmt.Errorf("failed to enforce finalizer for cluster dns %s: %v", dns.Name, err))
		} else {
			// Handle everything else.
			if err := r.ensureClusterDNS(dns); err != nil {
				errs = append(errs, fmt.Errorf("failed to ensure cluster dns %s: %v", dns.Name, err))
			} else if err := r.ensureExternalNameForOpenshiftService(); err != nil {
				errs = append(errs, fmt.Errorf("failed to ensure external name for openshift service: %v", err))
			}
		}
	}

	// TODO: Should this be another controller?
	if err := r.syncOperatorStatus(); err != nil {
		errs = append(errs, fmt.Errorf("failed to sync operator status: %v", err))
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

// ensureDNSDeleted tries to delete related cluster dns resources.
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

	cr := manifests.DNSClusterRole()
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: cr.Name}, cr); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns cluster role %s: %v", cr.Name, err)
		}
		if err := r.client.Create(context.TODO(), cr); err != nil {
			return fmt.Errorf("failed to create dns cluster role %s: %v", cr.Name, err)
		}
		logrus.Infof("created dns cluster role: %s", cr.Name)
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

// ensureClusterDNS ensures all necessary dns resources exist for a given cluster dns.
func (r *reconciler) ensureClusterDNS(dns *operatorv1.DNS) error {
	// TODO: fetch this from higher level openshift resource when it is exposed
	clusterDomain := "cluster.local"
	clusterIP, err := r.getClusterIPFromNetworkConfig()
	if err != nil {
		return fmt.Errorf("failed to get cluster IP from network config: %v", err)
	}

	errs := []error{}
	if daemonset, err := r.ensureDNSDaemonSet(dns, clusterIP, clusterDomain); err != nil {
		errs = append(errs, fmt.Errorf("failed to ensure daemonset for cluster dns %s: %v", dns.Name, err))
	} else {
		trueVar := true
		daemonsetRef := metav1.OwnerReference{
			APIVersion: "apps/v1",
			Kind:       "DaemonSet",
			Name:       daemonset.Name,
			UID:        daemonset.UID,
			Controller: &trueVar,
		}

		if _, err := r.ensureDNSConfigMap(dns, clusterDomain, daemonsetRef); err != nil {
			errs = append(errs, fmt.Errorf("failed to create configmap for cluster dns %s: %v", dns.Name, err))
		}
		if _, err := r.ensureDNSService(dns, clusterIP, daemonsetRef); err != nil {
			errs = append(errs, fmt.Errorf("failed to create service for cluster dns %s: %v", dns.Name, err))
		}

		if err := r.syncClusterDNSStatus(dns, clusterIP, clusterDomain); err != nil {
			errs = append(errs, fmt.Errorf("failed to sync status of cluster dns %s/%s: %v", daemonset.Namespace, daemonset.Name, err))
		}
	}

	return utilerrors.NewAggregate(errs)
}

// syncClusterDNSStatus updates the status for a given cluster dns.
func (r *reconciler) syncClusterDNSStatus(dns *operatorv1.DNS, clusterIP, clusterDomain string) error {
	current := &operatorv1.DNS{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: dns.Name}, current); err != nil {
		return fmt.Errorf("failed to get cluster dns %s: %v", dns.Name, err)
	}
	if current.Status.ClusterIP == clusterIP &&
		current.Status.ClusterDomain == clusterDomain {
		return nil
	}
	current.Status.ClusterIP = clusterIP
	current.Status.ClusterDomain = clusterDomain

	if err := r.client.Status().Update(context.TODO(), current); err != nil {
		return fmt.Errorf("failed to update status for cluster dns %s: %v", current.Name, err)
	}
	return nil
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
