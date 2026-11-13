package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	opclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestEnsureDNSConfigMapWithInvalidResponseRules(t *testing.T) {
	unsafe := operatorv1.DNSResponseRule{
		Name:  "cycle",
		Query: operatorv1.DNSResponseQuery{Name: "*.example.com"},
		Action: operatorv1.DNSResponseAction{
			Type:  operatorv1.DNSResponseActionTypeCNAME,
			CNAME: operatorv1.DNSResponseActionCNAME{Target: "sink.example.com"},
		},
	}
	safe := operatorv1.DNSResponseRule{
		Name:  "safe",
		Query: operatorv1.DNSResponseQuery{Name: "safe.other.com"},
		Action: operatorv1.DNSResponseAction{
			Type: operatorv1.DNSResponseActionTypeA,
			A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}},
		},
	}
	for _, tc := range []struct {
		name         string
		existing     bool
		wantRendered bool
	}{
		{name: "existing configmap is retained", existing: true},
		{name: "initial configmap omits invalid rule", wantRendered: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dns := &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
				Spec:       operatorv1.DNSSpec{ResponseRules: []operatorv1.DNSResponseRule{unsafe, safe}},
			}
			builder := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(
				&configv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: configv1.IngressSpec{Domain: "apps.cluster.example.com"}},
				&configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
				dns,
			)
			if tc.existing {
				builder = builder.WithObjects(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: DNSConfigMapName(dns).Name, Namespace: DNSConfigMapName(dns).Namespace},
					Data:       map[string]string{"Corefile": "last-known-good"},
				})
			}
			r := &reconciler{client: builder.Build()}
			have, cm, err := r.ensureDNSConfigMap(dns, "cluster.local", nil)
			if err == nil || !strings.Contains(err.Error(), "cycle") {
				t.Fatalf("expected cycle validation error, got %v", err)
			}
			if !have || cm == nil {
				t.Fatalf("expected a retained or fallback configmap, got have=%v cm=%v", have, cm)
			}
			corefile := cm.Data["Corefile"]
			if tc.existing && corefile != "last-known-good" {
				t.Fatalf("existing Corefile changed: %s", corefile)
			}
			if tc.wantRendered {
				if !strings.Contains(corefile, "203.0.113.10") || strings.Contains(corefile, "sink") {
					t.Fatalf("fallback Corefile contains wrong rules: %s", corefile)
				}
				stored := &corev1.ConfigMap{}
				if err := r.client.Get(context.Background(), DNSConfigMapName(dns), stored); err != nil {
					t.Fatalf("fallback configmap was not created: %v", err)
				}
			}
		})
	}
}

func TestResponseRuleValidationCondition(t *testing.T) {
	dns := &operatorv1.DNS{ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController}, Spec: operatorv1.DNSSpec{
		ResponseRules: []operatorv1.DNSResponseRule{{Name: "cycle"}},
	}}
	client := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(dns).
		WithStatusSubresource(&operatorv1.DNS{}).Build()
	r := &reconciler{client: client, responseRulesEnabled: true}
	result := &reconcile.Result{}
	ruleError := &responseRuleValidationError{message: "response rule \"cycle\": CNAME target forms a cycle"}
	_ = r.syncDNSStatusWithResponseRules(dns, "", "cluster.local", false, nil, false, nil, 0*time.Second, result, ruleError, false)
	stored := &operatorv1.DNS{}
	if err := client.Get(context.Background(), DefaultDNSNamespaceName(), stored); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, condition := range stored.Status.Conditions {
		if condition.Type == "ResponseRulesValid" && condition.Status == operatorv1.ConditionFalse && strings.Contains(condition.Message, "cycle") {
			found = true
		}
	}
	if !found {
		t.Fatalf("response rule validation error missing from DNS status: %+v", stored.Status.Conditions)
	}
	_ = r.syncDNSStatusWithResponseRules(stored, "", "cluster.local", false, nil, false, nil, 0*time.Second, result, nil, true)
	if err := client.Get(context.Background(), DefaultDNSNamespaceName(), stored); err != nil {
		t.Fatal(err)
	}
	for _, condition := range stored.Status.Conditions {
		if condition.Type == "ResponseRulesValid" {
			if condition.Status != operatorv1.ConditionTrue || condition.Reason != "ResponseRulesApplied" {
				t.Fatalf("response rule validation condition did not recover: %+v", condition)
			}
			return
		}
	}
	t.Fatalf("healthy response rule condition missing from DNS status: %+v", stored.Status.Conditions)
}

func TestResponseRulesAreIgnoredWhenFeatureGateDisabled(t *testing.T) {
	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
		Spec: operatorv1.DNSSpec{ResponseRules: []operatorv1.DNSResponseRule{{
			Name:  "host",
			Query: operatorv1.DNSResponseQuery{Name: "host.example.com"},
			Action: operatorv1.DNSResponseAction{
				Type: operatorv1.DNSResponseActionTypeA,
				A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}},
			},
		}}, Template: operatorv1.DNSTemplate{
			Zones: []operatorv1.Zone{"."}, QueryClass: operatorv1.QueryClassIN,
			QueryType: operatorv1.QueryTypeAAAA, Action: operatorv1.DNSTemplateAction{Rcode: operatorv1.ResponseCodeNOERROR},
		}},
	}
	client := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(dns).Build()
	r := &reconciler{client: client, responseWebhookManaged: true}
	have, cm, err := r.ensureDNSConfigMap(dns, "cluster.local", nil)
	if err != nil || !have || cm == nil {
		t.Fatalf("expected gate-disabled configmap without validation, got have=%v err=%v", have, err)
	}
	if strings.Contains(cm.Data["Corefile"], "203.0.113.10") || strings.Contains(cm.Data["Corefile"], "template IN AAAA") {
		t.Fatalf("gate-disabled Corefile contains a gated template or response rule: %s", cm.Data["Corefile"])
	}
}

func TestInvalidRenderedRuleFallsBackToDefaultCorefile(t *testing.T) {
	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
		Spec: operatorv1.DNSSpec{ResponseRules: []operatorv1.DNSResponseRule{{
			Name:  "malformed",
			Query: operatorv1.DNSResponseQuery{Name: "bad.example.com"},
			Action: operatorv1.DNSResponseAction{
				Type: operatorv1.DNSResponseActionTypeA,
				A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10\"\n        invalid"}},
			},
		}, {
			Name:  "safe",
			Query: operatorv1.DNSResponseQuery{Name: "safe.other.com"},
			Action: operatorv1.DNSResponseAction{
				Type: operatorv1.DNSResponseActionTypeA,
				A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.11"}},
			},
		}}},
	}
	client := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(
		dns,
		&configv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: configv1.IngressSpec{Domain: "apps.cluster.example.com"}},
		&configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}},
	).Build()
	r := &reconciler{client: client}
	have, cm, err := r.ensureDNSConfigMap(dns, "cluster.local", nil)
	if err == nil || !have || cm == nil {
		t.Fatalf("expected fallback and validation error, got have=%v err=%v", have, err)
	}
	if strings.Contains(cm.Data["Corefile"], "203.0.113.10") {
		t.Fatalf("invalid rendered rule was applied: %s", cm.Data["Corefile"])
	}
	if !strings.Contains(cm.Data["Corefile"], "203.0.113.11") {
		t.Fatalf("valid rule was lost from fallback: %s", cm.Data["Corefile"])
	}
}

func TestMissingValidationContextFallsBackOnInitialConfigMap(t *testing.T) {
	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
		Spec: operatorv1.DNSSpec{ResponseRules: []operatorv1.DNSResponseRule{{
			Name:  "host",
			Query: operatorv1.DNSResponseQuery{Name: "host.example.com"},
			Action: operatorv1.DNSResponseAction{
				Type: operatorv1.DNSResponseActionTypeA,
				A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}},
			},
		}}},
	}
	client := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(dns).Build()
	r := &reconciler{client: client}
	have, cm, err := r.ensureDNSConfigMap(dns, "cluster.local", nil)
	if !have || cm == nil || err == nil || !strings.Contains(err.Error(), "ingress") {
		t.Fatalf("expected default Corefile with validation error, got have=%v cm=%v err=%v", have, cm, err)
	}
	if strings.Contains(cm.Data["Corefile"], "203.0.113.10") {
		t.Fatalf("rule was rendered without cluster validation context: %s", cm.Data["Corefile"])
	}
}

func responseRuleTestClient(dns *operatorv1.DNS, existingCorefile string) *reconciler {
	builder := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(
		&configv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: configv1.IngressSpec{Domain: "apps.cluster.example.com"}},
		&configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Status: configv1.InfrastructureStatus{APIServerURL: "https://api.cluster.example.com:6443"}},
		dns,
	)
	if existingCorefile != "" {
		builder = builder.WithObjects(&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: DNSConfigMapName(dns).Name, Namespace: DNSConfigMapName(dns).Namespace},
			Data:       map[string]string{"Corefile": existingCorefile},
		})
	}
	return &reconciler{client: builder.Build()}
}

func aRule(name, query, address string) operatorv1.DNSResponseRule {
	return operatorv1.DNSResponseRule{
		Name:  operatorv1.DNSResponseRuleName(name),
		Query: operatorv1.DNSResponseQuery{Name: query},
		Action: operatorv1.DNSResponseAction{
			Type: operatorv1.DNSResponseActionTypeA,
			A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{operatorv1.IPv4Address(address)}},
		},
	}
}

// TestProtectedNameConflictIsExcludedImmediately covers a rule that newly
// conflicts with a protected API or ingress name while a ConfigMap already
// exists: the operator must not keep serving the last applied Corefile, but
// exclude the rule and apply the remaining configuration, including other
// changes such as forwarding updates, in the same pass.
func TestProtectedNameConflictIsExcludedImmediately(t *testing.T) {
	for _, tc := range []struct {
		name     string
		conflict operatorv1.DNSResponseRule
	}{
		{name: "ingress domain", conflict: aRule("ingress", "console.apps.cluster.example.com", "203.0.113.66")},
		{name: "API hostname", conflict: aRule("api", "api.cluster.example.com", "203.0.113.66")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dns := &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
				Spec: operatorv1.DNSSpec{
					ResponseRules: []operatorv1.DNSResponseRule{tc.conflict, aRule("safe", "safe.other.com", "203.0.113.10")},
					Servers: []operatorv1.Server{{
						Name: "fwd", Zones: []string{"forwarded.example.net"},
						ForwardPlugin: operatorv1.ForwardPlugin{Upstreams: []string{"192.0.2.53"}},
					}},
				},
			}
			r := responseRuleTestClient(dns, "last-known-good serving 203.0.113.66")
			have, cm, err := r.ensureDNSConfigMap(dns, "cluster.local", nil)
			if err == nil || !strings.Contains(err.Error(), string(tc.conflict.Name)) {
				t.Fatalf("expected an error naming the excluded rule, got %v", err)
			}
			if !have || cm == nil {
				t.Fatalf("expected an updated configmap, got have=%v cm=%v", have, cm)
			}
			corefile := cm.Data["Corefile"]
			if strings.Contains(corefile, "203.0.113.66") {
				t.Fatalf("protected-name override is still served: %s", corefile)
			}
			if !strings.Contains(corefile, "203.0.113.10") {
				t.Fatalf("valid rule was not applied: %s", corefile)
			}
			if !strings.Contains(corefile, "192.0.2.53") {
				t.Fatalf("forwarding change was not applied in the same pass: %s", corefile)
			}
		})
	}
}

// TestUnattributableValidationError injects a validator error that does
// not map back to any single rule's rendered stanza.
func TestUnattributableValidationError(t *testing.T) {
	saved := validateAllResponseRuleStanzas
	validateAllResponseRuleStanzas = func(stanzas string) error {
		if stanzas == "" {
			return nil
		}
		return errors.New("injected validator error")
	}
	defer func() { validateAllResponseRuleStanzas = saved }()

	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
		Spec:       operatorv1.DNSSpec{ResponseRules: []operatorv1.DNSResponseRule{aRule("one", "one.other.com", "203.0.113.10")}},
	}

	t.Run("initial configmap omits all rules", func(t *testing.T) {
		r := responseRuleTestClient(dns, "")
		have, cm, err := r.ensureDNSConfigMap(dns, "cluster.local", nil)
		if err == nil || !strings.Contains(err.Error(), "injected validator error") {
			t.Fatalf("expected the injected error to be reported, got %v", err)
		}
		if !have || cm == nil || strings.Contains(cm.Data["Corefile"], "203.0.113.10") || strings.Contains(cm.Data["Corefile"], "template") {
			t.Fatalf("expected a Corefile without response rules, got have=%v cm=%v", have, cm)
		}
	})
	t.Run("existing configmap is retained", func(t *testing.T) {
		r := responseRuleTestClient(dns, "last-known-good")
		_, cm, err := r.ensureDNSConfigMap(dns, "cluster.local", nil)
		if err == nil || cm == nil || cm.Data["Corefile"] != "last-known-good" {
			t.Fatalf("expected the last applied Corefile to be kept, got cm=%v err=%v", cm, err)
		}
	})
}

// TestResponseRulesNotServedOnExternalTopology covers the interim guard:
// on a hosted control plane, rules are not rendered and the condition
// reason states that the topology is unsupported.
func TestResponseRulesNotServedOnExternalTopology(t *testing.T) {
	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
		Spec:       operatorv1.DNSSpec{ResponseRules: []operatorv1.DNSResponseRule{aRule("one", "one.other.com", "203.0.113.10")}},
	}
	client := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(
		dns,
		&configv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: configv1.IngressSpec{Domain: "apps.cluster.example.com"}},
		&configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Status: configv1.InfrastructureStatus{ControlPlaneTopology: configv1.ExternalTopologyMode}},
	).Build()
	r := &reconciler{client: client}
	have, cm, err := r.ensureDNSConfigMap(dns, "cluster.local", nil)
	ruleErr, ok := err.(*responseRuleValidationError)
	if !ok || ruleErr.reason != responseRulesUnsupportedReason {
		t.Fatalf("expected an unsupported-topology error, got %v", err)
	}
	if !have || cm == nil || strings.Contains(cm.Data["Corefile"], "203.0.113.10") {
		t.Fatalf("rules must not be served on an External topology, got have=%v cm=%v", have, cm)
	}
}
