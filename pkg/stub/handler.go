package stub

import (
	"context"
	"fmt"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	"github.com/operator-framework/operator-sdk/pkg/sdk"

	"k8s.io/apimachinery/pkg/api/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
)

func NewHandler() sdk.Handler {
	return &Handler{
		manifestFactory: manifests.NewFactory(),
	}
}

type Handler struct {
	manifestFactory *manifests.Factory
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	switch o := event.Object.(type) {
	case *dnsv1alpha1.ClusterDNS:
		if event.Deleted {
			return h.deleteDNS(o)
		} else {
			return h.syncDNSUpdate(o)
		}
	}
	return nil
}

func (h *Handler) deleteDNS(dns *dnsv1alpha1.ClusterDNS) error {
	var errs []error
	s, err := h.manifestFactory.DNSService(dns)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to build service for deletion, ClusterDNS: %q, %v", dns.Name, err))
	} else if err = sdk.Delete(s); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete service, ClusterDNS %q: %v", dns.Name, err))
	}

	ds, err := h.manifestFactory.DNSDaemonSet(dns)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to build daemonset for deletion, ClusterDNS: %q, %v", dns.Name, err))
	} else if err = sdk.Delete(ds); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete daemonset, ClusterDNS %q: %v", dns.Name, err))
	}

	cm, err := h.manifestFactory.DNSConfigMap(dns)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to build configmap for deletion, ClusterDNS: %q, %v", dns.Name, err))
	} else if err = sdk.Delete(cm); err != nil {
		errs = append(errs, fmt.Errorf("failed to delete configmap, ClusterDNS %q: %v", dns.Name, err))
	}
	return kerrors.NewAggregate(errs)
}

func (h *Handler) syncDNSUpdate(dns *dnsv1alpha1.ClusterDNS) error {
	ns, err := h.manifestFactory.DNSNamespace()
	if err != nil {
		return fmt.Errorf("couldn't build dns namespace: %v", err)
	}
	err = sdk.Create(ns)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns namespace: %v", err)
	}

	sa, err := h.manifestFactory.DNSServiceAccount()
	if err != nil {
		return fmt.Errorf("couldn't build dns service account: %v", err)
	}
	err = sdk.Create(sa)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns service account: %v", err)
	}

	cr, err := h.manifestFactory.DNSClusterRole()
	if err != nil {
		return fmt.Errorf("couldn't build dns cluster role: %v", err)
	}
	err = sdk.Create(cr)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns cluster role: %v", err)
	}

	crb, err := h.manifestFactory.DNSClusterRoleBinding()
	if err != nil {
		return fmt.Errorf("couldn't build dns cluster role binding: %v", err)
	}
	err = sdk.Create(crb)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns cluster role binding: %v", err)
	}

	cm, err := h.manifestFactory.DNSConfigMap(dns)
	if err != nil {
		return fmt.Errorf("couldn't build dns config map: %v", err)
	}
	err = sdk.Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns config map: %v", err)
	}

	ds, err := h.manifestFactory.DNSDaemonSet(dns)
	if err != nil {
		return fmt.Errorf("couldn't build daemonset: %v", err)
	}
	err = sdk.Create(ds)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create daemonset: %v", err)
	}

	service, err := h.manifestFactory.DNSService(dns)
	if err != nil {
		return fmt.Errorf("couldn't build service: %v", err)
	}
	err = sdk.Create(service)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create service: %v", err)
	}

	return nil
}
