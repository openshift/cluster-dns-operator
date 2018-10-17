package stub

import (
	"context"
	"fmt"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	"github.com/openshift/cluster-dns-operator/pkg/util"

	"github.com/operator-framework/operator-sdk/pkg/sdk"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewHandler() *Handler {
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

// EnsureDefaultClusterDNS ensures that a default ClusterDNS exists.
func (h *Handler) EnsureDefaultClusterDNS() error {
	cm, err := util.GetInstallerConfigMap()
	if err != nil {
		return err
	}
	cd, err := h.manifestFactory.ClusterDNSDefaultCR(cm)
	if err != nil {
		return err
	}

	changed, ncd, err := checkClusterDNS(cd)
	if err != nil {
		return err
	}
	if changed {
		err = sdk.Update(ncd)
		if err != nil {
			return fmt.Errorf("updating default cluster dns %s/%s: %v", cd.Namespace, cd.Name, err)
		}
		logrus.Infof("updated default cluster dns %s/%s", cd.Namespace, cd.Name)
	} else if ncd == nil {
		err = sdk.Create(cd)
		if err != nil {
			return fmt.Errorf("creating default cluster dns %s/%s: %v", cd.Namespace, cd.Name, err)
		}
		logrus.Infof("created default cluster dns %s/%s", cd.Namespace, cd.Name)
	}
	return nil
}

func checkClusterDNS(cd *dnsv1alpha1.ClusterDNS) (bool, *dnsv1alpha1.ClusterDNS, error) {
	oldcd := &dnsv1alpha1.ClusterDNS{
		TypeMeta: metav1.TypeMeta{
			Kind:       cd.Kind,
			APIVersion: cd.APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cd.Name,
			Namespace: cd.Namespace,
		},
	}
	err := sdk.Get(oldcd)
	if err != nil {
		if !errors.IsNotFound(err) {
			return false, nil, fmt.Errorf("failed to fetch existing default cluster dns %s/%s, %v", cd.Namespace, cd.Name, err)
		}
		return false, nil, nil
	}

	if cd.Spec.ClusterIP == nil {
		return false, nil, fmt.Errorf("invalid cluster IP for default cluster dns %s/%s", cd.Namespace, cd.Name)
	}
	if oldcd.Spec.ClusterIP == nil {
		oldcd.Spec.ClusterIP = new(string)
	}

	if *oldcd.Spec.ClusterIP != *cd.Spec.ClusterIP {
		*oldcd.Spec.ClusterIP = *cd.Spec.ClusterIP
		return true, oldcd, nil
	}
	return false, oldcd, nil
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
