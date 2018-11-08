package stub

import (
	"context"
	"fmt"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	"github.com/openshift/cluster-dns-operator/pkg/util"
	"github.com/openshift/cluster-dns-operator/pkg/util/slice"

	"github.com/operator-framework/operator-sdk/pkg/sdk"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

const (
	// ClusterDNSFinalizer is applied to all ClusterDNS resources before they are
	// considered for processing; this ensures the operator has a chance to handle
	// all states.
	// TODO: Make this generic and not tied to the "default" clusterdns.
	ClusterDNSFinalizer = "dns.openshift.io/default-cluster-dns"
)

type Handler struct {
	InstallConfig   *util.InstallConfig
	ManifestFactory *manifests.Factory
	Namespace       string
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	// TODO: This should be adding an item to a rate limited work queue, but for
	// now correctness is more important than performance.
	switch o := event.Object.(type) {
	case *dnsv1alpha1.ClusterDNS:
		logrus.Infof("reconciling for update to clusterdns %q", o.Name)
	}
	return h.reconcile()
}

// EnsureDefaultClusterDNS ensures that the default ClusterDNS exists.
// TODO: overwrite existing persisted things like clusterIP.
func (h *Handler) EnsureDefaultClusterDNS() error {
	desired, err := h.ManifestFactory.ClusterDNSDefaultCR(h.InstallConfig)
	if err != nil {
		return err
	}
	err = sdk.Create(desired)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// Reconcile performs a full reconciliation loop for DNS, including
// generalized setup and handling of all clusterdns resources in the
// operator namespace.
func (h *Handler) reconcile() error {
	// Ensure we have all the necessary scaffolding on which to place DNS
	// instances.
	err := h.ensureDNSNamespace()
	if err != nil {
		return err
	}

	// Find all clusterdnses.
	dnses := &dnsv1alpha1.ClusterDNSList{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDNS",
			APIVersion: "dns.openshift.io/v1alpha1",
		},
	}
	err = sdk.List(h.Namespace, dnses, sdk.WithListOptions(&metav1.ListOptions{}))
	if err != nil {
		return fmt.Errorf("failed to list clusterdnses: %v", err)
	}

	// Reconcile all the clusterdnses.
	errors := []error{}
	for _, dns := range dnses.Items {
		// Handle deleted dns.
		// TODO: Assert/ensure that the dns has a finalizer so we can reliably detect
		// deletion.
		if dns.DeletionTimestamp != nil {
			// Destroy any coredns instance associated with the dns.
			err := h.ensureDNSDeleted(&dns)
			if err != nil {
				errors = append(errors, fmt.Errorf("couldn't delete clusterdns %q: %v", dns.Name, err))
				continue
			}
			// Clean up the finalizer to allow the clusterdns to be deleted.
			if slice.ContainsString(dns.Finalizers, ClusterDNSFinalizer) {
				dns.Finalizers = slice.RemoveString(dns.Finalizers, ClusterDNSFinalizer)
				err = sdk.Update(&dns)
				if err != nil {
					errors = append(errors, fmt.Errorf("couldn't remove finalizer from clusterdns %q: %v", dns.Name, err))
				}
			}
			continue
		}

		// Handle active DNS.
		err := h.ensureCoreDNSForClusterDNS(&dns)
		if err != nil {
			errors = append(errors, fmt.Errorf("couldn't ensure clusterdns %q: %v", dns.Name, err))
		}
	}
	return utilerrors.NewAggregate(errors)
}

// ensureDNSNamespace ensures all the necessary scaffolding exists for
// CoreDNS generally, including a namespace and all RBAC setup.
func (h *Handler) ensureDNSNamespace() error {
	ns, err := h.ManifestFactory.DNSNamespace()
	if err != nil {
		return fmt.Errorf("couldn't build dns namespace: %v", err)
	}
	err = sdk.Create(ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns namespace: %v", err)
	}

	sa, err := h.ManifestFactory.DNSServiceAccount()
	if err != nil {
		return fmt.Errorf("couldn't build dns service account: %v", err)
	}
	err = sdk.Create(sa)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns service account: %v", err)
	}

	cr, err := h.ManifestFactory.DNSClusterRole()
	if err != nil {
		return fmt.Errorf("couldn't build dns cluster role: %v", err)
	}
	err = sdk.Create(cr)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns cluster role: %v", err)
	}

	crb, err := h.ManifestFactory.DNSClusterRoleBinding()
	if err != nil {
		return fmt.Errorf("couldn't build dns cluster role binding: %v", err)
	}
	err = sdk.Create(crb)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns cluster role binding: %v", err)
	}

	return nil
}

// ensureCoreDNSForClusterDNS ensures all necessary CoreDNS resources exist for
// a given clusterdns.
func (h *Handler) ensureCoreDNSForClusterDNS(dns *dnsv1alpha1.ClusterDNS) error {
	ds, err := h.ManifestFactory.DNSDaemonSet(dns)
	if err != nil {
		return fmt.Errorf("couldn't build daemonset: %v", err)
	}
	err = sdk.Create(ds)
	if errors.IsAlreadyExists(err) {
		if err = sdk.Get(ds); err != nil {
			return fmt.Errorf("failed to fetch daemonset %s, %v", ds.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to create daemonset: %v", err)
	}
	trueVar := true
	dsRef := metav1.OwnerReference{
		APIVersion: ds.APIVersion,
		Kind:       ds.Kind,
		Name:       ds.Name,
		UID:        ds.UID,
		Controller: &trueVar,
	}

	cm, err := h.ManifestFactory.DNSConfigMap(dns)
	if err != nil {
		return fmt.Errorf("couldn't build dns config map: %v", err)
	}
	cm.SetOwnerReferences([]metav1.OwnerReference{dsRef})
	err = sdk.Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns config map: %v", err)
	}

	service, err := h.ManifestFactory.DNSService(dns)
	if err != nil {
		return fmt.Errorf("couldn't build service: %v", err)
	}
	err = sdk.Get(service)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to fetch service %s, %v", service.Name, err)
		}
		service.SetOwnerReferences([]metav1.OwnerReference{dsRef})
		if err = sdk.Create(service); err != nil {
			return fmt.Errorf("failed to create service: %v", err)
		}
	}

	return nil
}

// ensureDNSDeleted ensures that any CoreDNS resources associated with the
// clusterdns are deleted.
func (h *Handler) ensureDNSDeleted(dns *dnsv1alpha1.ClusterDNS) error {
	// DNS specific configmap and service has owner reference to daemonset.
	// So deletion of daemonset will trigger garbage collection of corresponding
	// configmap and service resources.
	ds, err := h.ManifestFactory.DNSDaemonSet(dns)
	if err != nil {
		return fmt.Errorf("failed to build daemonset for deletion, ClusterDNS: %q, %v", dns.Name, err)
	}
	err = sdk.Delete(ds)
	if !errors.IsNotFound(err) {
		return err
	}
	return nil
}
