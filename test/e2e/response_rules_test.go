//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestResponseRulesAdmissionAndReconcile(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := cl.Get(ctx, types.NamespacedName{Name: "dnses.operator.openshift.io"}, crd); err != nil {
		if apierrors.IsNotFound(err) {
			t.Skip("DNS CRD is unavailable")
		}
		t.Fatal(err)
	}
	if !responseRulesAvailableInSchema(crd) {
		t.Skip("DNSTemplatePlugin responseRules schema is not enabled")
	}

	dns := &operatorv1.DNS{}
	if err := cl.Get(ctx, dnsName, dns); err != nil {
		t.Fatal(err)
	}
	originalRules := append([]operatorv1.DNSResponseRule(nil), dns.Spec.ResponseRules...)
	defer func() {
		if err := updateResponseRules(ctx, cl, originalRules); err != nil {
			t.Errorf("restore DNS response rules: %v", err)
		}
	}()

	query := fmt.Sprintf("rule-%d.invalid", time.Now().UnixNano())
	valid := operatorv1.DNSResponseRule{
		Name: "response-rules-e2e", Query: operatorv1.DNSResponseQuery{Name: query},
		Action: operatorv1.DNSResponseAction{Type: operatorv1.DNSResponseActionTypeA,
			A: operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}}},
	}
	configured := append(append([]operatorv1.DNSResponseRule(nil), originalRules...), valid)
	if err := updateResponseRules(ctx, cl, configured); err != nil {
		t.Fatalf("admit valid response rule: %v", err)
	}
	if err := wait.PollImmediate(2*time.Second, 2*time.Minute, func() (bool, error) {
		cm := &corev1.ConfigMap{}
		if err := cl.Get(ctx, types.NamespacedName{Name: operatorcontroller.DNSConfigMapName(dns).Name, Namespace: operatorcontroller.DNSConfigMapName(dns).Namespace}, cm); err != nil {
			return false, nil
		}
		return strings.Contains(cm.Data["Corefile"], regexp.QuoteMeta(query)) && strings.Contains(cm.Data["Corefile"], "203.0.113.10"), nil
	}); err != nil {
		t.Fatalf("valid rule did not reach the Corefile: %v", err)
	}

	invalid := operatorv1.DNSResponseRule{
		Name: "response-rules-cycle", Query: operatorv1.DNSResponseQuery{Name: "*.response-rules-e2e.invalid"},
		Action: operatorv1.DNSResponseAction{Type: operatorv1.DNSResponseActionTypeCNAME,
			CNAME: operatorv1.DNSResponseActionCNAME{Target: "sink.response-rules-e2e.invalid"}},
	}
	if err := updateResponseRules(ctx, cl, append(configured, invalid)); err == nil {
		t.Fatal("cyclic wildcard CNAME was admitted")
	} else if !apierrors.IsInvalid(err) || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected a cycle-specific admission rejection, got %v", err)
	}
	stored := &operatorv1.DNS{}
	if err := cl.Get(ctx, dnsName, stored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored.Spec.ResponseRules, configured) {
		t.Fatalf("rejected rule changed persisted DNS spec: %+v", stored.Spec.ResponseRules)
	}
}

func responseRulesAvailableInSchema(crd *apiextensionsv1.CustomResourceDefinition) bool {
	for _, version := range crd.Spec.Versions {
		if version.Name != "v1" || !version.Served || version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
			continue
		}
		spec, found := version.Schema.OpenAPIV3Schema.Properties["spec"]
		if !found {
			continue
		}
		if _, found := spec.Properties["responseRules"]; found {
			return true
		}
	}
	return false
}

func updateResponseRules(ctx context.Context, cl client.Client, rules []operatorv1.DNSResponseRule) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		dns := &operatorv1.DNS{}
		if err := cl.Get(ctx, dnsName, dns); err != nil {
			return err
		}
		dns.Spec.ResponseRules = rules
		return cl.Update(ctx, dns)
	})
}
