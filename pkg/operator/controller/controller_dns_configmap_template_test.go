package controller

import (
	"strings"
	"testing"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/operator/responsevalidation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTemplateAndResponseRuleOrder(t *testing.T) {
	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
		Spec: operatorv1.DNSSpec{
			Servers: []operatorv1.Server{{Name: "custom", Zones: []string{"custom.example.com"}}},
			Template: operatorv1.DNSTemplate{
				Zones:      []operatorv1.Zone{"."},
				QueryClass: operatorv1.QueryClassIN,
				QueryType:  operatorv1.QueryTypeAAAA,
				Action:     operatorv1.DNSTemplateAction{Rcode: operatorv1.ResponseCodeNOERROR},
			},
			ResponseRules: []operatorv1.DNSResponseRule{{
				Name:  "alias",
				Query: operatorv1.DNSResponseQuery{Name: "alias.example.com"},
				Action: operatorv1.DNSResponseAction{
					Type:  operatorv1.DNSResponseActionTypeCNAME,
					CNAME: operatorv1.DNSResponseActionCNAME{Target: "target.example.net"},
				},
			}},
		},
	}
	cm, err := desiredDNSConfigMap(dns, "cluster.local", nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	corefile := cm.Data["Corefile"]
	if strings.Count(corefile, "template IN AAAA . {") != 2 || strings.Count(corefile, "template ANY ANY . {") != 1 {
		t.Fatalf("expected one response rule stanza in the default block and one template in each of the default and custom blocks: %s", corefile)
	}
	defaultBlock := corefile[strings.Index(corefile, ".:5353 {"):]
	rule := strings.Index(defaultBlock, `match "^alias\.example\.com\.$"`)
	template := strings.LastIndex(defaultBlock, "template IN AAAA . {")
	if rule < 0 || template < 0 || rule > template {
		t.Fatalf("response rule must precede the DNS template in the default block: %s", defaultBlock)
	}
	stanza, err := renderDNSTemplateStanzas(dns.Spec.Template)
	if err != nil {
		t.Fatal(err)
	}
	if err := responsevalidation.ValidateRenderedStanzas(stanza); err != nil {
		t.Fatalf("rendered template stanza is invalid: %v", err)
	}
}
