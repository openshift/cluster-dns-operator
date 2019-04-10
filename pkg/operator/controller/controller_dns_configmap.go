package controller

import (
	"context"
	"fmt"
	"strings"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	corev1 "k8s.io/api/core/v1"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ensureDNSConfigMap ensures that a configmap exists for a given DNS.
func (r *reconciler) ensureDNSConfigMap(dns *operatorv1.DNS, clusterDomain string, daemonsetRef metav1.OwnerReference) (*corev1.ConfigMap, error) {
	current, err := r.currentDNSConfigMap(dns)
	if err != nil {
		return nil, err
	}
	if current != nil {
		return current, nil
	}

	desired := desiredDNSConfigMap(dns, clusterDomain, daemonsetRef)
	if err := r.client.Create(context.TODO(), desired); err != nil {
		return nil, fmt.Errorf("failed to create dns configmap: %v", err)
	}
	logrus.Infof("created dns configmap: %s/%s", desired.Namespace, desired.Name)
	return desired, nil
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

func desiredDNSConfigMap(dns *operatorv1.DNS, clusterDomain string, daemonsetRef metav1.OwnerReference) *corev1.ConfigMap {
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
