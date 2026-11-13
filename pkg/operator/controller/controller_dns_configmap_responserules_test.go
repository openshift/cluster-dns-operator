package controller

import (
	"strings"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
)

func TestRenderResponseRuleStanzas(t *testing.T) {
	testCases := []struct {
		name          string
		rules         []operatorv1.DNSResponseRule
		expectErr     bool
		expectStanzas []string // substrings that must all appear, in order
	}{
		{
			name: "wildcard A rule with multiple addresses",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:  "wildcard-apps",
					Query: operatorv1.DNSResponseQuery{Name: "*.apps.customer.com"},
					Action: operatorv1.DNSResponseAction{
						Type: operatorv1.DNSResponseActionTypeA,
						A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10", "203.0.113.11"}},
					},
				},
			},
			expectStanzas: []string{
				`template IN A . {`,
				`match "^[^.]+\.apps\.customer\.com\.$"`,
				`answer "{{ .Name }} 60 IN A 203.0.113.10"`,
				`answer "{{ .Name }} 60 IN A 203.0.113.11"`,
				`fallthrough`,
			},
		},
		{
			name: "A rule renders an A stanza followed by a NODATA stanza for other types",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:       "app",
					Query:      operatorv1.DNSResponseQuery{Name: "app.customer.com"},
					TTLSeconds: 90,
					Action: operatorv1.DNSResponseAction{
						Type: operatorv1.DNSResponseActionTypeA,
						A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}},
					},
				},
			},
			expectStanzas: []string{
				`template IN A . {`,
				`match "^app\.customer\.com\.$"`,
				`answer "{{ .Name }} 90 IN A 203.0.113.10"`,
				`template ANY ANY . {`,
				`match "^app\.customer\.com\.$"`,
				`authority ". 90 IN SOA ns.invalid. admin.invalid. 1 60 60 60 90"`,
				`fallthrough`,
			},
		},
		{
			name: "CNAME rule answers every query type from a single ANY stanza",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:       "wildcard-alias",
					Query:      operatorv1.DNSResponseQuery{Name: "*.alias.customer.com"},
					TTLSeconds: 120,
					Action: operatorv1.DNSResponseAction{
						Type:  operatorv1.DNSResponseActionTypeCNAME,
						CNAME: operatorv1.DNSResponseActionCNAME{Target: "sentinel.customer.com"},
					},
				},
			},
			expectStanzas: []string{
				`template ANY ANY . {`,
				`answer "{{ .Name }} 120 IN CNAME sentinel.customer.com."`,
				`fallthrough`,
			},
		},
		{
			name: "names and targets are canonicalized",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:  "upper",
					Query: operatorv1.DNSResponseQuery{Name: "App.Customer.COM."},
					Action: operatorv1.DNSResponseAction{
						Type:  operatorv1.DNSResponseActionTypeCNAME,
						CNAME: operatorv1.DNSResponseActionCNAME{Target: "Target.Customer.com."},
					},
				},
			},
			expectStanzas: []string{
				`match "^app\.customer\.com\.$"`,
				`answer "{{ .Name }} 60 IN CNAME target.customer.com."`,
			},
		},
		{
			name: "A rule with more than 8 addresses is rejected",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:  "many",
					Query: operatorv1.DNSResponseQuery{Name: "many.customer.com"},
					Action: operatorv1.DNSResponseAction{
						Type: operatorv1.DNSResponseActionTypeA,
						A: operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{
							"203.0.113.1", "203.0.113.2", "203.0.113.3", "203.0.113.4", "203.0.113.5",
							"203.0.113.6", "203.0.113.7", "203.0.113.8", "203.0.113.9",
						}},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "exact-name NXDOMAIN rule includes fallthrough",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:  "blocked-host",
					Query: operatorv1.DNSResponseQuery{Name: "blocked.customer.com"},
					Action: operatorv1.DNSResponseAction{
						Type:     operatorv1.DNSResponseActionTypeNXDOMAIN,
						NXDOMAIN: &operatorv1.DNSResponseActionNXDOMAIN{},
					},
				},
			},
			expectStanzas: []string{
				`template ANY ANY . {`,
				`match "^blocked\.customer\.com\.$"`,
				`rcode NXDOMAIN`,
				`authority ". 60 IN SOA ns.invalid. admin.invalid. 1 60 60 60 60"`,
				`fallthrough`,
			},
		},
		{
			name: "NXDOMAIN SOA minimum uses explicit TTL",
			rules: []operatorv1.DNSResponseRule{{
				Name:       "blocked-explicit",
				Query:      operatorv1.DNSResponseQuery{Name: "blocked-explicit.customer.com"},
				TTLSeconds: 120,
				Action: operatorv1.DNSResponseAction{
					Type:     operatorv1.DNSResponseActionTypeNXDOMAIN,
					NXDOMAIN: &operatorv1.DNSResponseActionNXDOMAIN{},
				},
			}},
			expectStanzas: []string{`SOA ns.invalid. admin.invalid. 1 60 60 60 120`},
		},
		{
			name: "nxdomain rule rejected on a wildcard name",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:  "wildcard-blocked",
					Query: operatorv1.DNSResponseQuery{Name: "*.blocked.customer.com"},
					Action: operatorv1.DNSResponseAction{
						Type:     operatorv1.DNSResponseActionTypeNXDOMAIN,
						NXDOMAIN: &operatorv1.DNSResponseActionNXDOMAIN{},
					},
				},
			},
			expectErr: true,
		},
		{
			name: "A action missing its a field",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:   "missing-a",
					Query:  operatorv1.DNSResponseQuery{Name: "missing-a.customer.com"},
					Action: operatorv1.DNSResponseAction{Type: operatorv1.DNSResponseActionTypeA},
				},
			},
			expectErr: true,
		},
		{
			name: "default TTL is used when ttlSeconds is unset",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:  "default-ttl",
					Query: operatorv1.DNSResponseQuery{Name: "default-ttl.customer.com"},
					Action: operatorv1.DNSResponseAction{
						Type: operatorv1.DNSResponseActionTypeA,
						A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}},
					},
				},
			},
			expectStanzas: []string{
				`answer "{{ .Name }} 60 IN A 203.0.113.10"`,
			},
		},
		{
			name:          "no rules renders no stanzas",
			rules:         nil,
			expectStanzas: nil,
		},
		{
			// Regression test for a real finding: verified empirically against
			// CoreDNS 1.11.3 that when two `template` stanzas in the same zone
			// both match a query, CoreDNS answers from whichever is declared
			// first in the Corefile and never evaluates the second, even with
			// fallthrough set on both. This rule list deliberately lists the
			// wildcard BEFORE the exact rule -- renderResponseRuleStanzas must
			// still emit the exact rule's stanza first, or "app.example.com"
			// would incorrectly get the wildcard's answer instead of its own.
			name: "exact rule is rendered before an overlapping wildcard rule, regardless of input order",
			rules: []operatorv1.DNSResponseRule{
				{
					Name:  "wildcard-example",
					Query: operatorv1.DNSResponseQuery{Name: "*.example.com"},
					Action: operatorv1.DNSResponseAction{
						Type: operatorv1.DNSResponseActionTypeA,
						A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.50"}},
					},
				},
				{
					Name:  "exact-app",
					Query: operatorv1.DNSResponseQuery{Name: "app.example.com"},
					Action: operatorv1.DNSResponseAction{
						Type:  operatorv1.DNSResponseActionTypeCNAME,
						CNAME: operatorv1.DNSResponseActionCNAME{Target: "sentinel.example.com"},
					},
				},
			},
			expectStanzas: []string{
				`match "^app\.example\.com\.$"`,
				`match "^[^.]+\.example\.com\.$"`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderResponseRuleStanzas(tc.rules)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected an error, got none; rendered:\n%s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			offset := 0
			for _, substr := range tc.expectStanzas {
				idx := strings.Index(got[offset:], substr)
				if idx == -1 {
					t.Fatalf("expected rendered Corefile to contain %q after the previous expected substring; got:\n%s", substr, got)
				}
				offset += idx + len(substr)
			}
			if tc.expectStanzas == nil && got != "" {
				t.Fatalf("expected no rendered output, got:\n%s", got)
			}
		})
	}
}

func TestRenderCNAMERuleUsesOneStanza(t *testing.T) {
	rules := []operatorv1.DNSResponseRule{
		{
			Name:       "wildcard-alias",
			Query:      operatorv1.DNSResponseQuery{Name: "*.alias.customer.com"},
			TTLSeconds: 120,
			Action: operatorv1.DNSResponseAction{
				Type:  operatorv1.DNSResponseActionTypeCNAME,
				CNAME: operatorv1.DNSResponseActionCNAME{Target: "sentinel.customer.com"},
			},
		},
	}
	got, err := renderResponseRuleStanzas(rules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n := strings.Count(got, "template "); n != 1 {
		t.Errorf("expected exactly one template stanza for a cname rule, got %d; rendered:\n%s", n, got)
	}
}

// TestRenderedResponseRuleStanzasPassCoreDNSSetup runs CoreDNS's template
// setup and the rendered-answer checks against every action type.
func TestRenderedResponseRuleStanzasPassCoreDNSSetup(t *testing.T) {
	eight := []operatorv1.IPv4Address{"203.0.113.1", "203.0.113.2", "203.0.113.3", "203.0.113.4", "203.0.113.5", "203.0.113.6", "203.0.113.7", "203.0.113.8"}
	longSuffix := strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 57) + ".com"
	rules := []operatorv1.DNSResponseRule{
		{Name: "a", Query: operatorv1.DNSResponseQuery{Name: "*." + longSuffix}, TTLSeconds: 3600,
			Action: operatorv1.DNSResponseAction{Type: operatorv1.DNSResponseActionTypeA, A: operatorv1.DNSResponseActionA{Addresses: eight}}},
		{Name: "cname", Query: operatorv1.DNSResponseQuery{Name: "*.alias.customer.com"},
			Action: operatorv1.DNSResponseAction{Type: operatorv1.DNSResponseActionTypeCNAME, CNAME: operatorv1.DNSResponseActionCNAME{Target: longSuffix}}},
		{Name: "nx", Query: operatorv1.DNSResponseQuery{Name: "blocked.customer.com"},
			Action: operatorv1.DNSResponseAction{Type: operatorv1.DNSResponseActionTypeNXDOMAIN, NXDOMAIN: &operatorv1.DNSResponseActionNXDOMAIN{}}},
	}
	for _, rule := range rules {
		if err := validateResponseRule(rule); err != nil {
			t.Errorf("rule %q: %v", rule.Name, err)
		}
	}
}

func TestRepresentativeQueryNames(t *testing.T) {
	if got := representativeQueryNames("app.example.com"); len(got) != 1 || got[0] != "app.example.com." {
		t.Fatalf("unexpected exact names %v", got)
	}
	got := representativeQueryNames("*.example.com")
	if len(got) != 2 || got[0] != "a.example.com." || got[1] != strings.Repeat("a", 63)+".example.com." {
		t.Fatalf("unexpected wildcard names %v", got)
	}
	suffix := strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 63) + ".com"
	for _, name := range representativeQueryNames("*." + suffix) {
		if len(name) > 254 {
			t.Fatalf("representative name %q is longer than 253 octets", name)
		}
	}
}

func TestResponseRuleNameRegex(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "exact name",
			input:    "blocked.customer.com",
			expected: `^blocked\.customer\.com\.$`,
		},
		{
			name:     "single-label wildcard",
			input:    "*.apps.customer.com",
			expected: `^[^.]+\.apps\.customer\.com\.$`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := responseRuleNameRegex(tc.input); got != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
