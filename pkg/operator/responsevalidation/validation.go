package responsevalidation

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
)

// MaxAddressesPerRule is the largest number of addresses an A rule may
// return, so that an answer fits a 512-byte UDP response even for a
// maximum-length name.
const MaxAddressesPerRule = 8

// ViolationKind classifies why a response rule cannot be rendered.
type ViolationKind string

const (
	// ViolationInvalid is a rule that is invalid on its own or in
	// combination with other rules or forwarding zones.
	ViolationInvalid ViolationKind = "Invalid"
	// ViolationProtected is a rule that covers the cluster's own ingress
	// domain or API hostnames. Serving such a rule is a safety failure, so
	// the operator excludes it immediately rather than keeping the last
	// applied configuration.
	ViolationProtected ViolationKind = "Protected"
)

// Violation identifies a response rule that cannot safely be rendered.
type Violation struct {
	Rule   string
	Kind   ViolationKind
	Reason string
}

func (v Violation) Error() string {
	return fmt.Sprintf("response rule %q: %s", v.Rule, v.Reason)
}

// Canonicalize returns the canonical form of a DNS name used by every
// check: lowercase with a single trailing dot removed.
func Canonicalize(name string) string {
	return strings.TrimSuffix(strings.ToLower(name), ".")
}

// Validate checks rules against the other DNS server blocks, the cluster's
// protected names, and the effective CNAME resolution graph.
func Validate(dns *operatorv1.DNS, ingress *configv1.Ingress, infrastructure *configv1.Infrastructure) ([]Violation, error) {
	if dns == nil {
		return nil, fmt.Errorf("DNS configuration is unavailable")
	}
	if len(dns.Spec.ResponseRules) == 0 {
		return nil, nil
	}
	if ingress == nil || infrastructure == nil {
		return nil, fmt.Errorf("cluster ingress or infrastructure configuration is unavailable")
	}
	ingressDomain := Canonicalize(ingress.Spec.Domain)
	if ingressDomain == "" {
		return nil, fmt.Errorf("cluster ingress domain is unavailable")
	}

	apiNames, err := apiHostnames(infrastructure.Status.APIServerURL, infrastructure.Status.APIServerInternalURL)
	if err != nil {
		return nil, err
	}

	rules := dns.Spec.ResponseRules
	queries := make([]string, len(rules))
	for i := range rules {
		queries[i] = Canonicalize(rules[i].Query.Name)
	}

	violations := make([]Violation, 0)
	invalid := func(rule *operatorv1.DNSResponseRule, format string, args ...interface{}) {
		violations = append(violations, Violation{Rule: string(rule.Name), Kind: ViolationInvalid, Reason: fmt.Sprintf(format, args...)})
	}
	protected := func(rule *operatorv1.DNSResponseRule, format string, args ...interface{}) {
		violations = append(violations, Violation{Rule: string(rule.Name), Kind: ViolationProtected, Reason: fmt.Sprintf(format, args...)})
	}

	seenQueries := make(map[string]string, len(rules))
	for i := range rules {
		rule := &rules[i]
		query := queries[i]
		if query == "" {
			invalid(rule, "query name is empty")
			continue
		}
		if previous, found := seenQueries[query]; found {
			invalid(rule, "query %q duplicates rule %q", query, previous)
		} else {
			seenQueries[query] = string(rule.Name)
		}
		if rule.Action.Type == operatorv1.DNSResponseActionTypeA && len(rule.Action.A.Addresses) > MaxAddressesPerRule {
			invalid(rule, "has %d addresses; at most %d are allowed", len(rule.Action.A.Addresses), MaxAddressesPerRule)
		}
		for _, server := range dns.Spec.Servers {
			for _, zone := range server.Zones {
				if overlapsZone(query, zone) {
					invalid(rule, "query %q overlaps forwarding zone %q of server %q", query, zone, server.Name)
				}
			}
		}
		for _, zone := range []string{"cluster.local", "in-addr.arpa", "ip6.arpa"} {
			if overlapsZone(query, zone) {
				invalid(rule, "query %q overlaps protected zone %q", query, zone)
			}
		}
		if overlapsZone(query, ingressDomain) {
			protected(rule, "query %q overlaps protected zone %q", query, ingressDomain)
		}
		for _, name := range apiNames {
			if matches(query, name) {
				protected(rule, "query %q covers protected API hostname %q", query, name)
			}
		}
		if rule.Action.Type != operatorv1.DNSResponseActionTypeCNAME || rule.Action.CNAME.Target == "" {
			continue
		}
		seen := map[int]bool{i: true}
		target := Canonicalize(rule.Action.CNAME.Target)
		for {
			next := winningRule(queries, target)
			if next < 0 || rules[next].Action.Type != operatorv1.DNSResponseActionTypeCNAME || rules[next].Action.CNAME.Target == "" {
				break
			}
			if seen[next] {
				invalid(rule, "CNAME target %q forms a cycle through rule %q", target, rules[next].Name)
				break
			}
			seen[next] = true
			target = Canonicalize(rules[next].Action.CNAME.Target)
		}
	}
	return violations, nil
}

func apiHostnames(urls ...string) ([]string, error) {
	var names []string
	for _, raw := range urls {
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
			return nil, fmt.Errorf("invalid cluster API URL %q", raw)
		}
		name := Canonicalize(parsed.Hostname())
		if net.ParseIP(name) == nil {
			names = append(names, name)
		}
	}
	return names, nil
}

// winningRule returns the index of the rule that answers name at runtime:
// the exact-name rule if there is one, otherwise the wildcard rule matching
// it. A wildcard replaces exactly one label, so at most one distinct
// wildcard pattern can match a name.
func winningRule(queries []string, name string) int {
	wildcard := -1
	for i, query := range queries {
		if query == name {
			return i
		}
		if wildcard < 0 && strings.HasPrefix(query, "*.") && matches(query, name) {
			wildcard = i
		}
	}
	return wildcard
}

func matches(query, name string) bool {
	if !strings.HasPrefix(query, "*.") {
		return query == name
	}
	suffix := query[1:]
	if !strings.HasSuffix(name, suffix) {
		return false
	}
	label := strings.TrimSuffix(name, suffix)
	return label != "" && !strings.Contains(label, ".")
}

func overlapsZone(query, zone string) bool {
	if zone == "." {
		return true
	}
	zone = Canonicalize(zone)
	if zone == "" {
		return false
	}
	if !strings.HasPrefix(query, "*.") {
		return query == zone || strings.HasSuffix(query, "."+zone)
	}
	suffix := query[2:]
	if suffix == zone || strings.HasSuffix(suffix, "."+zone) {
		return true
	}
	return matches(query, zone)
}
