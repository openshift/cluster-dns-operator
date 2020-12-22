package controller

import (
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

// dnsOwnsObject verifies that the given dns owns the given object.
func dnsOwnsObject(dns *operatorv1.DNS, object metav1.Object) (bool, error) {
	key := manifests.OwningDNSLabel
	val := DNSDaemonSetLabel(dns)
	req, err := labels.NewRequirement(key, selection.Equals, []string{val})
	if err != nil {
		return false, err
	}
	sel := labels.NewSelector().Add(*req)
	return sel.Matches(labels.Set(object.GetLabels())), nil
}
