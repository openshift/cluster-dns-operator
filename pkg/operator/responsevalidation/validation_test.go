package responsevalidation

import (
	"fmt"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
)

func cname(name, query, target string) operatorv1.DNSResponseRule {
	return operatorv1.DNSResponseRule{
		Name:  operatorv1.DNSResponseRuleName(name),
		Query: operatorv1.DNSResponseQuery{Name: query},
		Action: operatorv1.DNSResponseAction{
			Type:  operatorv1.DNSResponseActionTypeCNAME,
			CNAME: operatorv1.DNSResponseActionCNAME{Target: target},
		},
	}
}

func address(name, query string) operatorv1.DNSResponseRule {
	return operatorv1.DNSResponseRule{
		Name:  operatorv1.DNSResponseRuleName(name),
		Query: operatorv1.DNSResponseQuery{Name: query},
		Action: operatorv1.DNSResponseAction{
			Type: operatorv1.DNSResponseActionTypeA,
			A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}},
		},
	}
}

func addresses(name, query string, count int) operatorv1.DNSResponseRule {
	rule := address(name, query)
	rule.Action.A.Addresses = nil
	for i := 1; i <= count; i++ {
		rule.Action.A.Addresses = append(rule.Action.A.Addresses, operatorv1.IPv4Address(fmt.Sprintf("203.0.113.%d", i)))
	}
	return rule
}

func TestValidate(t *testing.T) {
	ingress := &configv1.Ingress{Spec: configv1.IngressSpec{Domain: "apps.cluster.example.com"}}
	infrastructure := &configv1.Infrastructure{Status: configv1.InfrastructureStatus{
		APIServerURL:         "https://api.cluster.example.com:6443",
		APIServerInternalURL: "https://api-int.cluster.example.com:6443",
	}}
	tests := []struct {
		name      string
		rules     []operatorv1.DNSResponseRule
		servers   []operatorv1.Server
		wantError string
	}{
		{name: "unmatched target terminates", rules: []operatorv1.DNSResponseRule{cname("alias", "*.example.com", "target.other.com")}},
		{name: "wildcard self reference", rules: []operatorv1.DNSResponseRule{cname("alias", "*.example.com", "sink.example.com")}, wantError: "forms a cycle"},
		{name: "exact A override terminates a wildcard self match", rules: []operatorv1.DNSResponseRule{
			cname("alias", "*.example.com", "sink.example.com"),
			address("sink", "sink.example.com"),
		}},
		{name: "exact CNAME override terminates", rules: []operatorv1.DNSResponseRule{
			cname("alias", "*.example.com", "sink.example.com"),
			cname("sink", "sink.example.com", "target.other.com"),
		}},
		{name: "two exact CNAMEs form a cycle", rules: []operatorv1.DNSResponseRule{
			cname("one", "one.example.com", "two.example.com"),
			cname("two", "two.example.com", "one.example.com"),
		}, wantError: "forms a cycle"},
		{name: "duplicate query", rules: []operatorv1.DNSResponseRule{
			address("one", "same.example.com"),
			address("two", "same.example.com"),
		}, wantError: "duplicates rule"},
		{name: "exact A under a wildcard CNAME is allowed", rules: []operatorv1.DNSResponseRule{
			address("exact", "app.example.com"),
			cname("wildcard", "*.example.com", "other.example.net"),
		}},
		{name: "duplicate query after canonicalization", rules: []operatorv1.DNSResponseRule{
			address("one", "Same.Example.com."),
			cname("two", "same.example.com", "other.example.net"),
		}, wantError: "duplicates rule"},
		{name: "cycle detected through a non-canonical target", rules: []operatorv1.DNSResponseRule{
			cname("one", "one.example.com", "TWO.example.com."),
			cname("two", "two.example.com", "one.example.com"),
		}, wantError: "forms a cycle"},
		{name: "more than eight addresses", rules: []operatorv1.DNSResponseRule{
			addresses("many", "many.example.com", 9),
		}, wantError: "at most 8 are allowed"},
		{name: "eight addresses", rules: []operatorv1.DNSResponseRule{
			addresses("eight", "eight.example.com", 8),
		}},
		{name: "forwarding zone covers exact query", rules: []operatorv1.DNSResponseRule{
			address("host", "host.example.com"),
		}, servers: []operatorv1.Server{{Name: "example", Zones: []string{"example.com"}}}, wantError: "overlaps forwarding zone"},
		{name: "forwarding zone intersects wildcard", rules: []operatorv1.DNSResponseRule{
			address("wildcard", "*.example.com"),
		}, servers: []operatorv1.Server{{Name: "example", Zones: []string{"host.example.com"}}}, wantError: "overlaps forwarding zone"},
		{name: "protected ingress domain", rules: []operatorv1.DNSResponseRule{
			address("apps", "*.apps.cluster.example.com"),
		}, wantError: "overlaps protected zone"},
		{name: "protected external API hostname", rules: []operatorv1.DNSResponseRule{
			address("api", "api.cluster.example.com"),
		}, wantError: "covers protected API hostname"},
		{name: "protected internal API hostname via wildcard", rules: []operatorv1.DNSResponseRule{
			address("api", "*.cluster.example.com"),
		}, wantError: "covers protected API hostname"},
		{name: "fixed cluster domain", rules: []operatorv1.DNSResponseRule{
			address("cluster", "svc.cluster.local"),
		}, wantError: "overlaps protected zone"},
		{name: "deeper server zone does not intersect single-label wildcard", rules: []operatorv1.DNSResponseRule{
			address("wildcard", "*.example.com"),
		}, servers: []operatorv1.Server{{Name: "deep", Zones: []string{"deep.host.example.com"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dns := &operatorv1.DNS{Spec: operatorv1.DNSSpec{ResponseRules: tt.rules, Servers: tt.servers}}
			violations, err := Validate(dns, ingress, infrastructure)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantError == "" && len(violations) != 0 {
				t.Fatalf("unexpected violations: %v", violations)
			}
			if tt.wantError != "" {
				found := false
				for _, violation := range violations {
					found = found || strings.Contains(violation.Reason, tt.wantError)
				}
				if !found {
					t.Fatalf("wanted %q, got %v", tt.wantError, violations)
				}
			}
		})
	}
}

func TestValidateViolationKinds(t *testing.T) {
	ingress := &configv1.Ingress{Spec: configv1.IngressSpec{Domain: "apps.cluster.example.com."}}
	infrastructure := &configv1.Infrastructure{Status: configv1.InfrastructureStatus{
		APIServerURL: "https://API.cluster.example.com:6443",
	}}
	dns := &operatorv1.DNS{Spec: operatorv1.DNSSpec{
		ResponseRules: []operatorv1.DNSResponseRule{
			address("ingress", "console.apps.cluster.example.com"),
			address("api", "api.cluster.example.com"),
			address("local", "svc.cluster.local"),
			address("fine", "app.customer.com"),
		},
		Servers: []operatorv1.Server{{Name: "fwd", Zones: []string{"customer.net"}}},
	}}
	violations, err := Validate(dns, ingress, infrastructure)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]ViolationKind{}
	for _, v := range violations {
		kinds[v.Rule] = v.Kind
	}
	want := map[string]ViolationKind{"ingress": ViolationProtected, "api": ViolationProtected, "local": ViolationInvalid}
	if len(kinds) != len(want) {
		t.Fatalf("unexpected violations: %v", violations)
	}
	for rule, kind := range want {
		if kinds[rule] != kind {
			t.Errorf("rule %q: expected kind %q, got %q", rule, kind, kinds[rule])
		}
	}
}

func TestCanonicalize(t *testing.T) {
	for in, want := range map[string]string{"Foo.Example.COM.": "foo.example.com", "*.example.com": "*.example.com", "a.b": "a.b"} {
		if got := Canonicalize(in); got != want {
			t.Errorf("Canonicalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateMissingClusterContext(t *testing.T) {
	dns := &operatorv1.DNS{Spec: operatorv1.DNSSpec{ResponseRules: []operatorv1.DNSResponseRule{address("test", "test.example.com")}}}
	if _, err := Validate(dns, nil, nil); err == nil {
		t.Fatal("expected missing cluster configuration to fail closed")
	}
	if _, err := Validate(dns, &configv1.Ingress{}, &configv1.Infrastructure{}); err == nil {
		t.Fatal("expected missing ingress domain to fail closed")
	}
	dns.Spec.ResponseRules = nil
	if _, err := Validate(dns, nil, nil); err != nil {
		t.Fatalf("empty rules should not require cluster configuration: %v", err)
	}
}

func TestAPIHostnames(t *testing.T) {
	names, err := apiHostnames("https://[2001:db8::1]:6443", "https://api.example.com:6443")
	if err != nil || len(names) != 1 || names[0] != "api.example.com" {
		t.Fatalf("unexpected hostnames %v, error %v", names, err)
	}
	if _, err := apiHostnames("not a URL"); err == nil {
		t.Fatal("expected malformed URL to fail")
	}
}
