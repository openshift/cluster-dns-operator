// +build e2e

package e2e

import (
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"

	"github.com/operator-framework/operator-sdk/pkg/sdk"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestOperatorAvailable(t *testing.T) {
	co := &configv1.ClusterOperator{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterOperator",
			APIVersion: "config.openshift.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshift-dns-operator",
		},
	}
	err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		if err := sdk.Get(co); err != nil {
			return false, nil
		}

		for _, cond := range co.Status.Conditions {
			if cond.Type == configv1.OperatorAvailable &&
				cond.Status == configv1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	})
	if err != nil {
		t.Errorf("did not get expected available condition: %v", err)
	}
}

func TestDefaultClusterDNSExists(t *testing.T) {
	dns := &dnsv1alpha1.ClusterDNS{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterDNS",
			APIVersion: "dns.openshift.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		if err := sdk.Get(dns); err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to get default ClusterDNS: %v", err)
	}
}
