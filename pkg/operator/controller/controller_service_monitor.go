package controller

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func (r *reconciler) ensureServiceMonitor(dns *operatorv1.DNS, svc *corev1.Service, daemonsetRef metav1.OwnerReference) (*unstructured.Unstructured, error) {
	desired := desiredServiceMonitor(dns, svc, daemonsetRef)

	current, err := r.currentServiceMonitor(dns)
	if err != nil {
		return nil, err
	}

	switch {
	case desired != nil && current == nil:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return nil, fmt.Errorf("failed to create servicemonitor %s/%s: %v", desired.GetNamespace(), desired.GetName(), err)
		}
		logrus.Infof("created servicemonitor %s/%s", desired.GetNamespace(), desired.GetName())
	case desired != nil && current != nil:
		if err := r.updateDNSServiceMonitor(current, desired); err != nil {
			return nil, err
		}
	}
	return r.currentServiceMonitor(dns)
}

func desiredServiceMonitor(dns *operatorv1.DNS, svc *corev1.Service, daemonsetRef metav1.OwnerReference) *unstructured.Unstructured {
	name := DNSServiceMonitorName(dns)
	sm := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"namespace": name.Namespace,
				"name":      name.Name,
			},
			"spec": map[string]interface{}{
				"namespaceSelector": map[string]interface{}{
					"matchNames": []interface{}{
						"openshift-dns",
					},
				},
				"selector": map[string]interface{}{},
				"endpoints": []interface{}{
					map[string]interface{}{
						"bearerTokenFile": "/var/run/secrets/kubernetes.io/serviceaccount/token",
						"interval":        "30s",
						"port":            "metrics",
						"scheme":          "https",
						"path":            "/metrics",
						"tlsConfig": map[string]interface{}{
							"caFile":     "/etc/prometheus/configmaps/serving-certs-ca-bundle/service-ca.crt",
							"serverName": fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace),
						},
					},
				},
			},
		},
	}
	sm.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Kind:    "ServiceMonitor",
		Version: "v1",
	})
	sm.SetOwnerReferences([]metav1.OwnerReference{daemonsetRef})
	return sm
}

func (r *reconciler) currentServiceMonitor(dns *operatorv1.DNS) (*unstructured.Unstructured, error) {
	sm := &unstructured.Unstructured{}
	sm.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Kind:    "ServiceMonitor",
		Version: "v1",
	})
	if err := r.client.Get(context.TODO(), DNSServiceMonitorName(dns), sm); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return sm, nil
}

func (r *reconciler) updateDNSServiceMonitor(current, desired *unstructured.Unstructured) error {
	changed, updated := serviceMonitorChanged(current, desired)
	if !changed {
		return nil
	}

	if err := r.client.Update(context.TODO(), updated); err != nil {
		return fmt.Errorf("failed to update dns servicemonitor %s/%s: %v", updated.GetNamespace(), updated.GetName(), err)
	}
	logrus.Infof("updated dns servicemonitor: %s/%s", updated.GetNamespace(), updated.GetName())
	return nil
}

func serviceMonitorChanged(current, expected *unstructured.Unstructured) (bool, *unstructured.Unstructured) {
	if cmp.Equal(current.Object["spec"], expected.Object["spec"], cmpopts.EquateEmpty()) {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Object["spec"] = expected.Object["spec"]

	return true, updated
}
