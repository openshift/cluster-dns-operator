package controller

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	operatorv1 "github.com/openshift/api/operator/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDesiredCABundleConfigmap(t *testing.T) {
	sourceConfigmap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cabundle-config",
			Namespace: GlobalUserSpecifiedConfigNamespace,
		},
		Data: map[string]string{"caBundle": "test-bundle"},
	}

	destName := CABundleConfigMapName(sourceConfigmap.Name)

	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{
			Name: DefaultDNSController,
		},
		Spec: operatorv1.DNSSpec{},
	}

	_, cm, err := desiredCABundleConfigMap(dns, true, &sourceConfigmap, destName)
	if err != nil {
		t.Errorf("Unexpected error : %v", err)
	} else if diff := cmp.Diff(cm.Data, sourceConfigmap.Data); diff != "" {
		t.Errorf("Unexpected CA Bundle ConfigMap data;\n%s", diff)
	} else if diff := cmp.Diff(cm.OwnerReferences, []metav1.OwnerReference{dnsOwnerRef(dns)}); diff != "" {
		t.Errorf("Unexpected CA Bundle ConfigMap OwnerReference;\n%s", diff)
	}
}
