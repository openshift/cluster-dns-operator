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
