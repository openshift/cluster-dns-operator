package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	"github.com/sirupsen/logrus"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	operatorv1 "github.com/openshift/api/operator/v1"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ensureDNSConfigMap ensures that a configmap exists for a given DNS.
func (r *reconciler) ensureDNSConfigMap(dns *operatorv1.DNS, clusterDomain string) (*corev1.ConfigMap, error) {
	current, err := r.currentDNSConfigMap(dns)
	if err != nil {
		return nil, fmt.Errorf("failed to get configmap: %v", err)
	}
	desired := desiredDNSConfigMap(dns, clusterDomain)

	switch {
	case desired != nil && current == nil:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return nil, fmt.Errorf("failed to create configmap: %v", err)
		}
		logrus.Infof("created configmap: %s", desired.Name)
	case desired != nil && current != nil:
		if needsUpdate, updated := corefileChanged(current, desired); needsUpdate {
			if err := r.client.Update(context.TODO(), updated); err != nil {
				return nil, fmt.Errorf("failed to update configmap: %v", err)
			}
			logrus.Infof("updated configmap; old: %#v, new: %#v", current, updated)
		}
	}
	return r.currentDNSConfigMap(dns)
}

func (r *reconciler) currentDNSConfigMap(dns *operatorv1.DNS) (*corev1.ConfigMap, error) {
	current := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), DNSConfigMapName(dns), current)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return current, nil
}

func desiredDNSConfigMap(dns *operatorv1.DNS, clusterDomain string) *corev1.ConfigMap {
	cm := manifests.DNSConfigMap()

	name := DNSConfigMapName(dns)
	cm.Namespace = name.Namespace
	cm.Name = name.Name
	cm.SetOwnerReferences([]metav1.OwnerReference{dnsOwnerRef(dns)})

	cm.Labels = map[string]string{
		manifests.OwningDNSLabel: DNSDaemonSetLabel(dns),
	}

	if len(clusterDomain) > 0 {
		cm.Data["Corefile"] = strings.Replace(cm.Data["Corefile"], "cluster.local", clusterDomain, -1)
	}
	return cm
}

func corefileChanged(current, expected *corev1.ConfigMap) (bool, *corev1.ConfigMap) {
	if cmp.Equal(current.Data, expected.Data, cmpopts.EquateEmpty()) {
		return false, current
	}
	updated := current.DeepCopy()
	updated.Data = expected.Data
	return true, updated
}
