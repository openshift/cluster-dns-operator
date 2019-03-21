package stub

import (
	"context"
	"fmt"
	"net"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	"github.com/openshift/cluster-dns-operator/pkg/operator"
	"github.com/openshift/cluster-dns-operator/pkg/util/slice"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/apparentlymart/go-cidr/cidr"
)

const (
	// ClusterDNSFinalizer is applied to all ClusterDNS resources before they are
	// considered for processing; this ensures the operator has a chance to handle
	// all states.
	// TODO: Make this generic and not tied to the "default" clusterdns.
	ClusterDNSFinalizer = "dns.openshift.io/default-cluster-dns"
)

type Handler struct {
	Config          operator.Config
	ManifestFactory *manifests.Factory
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	defer h.syncOperatorStatus()

	// TODO: This should be adding an item to a rate limited work queue, but for
	// now correctness is more important than performance.
	switch o := event.Object.(type) {
	case *operatorv1.DNS:
		logrus.Infof("reconciling for update to dns %s", o.Name)
	}
	return h.reconcile()
}

// EnsureDefaultClusterDNS ensures that the default ClusterDNS exists.
func (h *Handler) EnsureDefaultClusterDNS() error {
	desired, err := h.ManifestFactory.ClusterDNSDefaultCR()
	if err != nil {
		return fmt.Errorf("failed to build default clusterdns: %v", err)
	}
	if err := sdk.Get(desired); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get clusterdns %s: %v", desired.Name, err)
		}
		if err := sdk.Create(desired); err != nil {
			return fmt.Errorf("failed to create clusterdns %s: %v", desired.Name, err)
		}
		logrus.Infof("created clusterdns %s", desired.Name)
	}
	return nil
}

// Reconcile performs a full reconciliation loop for DNS, including
// generalized setup and handling of all clusterdns resources in the
// operator namespace.
func (h *Handler) reconcile() error {
	// Ensure we have all the necessary scaffolding on which to place DNS
	// instances.
	if err := h.ensureDNSNamespace(); err != nil {
		return fmt.Errorf("failed to ensure dns namespace: %v", err)
	}

	// Find all clusterdnses.
	dnses := &operatorv1.DNSList{}
	if err := sdk.List(corev1.NamespaceAll, dnses, sdk.WithListOptions(&metav1.ListOptions{})); err != nil {
		return fmt.Errorf("failed to list clusterdnses: %v", err)
	}

	// Reconcile all the clusterdnses.
	errors := []error{}
	for _, dns := range dnses.Items {
		if dns.Name != "default" {
			// We do not want to return this error to avoid re-triggering the event
			logrus.Errorf("skipping unexpected clusterdns %s", dns.Name)
			continue
		}
		// Handle deleted dns.
		// TODO: Assert/ensure that the dns has a finalizer so we can reliably detect
		// deletion.
		if dns.DeletionTimestamp != nil {
			if err := h.ensureOpenshiftExternalNameServiceDeleted(); err != nil {
				errors = append(errors, fmt.Errorf("failed to delete external name for openshift service: %v", err))
			}
			// Destroy any coredns instance associated with the dns.
			if err := h.ensureDNSDeleted(&dns); err != nil {
				errors = append(errors, fmt.Errorf("failed to delete clusterdns %s: %v", dns.Name, err))
			}

			if len(errors) == 0 {
				// Clean up the finalizer to allow the clusterdns to be deleted.
				if slice.ContainsString(dns.Finalizers, ClusterDNSFinalizer) {
					dns.Finalizers = slice.RemoveString(dns.Finalizers, ClusterDNSFinalizer)
					if err := sdk.Update(&dns); err != nil {
						errors = append(errors, fmt.Errorf("failed to remove finalizer from clusterdns %s: %v", dns.Name, err))
					}
				}
			}
		} else {
			// Handle active DNS.
			if err := h.ensureCoreDNSForClusterDNS(&dns); err != nil {
				errors = append(errors, fmt.Errorf("failed to ensure clusterdns %s: %v", dns.Name, err))
			} else if err := h.ensureExternalNameForOpenshiftService(); err != nil {
				errors = append(errors, fmt.Errorf("failed to ensure external name for openshift service: %v", err))
			}
		}
	}
	return utilerrors.NewAggregate(errors)
}

// ensureDNSNamespace ensures all the necessary scaffolding exists for
// CoreDNS generally, including a namespace and all RBAC setup.
func (h *Handler) ensureDNSNamespace() error {
	ns, err := h.ManifestFactory.DNSNamespace()
	if err != nil {
		return fmt.Errorf("failed to build dns namespace: %v", err)
	}
	if err := sdk.Get(ns); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns namespace %s: %v", ns.Name, err)
		}
		if err := sdk.Create(ns); err != nil {
			return fmt.Errorf("failed to create dns namespace %s: %v", ns.Name, err)
		}
		logrus.Infof("created dns namespace %s", ns.Name)
	}

	sa, err := h.ManifestFactory.DNSServiceAccount()
	if err != nil {
		return fmt.Errorf("failed to build dns service account: %v", err)
	}
	if err := sdk.Get(sa); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns service account %s: %v", sa.Name, err)
		}
		if err := sdk.Create(sa); err != nil {
			return fmt.Errorf("failed to create dns service account %s: %v", sa.Name, err)
		}
		logrus.Infof("created dns service account %s", sa.Name)
	}

	cr, err := h.ManifestFactory.DNSClusterRole()
	if err != nil {
		return fmt.Errorf("failed to build dns cluster role: %v", err)
	}
	if err := sdk.Get(cr); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns cluster role %s: %v", cr.Name, err)
		}
		if err := sdk.Create(cr); err != nil {
			return fmt.Errorf("failed to create dns cluster role %s: %v", cr.Name, err)
		}
		logrus.Infof("created dns cluster role %s", cr.Name)
	}

	crb, err := h.ManifestFactory.DNSClusterRoleBinding()
	if err != nil {
		return fmt.Errorf("failed to build dns cluster role binding: %v", err)
	}
	if err := sdk.Get(crb); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns cluster role binding %s: %v", crb.Name, err)
		}
		if err := sdk.Create(crb); err != nil {
			return fmt.Errorf("failed to create dns cluster role binding %s: %v", crb.Name, err)
		}
		logrus.Infof("created dns cluster role binding %s", crb.Name)
	}
	return nil
}

// ensureCoreDNSForClusterDNS ensures all necessary CoreDNS resources exist for
// a given clusterdns.
func (h *Handler) ensureCoreDNSForClusterDNS(dns *operatorv1.DNS) error {
	// TODO: fetch this from higher level openshift resource when it is exposed
	clusterDomain := "cluster.local"
	clusterIP, err := getClusterIPFromNetworkConfig()
	if err != nil {
		return fmt.Errorf("failed to fetch cluster IP from network config for clusterdns %s, %v", dns.Name, err)
	}

	ds, err := h.ManifestFactory.DNSDaemonSet(dns, clusterIP, clusterDomain)
	if err != nil {
		return fmt.Errorf("failed to build dns daemonset: %v", err)
	}
	if err := sdk.Get(ds); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns daemonset %s: %v", ds.Name, err)
		}
		if err := sdk.Create(ds); err != nil {
			return fmt.Errorf("failed to create dns daemonset %s: %v", ds.Name, err)
		}
		logrus.Infof("created dns daemonset %s", ds.Name)
	}

	needsUpdate := false
	for i, c := range ds.Spec.Template.Spec.Containers {
		switch c.Name {
		case "dns":
			if h.Config.CoreDNSImage != ds.Spec.Template.Spec.Containers[i].Image {
				ds.Spec.Template.Spec.Containers[i].Image = h.Config.CoreDNSImage
				needsUpdate = true
			}
		case "dns-node-resolver":
			if h.Config.OpenshiftCLIImage != ds.Spec.Template.Spec.Containers[i].Image {
				ds.Spec.Template.Spec.Containers[i].Image = h.Config.OpenshiftCLIImage
				needsUpdate = true
			}
		}
	}
	if needsUpdate {
		if err := sdk.Update(ds); err != nil {
			return fmt.Errorf("failed to update dns daemonset %s: %v", ds.Name, err)
		}
		logrus.Infof("updated dns daemonset %s", ds.Name)
	}

	trueVar := true
	dsRef := metav1.OwnerReference{
		APIVersion: ds.APIVersion,
		Kind:       ds.Kind,
		Name:       ds.Name,
		UID:        ds.UID,
		Controller: &trueVar,
	}

	cm, err := h.ManifestFactory.DNSConfigMap(dns, clusterDomain)
	if err != nil {
		return fmt.Errorf("failed to build dns config map: %v", err)
	}
	if err := sdk.Get(cm); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns config map %s: %v", cm.Name, err)
		}
		cm.SetOwnerReferences([]metav1.OwnerReference{dsRef})
		if err := sdk.Create(cm); err != nil {
			return fmt.Errorf("failed to create dns config map %s: %v", cm.Name, err)
		}
		logrus.Infof("created dns config map %s", cm.Name)
	}

	service, err := h.ManifestFactory.DNSService(dns, clusterIP)
	if err != nil {
		return fmt.Errorf("failed to build service: %v", err)
	}
	if err := sdk.Get(service); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get dns service %s: %v", service.Name, err)
		}
		service.SetOwnerReferences([]metav1.OwnerReference{dsRef})
		if err := sdk.Create(service); err != nil {
			return fmt.Errorf("failed to create dns service %s: %v", service.Name, err)
		}
		logrus.Infof("created dns service %s", service.Name)
	}

	if err := syncClusterDNSStatus(dns, clusterIP, clusterDomain); err != nil {
		return fmt.Errorf("failed to sync dns status %s, %v", dns.Name, err)
	}
	return nil
}

// ensureExternalNameForOpenshiftService ensures 'openshift.default.svc'
// resolves to 'kubernetes.default.svc'.
// This will ensure backward compatibility with openshift 3.x
func (h *Handler) ensureExternalNameForOpenshiftService() error {
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

	if err := sdk.Get(svc); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to get external name service %s/%s: %v", svc.Namespace, svc.Name, err)
		}

		if err := sdk.Create(svc); err != nil {
			return fmt.Errorf("failed to create external name service %s/%s: %v", svc.Namespace, svc.Name, err)
		}
		logrus.Infof("created external name service %s/%s", svc.Namespace, svc.Name)
	}
	return nil
}

// ensureOpenshiftExternalNameServiceDeleted ensures deletion of 'openshift.default.svc'
func (h *Handler) ensureOpenshiftExternalNameServiceDeleted() error {
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
	if err := sdk.Delete(svc); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete external name service %s/%s: %v", svc.Namespace, svc.Name, err)
	}
	logrus.Infof("deleted external name service %s/%s", svc.Namespace, svc.Name)
	return nil
}

// ensureDNSDeleted ensures that any CoreDNS resources associated with the
// clusterdns are deleted.
func (h *Handler) ensureDNSDeleted(dns *operatorv1.DNS) error {
	// DNS specific configmap and service has owner reference to daemonset.
	// So deletion of daemonset will trigger garbage collection of corresponding
	// configmap and service resources.
	ds, err := h.ManifestFactory.DNSDaemonSet(dns, "", "")
	if err != nil {
		return fmt.Errorf("failed to build daemonset for deletion, ClusterDNS: %s, %v", dns.Name, err)
	}
	if err := sdk.Delete(ds); err != nil && !errors.IsNotFound(err) {
		return err
	}
	logrus.Infof("deleted dns daemonset %s", ds.Name)
	return nil
}

// syncClusterDNSStatus updates the status for a given clusterdns.
func syncClusterDNSStatus(dns *operatorv1.DNS, clusterIP, clusterDomain string) error {
	if err := sdk.Get(dns); err != nil {
		return fmt.Errorf("failed to get latest dns object %s: %v", dns.Name, err)
	}
	dns.Status = operatorv1.DNSStatus{
		ClusterIP:     clusterIP,
		ClusterDomain: clusterDomain,
	}

	unstructObj, err := k8sutil.UnstructuredFromRuntimeObject(dns)
	if err != nil {
		return fmt.Errorf("failed to convert unstructured obj from runtime obj: %v", err)
	}

	client, _, err := k8sclient.GetResourceClient(dns.APIVersion, dns.Kind, dns.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get resource client for dns %s: %v", dns.Name, err)
	}

	if _, err := client.UpdateStatus(unstructObj); err != nil {
		return fmt.Errorf("failed to update status for dns %s: %v", dns.Name, err)
	}
	return nil
}

// getClusterIPFromNetworkConfig will return 10th IP from the service CIDR range
// defined in the cluster network config.
func getClusterIPFromNetworkConfig() (string, error) {
	networkConfig := &configv1.Network{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Network",
			APIVersion: "config.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
	if err := sdk.Get(networkConfig); err != nil {
		return "", fmt.Errorf("failed to get network 'cluster': %v", err)
	}

	if len(networkConfig.Status.ServiceNetwork) == 0 {
		return "", fmt.Errorf("no service networks found in cluster network config")
	}
	_, serviceCIDR, err := net.ParseCIDR(networkConfig.Status.ServiceNetwork[0])
	if err != nil {
		return "", fmt.Errorf("invalid serviceCIDR %s: %v", networkConfig.Status.ServiceNetwork[0], err)
	}

	dnsClusterIP, err := cidr.Host(serviceCIDR, 10)
	if err != nil {
		return "", fmt.Errorf("invalid serviceCIDR %v: %v", serviceCIDR, err)
	}
	return dnsClusterIP.String(), nil
}
