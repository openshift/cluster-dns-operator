package controller

import (
	operatorv1 "github.com/openshift/api/operator/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// controllerDaemonSetLabel identifies a daemonset as a dns
	// daemonset, and the value is the name of the owning dns.
	controllerDaemonSetLabel = "dns.operator.openshift.io/daemonset-dns"

	// MetricsServingCertAnnotation is the annotation needed to generate
	// the certificates for secure DNS metrics.
	MetricsServingCertAnnotation = "service.beta.openshift.io/serving-cert-secret-name"
)

// DNSDaemonSetName returns the namespaced name for the dns daemonset.
func DNSDaemonSetName(dns *operatorv1.DNS) types.NamespacedName {
	return types.NamespacedName{
		Namespace: "openshift-dns",
		Name:      "dns-" + dns.Name,
	}
}

func DNSDaemonSetLabel(dns *operatorv1.DNS) string {
	return dns.Name
}

func DNSDaemonSetPodSelector(dns *operatorv1.DNS) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			controllerDaemonSetLabel: DNSDaemonSetLabel(dns),
		},
	}
}

func DNSServiceName(dns *operatorv1.DNS) types.NamespacedName {
	return types.NamespacedName{
		Namespace: "openshift-dns",
		Name:      "dns-" + dns.Name,
	}
}

func DNSConfigMapName(dns *operatorv1.DNS) types.NamespacedName {
	return types.NamespacedName{
		Namespace: "openshift-dns",
		Name:      "dns-" + dns.Name,
	}
}

func DNSServiceMonitorName(dns *operatorv1.DNS) types.NamespacedName {
	return types.NamespacedName{
		Namespace: "openshift-dns",
		Name:      "dns-" + dns.Name,
	}
}

func DNSMetricsSecretName(dns *operatorv1.DNS) string {
	return "dns-" + dns.Name + "-metrics-tls"
}
