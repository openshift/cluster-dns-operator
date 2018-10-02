package stub

import (
	"context"
	"fmt"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	"github.com/openshift/cluster-dns-operator/pkg/util"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	"github.com/operator-framework/operator-sdk/pkg/sdk"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// DNS specific configmap and service has owner reference to daemonset.
	// So deletion of daemonset will trigger garbage collection of corresponding
	// configmap and service resources.
	ds, err := h.manifestFactory.DNSDaemonSet(dns)
	if err != nil {
		return fmt.Errorf("failed to build daemonset for deletion, ClusterDNS: %q, %v", dns.Name, err)
	}
	return sdk.Delete(ds)
}

func (h *Handler) syncDNSUpdate(dns *dnsv1alpha1.ClusterDNS) error {
	if dns.Spec.ClusterIP == nil {
		// Check for default cluster ip.
		ipaddr, err := util.ClusterDNSIP(k8sclient.GetKubeClient())
		if err != nil {
			logrus.Errorf("Getting cluster dns ip: %v", err)
		} else {
			logrus.Infof("Using default cluster dns ip address %s", ipaddr)
			dns.Spec.ClusterIP = &ipaddr
		}
	}
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

	ds, err := h.manifestFactory.DNSDaemonSet(dns)
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

	cm, err := h.manifestFactory.DNSConfigMap(dns)
	if err != nil {
		return fmt.Errorf("couldn't build dns config map: %v", err)
	}
	cm.SetOwnerReferences([]metav1.OwnerReference{dsRef})
	err = sdk.Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("couldn't create dns config map: %v", err)
	}

	service, err := h.manifestFactory.DNSService(dns)
	if err != nil {
		return fmt.Errorf("couldn't build service: %v", err)
	}
	service.SetOwnerReferences([]metav1.OwnerReference{dsRef})
	err = sdk.Create(service)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create service: %v", err)
	}

	return nil
}
