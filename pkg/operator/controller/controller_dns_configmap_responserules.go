package controller

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/miekg/dns"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/operator/responsevalidation"
)

// defaultResponseRuleTTLSeconds is the default TTL, in seconds, used for a
// response rule when ttlSeconds is not set.
const defaultResponseRuleTTLSeconds = 60

// maxUDPResponseSize is the largest DNS response guaranteed to fit a UDP
// datagram without EDNS0.
const maxUDPResponseSize = 512

// responseRuleZone is the zone of the default server block the response
// rule stanzas are rendered into.
const responseRuleZone = "."

// renderedResponseRule holds the record templates a rule renders, in the
// form the CoreDNS template plugin executes them.
type renderedResponseRule struct {
	regex string
	// aAnswers are the answers of the `template IN A` stanza (a rules).
	aAnswers []string
	// anyAnswer is the answer of the `template ANY ANY` stanza (cname rules).
	anyAnswer string
	// anyRcode is the rcode of the `template ANY ANY` stanza (nxdomain rules).
	anyRcode string
	// anyAuthority is the SOA authority of the `template ANY ANY` stanza
	// (NODATA for a rules, NXDOMAIN for nxdomain rules).
	anyAuthority string
}

// renderResponseRuleStanzas renders the CoreDNS `template` plugin stanzas
// for spec.responseRules, to be included in the default server block (".").
// Every rule answers every query type for the names it matches. Exact-name
// rules are rendered before wildcard rules because CoreDNS answers from the
// first matching template stanza; fallthrough passes queries that do not
// match a stanza's regex to the next stanza.
func renderResponseRuleStanzas(rules []operatorv1.DNSResponseRule) (string, error) {
	rules = sortResponseRulesBySpecificity(rules)

	var b strings.Builder
	for _, rule := range rules {
		rendered, err := renderResponseRule(rule)
		if err != nil {
			return "", err
		}
		writeResponseRuleStanzas(&b, rendered)
	}
	return b.String(), nil
}

func renderResponseRule(rule operatorv1.DNSResponseRule) (renderedResponseRule, error) {
	ttl := rule.TTLSeconds
	if ttl == 0 {
		ttl = defaultResponseRuleTTLSeconds
	}
	query := responsevalidation.Canonicalize(rule.Query.Name)
	rendered := renderedResponseRule{regex: responseRuleNameRegex(query)}
	soa := fmt.Sprintf("%s %d IN SOA ns.invalid. admin.invalid. 1 60 60 60 %d", responseRuleZone, ttl, ttl)

	switch rule.Action.Type {
	case operatorv1.DNSResponseActionTypeA:
		if len(rule.Action.A.Addresses) == 0 {
			return rendered, fmt.Errorf("response rule %q has action type %s but no addresses", rule.Name, rule.Action.Type)
		}
		if len(rule.Action.A.Addresses) > responsevalidation.MaxAddressesPerRule {
			return rendered, fmt.Errorf("response rule %q has %d addresses; at most %d are allowed", rule.Name, len(rule.Action.A.Addresses), responsevalidation.MaxAddressesPerRule)
		}
		for _, addr := range rule.Action.A.Addresses {
			rendered.aAnswers = append(rendered.aAnswers, fmt.Sprintf("{{ .Name }} %d IN A %s", ttl, addr))
		}
		rendered.anyAuthority = soa
	case operatorv1.DNSResponseActionTypeCNAME:
		target := responsevalidation.Canonicalize(rule.Action.CNAME.Target)
		if target == "" {
			return rendered, fmt.Errorf("response rule %q has action type %s but no cname field", rule.Name, rule.Action.Type)
		}
		rendered.anyAnswer = fmt.Sprintf("{{ .Name }} %d IN CNAME %s.", ttl, target)
	case operatorv1.DNSResponseActionTypeNXDOMAIN:
		if rule.Action.NXDOMAIN == nil {
			return rendered, fmt.Errorf("response rule %q has action type %s but no nxdomain field", rule.Name, rule.Action.Type)
		}
		if isWildcardQuery(query) {
			return rendered, fmt.Errorf("response rule %q: nxdomain is only valid when query.name is an exact name, not a wildcard", rule.Name)
		}
		rendered.anyRcode = "NXDOMAIN"
		rendered.anyAuthority = soa
	default:
		return rendered, fmt.Errorf("response rule %q has unknown action type %q", rule.Name, rule.Action.Type)
	}
	return rendered, nil
}

// writeResponseRuleStanzas writes a rule's stanzas. For an a rule, the
// `template IN A` stanza comes first and answers A (and ANY) queries; the
// following `template ANY ANY` stanza answers every other type with
// NOERROR and no answer (NODATA), negatively cached for the rule's TTL
// through the SOA minimum field.
func writeResponseRuleStanzas(b *strings.Builder, r renderedResponseRule) {
	if len(r.aAnswers) != 0 {
		fmt.Fprintf(b, "    template IN A %s {\n", responseRuleZone)
		fmt.Fprintf(b, "        match \"%s\"\n", r.regex)
		for _, answer := range r.aAnswers {
			fmt.Fprintf(b, "        answer \"%s\"\n", answer)
		}
		fmt.Fprintf(b, "        fallthrough\n")
		fmt.Fprintf(b, "    }\n")
	}
	fmt.Fprintf(b, "    template ANY ANY %s {\n", responseRuleZone)
	fmt.Fprintf(b, "        match \"%s\"\n", r.regex)
	if r.anyAnswer != "" {
		fmt.Fprintf(b, "        answer \"%s\"\n", r.anyAnswer)
	}
	if r.anyRcode != "" {
		fmt.Fprintf(b, "        rcode %s\n", r.anyRcode)
	}
	if r.anyAuthority != "" {
		fmt.Fprintf(b, "        authority \"%s\"\n", r.anyAuthority)
	}
	fmt.Fprintf(b, "        fallthrough\n")
	fmt.Fprintf(b, "    }\n")
}

// validateResponseRule renders a single rule, runs CoreDNS's template
// setup against its stanzas, and checks the records its templates produce
// for representative query names.
func validateResponseRule(rule operatorv1.DNSResponseRule) error {
	stanzas, err := renderResponseRuleStanzas([]operatorv1.DNSResponseRule{rule})
	if err != nil {
		return err
	}
	if err := responsevalidation.ValidateRenderedStanzas(stanzas); err != nil {
		return err
	}
	return checkRenderedResponseRuleAnswers(rule)
}

// checkRenderedResponseRuleAnswers executes the rule's record templates
// for representative names (for a wildcard, the shortest and the longest
// allowed first label) and checks that every record parses, has the
// expected type, and that an A answer fits a 512-byte UDP response.
// Template setup only proves that templates compile; records are produced
// when a query executes them.
func checkRenderedResponseRuleAnswers(rule operatorv1.DNSResponseRule) error {
	rendered, err := renderResponseRule(rule)
	if err != nil {
		return err
	}
	for _, name := range representativeQueryNames(responsevalidation.Canonicalize(rule.Query.Name)) {
		if _, ok := dns.IsDomainName(name); !ok || len(name) > 254 {
			return fmt.Errorf("response rule %q: representative query name %q is not a valid DNS name", rule.Name, name)
		}
		msg := new(dns.Msg)
		msg.SetQuestion(name, dns.TypeA)
		msg.Compress = true
		for _, answer := range rendered.aAnswers {
			rr, err := executeRecordTemplate(answer, name, dns.TypeA)
			if err != nil {
				return fmt.Errorf("response rule %q: %w", rule.Name, err)
			}
			msg.Answer = append(msg.Answer, rr)
		}
		if rendered.anyAnswer != "" {
			rr, err := executeRecordTemplate(rendered.anyAnswer, name, dns.TypeCNAME)
			if err != nil {
				return fmt.Errorf("response rule %q: %w", rule.Name, err)
			}
			msg.Answer = append(msg.Answer, rr)
		}
		if rendered.anyAuthority != "" {
			if _, err := executeRecordTemplate(rendered.anyAuthority, name, dns.TypeSOA); err != nil {
				return fmt.Errorf("response rule %q: %w", rule.Name, err)
			}
		}
		if size := msg.Len(); size > maxUDPResponseSize {
			return fmt.Errorf("response rule %q: answer for %q is %d bytes, larger than a %d-byte UDP response", rule.Name, name, size, maxUDPResponseSize)
		}
	}
	return nil
}

// representativeQueryNames returns fully-qualified names a query matches.
func representativeQueryNames(query string) []string {
	suffix, wildcard := strings.CutPrefix(query, "*")
	if !wildcard {
		return []string{query + "."}
	}
	longest := 253 - len(suffix)
	if longest > 63 {
		longest = 63
	}
	names := []string{"a" + suffix + "."}
	if longest > 1 {
		names = append(names, strings.Repeat("a", longest)+suffix+".")
	}
	return names
}

func executeRecordTemplate(text, name string, rrType uint16) (dns.RR, error) {
	tmpl, err := template.New("record").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("record template %q does not compile: %w", text, err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, struct{ Name string }{Name: name}); err != nil {
		return nil, fmt.Errorf("record template %q failed for %q: %w", text, name, err)
	}
	rr, err := dns.NewRR(b.String())
	if err != nil || rr == nil {
		return nil, fmt.Errorf("record %q for %q does not parse: %v", b.String(), name, err)
	}
	if rr.Header().Rrtype != rrType {
		return nil, fmt.Errorf("record %q for %q has type %s, expected %s", b.String(), name, dns.TypeToString[rr.Header().Rrtype], dns.TypeToString[rrType])
	}
	return rr, nil
}

// sortResponseRulesBySpecificity preserves order within the exact and wildcard groups.
func sortResponseRulesBySpecificity(rules []operatorv1.DNSResponseRule) []operatorv1.DNSResponseRule {
	sorted := make([]operatorv1.DNSResponseRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return !isWildcardQuery(sorted[i].Query.Name) && isWildcardQuery(sorted[j].Query.Name)
	})
	return sorted
}

func isWildcardQuery(name string) bool {
	return strings.HasPrefix(name, "*.")
}

// responseRuleNameRegex converts a canonical DNSResponseQuery name (an
// exact name, or a name with exactly one leading wildcard label, e.g.
// "*.example.com") into an anchored regex matching the fully-qualified,
// lowercased query name CoreDNS presents to the template plugin (which
// includes the trailing dot).
//
// The wildcard case matches exactly one leading label -- not an arbitrary
// number of labels -- per the enhancement's restriction that wildcards
// replace exactly one leading name segment.
func responseRuleNameRegex(name string) string {
	if wildcardName, ok := strings.CutPrefix(name, "*."); ok {
		return `^[^.]+\.` + regexp.QuoteMeta(wildcardName) + `\.$`
	}
	return `^` + regexp.QuoteMeta(name) + `\.$`
}
