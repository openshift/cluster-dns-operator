package responsewebhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	opclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestHandler(t *testing.T) {
	reader := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(
		&configv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: configv1.IngressSpec{Domain: "apps.cluster.example.com"}},
		&configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Status: configv1.InfrastructureStatus{
			APIServerURL: "https://api.external.example.com:6443", APIServerInternalURL: "https://api.internal.example.com:6443",
		}},
	).Build()
	address := func(name, query string) operatorv1.DNSResponseRule {
		return operatorv1.DNSResponseRule{
			Name: operatorv1.DNSResponseRuleName(name), Query: operatorv1.DNSResponseQuery{Name: query},
			Action: operatorv1.DNSResponseAction{Type: operatorv1.DNSResponseActionTypeA,
				A: operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}}},
		}
	}
	wildcard := operatorv1.DNSResponseRule{
		Name:  "wildcard",
		Query: operatorv1.DNSResponseQuery{Name: "*.example.com"},
		Action: operatorv1.DNSResponseAction{
			Type:  operatorv1.DNSResponseActionTypeCNAME,
			CNAME: operatorv1.DNSResponseActionCNAME{Target: "sink.example.com"},
		},
	}
	override := operatorv1.DNSResponseRule{
		Name:  "sink",
		Query: operatorv1.DNSResponseQuery{Name: "sink.example.com"},
		Action: operatorv1.DNSResponseAction{
			Type:  operatorv1.DNSResponseActionTypeCNAME,
			CNAME: operatorv1.DNSResponseActionCNAME{Target: "target.other.com"},
		},
	}
	conflictingA := operatorv1.DNSResponseRule{
		Name:  "sink-a",
		Query: operatorv1.DNSResponseQuery{Name: "sink.example.com"},
		Action: operatorv1.DNSResponseAction{
			Type: operatorv1.DNSResponseActionTypeA,
			A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}},
		},
	}
	for _, tc := range []struct {
		name        string
		rules       []operatorv1.DNSResponseRule
		servers     []operatorv1.Server
		reader      bool
		subresource string
		dryRun      bool
		wantAllow   bool
		wantReason  metav1.StatusReason
	}{
		{name: "unsafe wildcard cycle", rules: []operatorv1.DNSResponseRule{wildcard}, reader: true, wantReason: metav1.StatusReasonInvalid},
		{name: "safe exact override", rules: []operatorv1.DNSResponseRule{wildcard, override}, reader: true, wantAllow: true},
		{name: "exact A under a wildcard CNAME is allowed", rules: []operatorv1.DNSResponseRule{wildcard, conflictingA}, reader: true, wantAllow: true},
		{name: "ingress domain conflict", rules: []operatorv1.DNSResponseRule{address("ingress", "shop.apps.cluster.example.com")}, reader: true, wantReason: metav1.StatusReasonInvalid},
		{name: "external API hostname conflict", rules: []operatorv1.DNSResponseRule{address("external-api", "api.external.example.com")}, reader: true, wantReason: metav1.StatusReasonInvalid},
		{name: "internal API hostname conflict", rules: []operatorv1.DNSResponseRule{address("internal-api", "api.internal.example.com")}, reader: true, wantReason: metav1.StatusReasonInvalid},
		{name: "forward zone change conflicts with retained rule", rules: []operatorv1.DNSResponseRule{address("custom", "host.customer.example")}, servers: []operatorv1.Server{{Name: "customer", Zones: []string{"customer.example"}}}, reader: true, wantReason: metav1.StatusReasonInvalid},
		{name: "custom application domain is allowed", rules: []operatorv1.DNSResponseRule{address("custom", "host.customer.example")}, reader: true, wantAllow: true},
		{name: "dry-run has the same policy", rules: []operatorv1.DNSResponseRule{address("ingress", "shop.apps.cluster.example.com")}, reader: true, dryRun: true, wantReason: metav1.StatusReasonInvalid},
		{name: "empty rules bypass unavailable validator", wantAllow: true},
		{name: "status update bypasses unavailable validator", rules: []operatorv1.DNSResponseRule{wildcard}, subresource: "status", wantAllow: true},
		{name: "unavailable cluster configuration fails closed", rules: []operatorv1.DNSResponseRule{override}, wantReason: metav1.StatusReasonServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dns := &operatorv1.DNS{ObjectMeta: metav1.ObjectMeta{Name: "default"}, Spec: operatorv1.DNSSpec{ResponseRules: tc.rules, Servers: tc.servers}}
			raw, err := json.Marshal(dns)
			if err != nil {
				t.Fatal(err)
			}
			request := admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{
					UID:         types.UID("test"),
					Operation:   admissionv1.Update,
					SubResource: tc.subresource,
					Resource:    metav1.GroupVersionResource{Group: "operator.openshift.io", Version: "v1", Resource: "dnses"},
					Object:      runtime.RawExtension{Raw: raw},
				},
			}
			if tc.dryRun {
				request.Request.DryRun = &tc.dryRun
			}
			payload, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			handler := Handler{}
			if tc.reader {
				handler.Reader = reader
			}
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/validate", bytes.NewReader(payload)))
			if recorder.Code != http.StatusOK {
				t.Fatalf("unexpected HTTP status %d: %s", recorder.Code, recorder.Body.String())
			}
			var result admissionv1.AdmissionReview
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Response == nil || result.Response.Allowed != tc.wantAllow || result.Response.UID != types.UID("test") {
				t.Fatalf("unexpected admission response: %+v", result.Response)
			}
			if !tc.wantAllow && (result.Response.Result == nil || result.Response.Result.Reason != tc.wantReason) {
				t.Fatalf("wanted reason %q, got %+v", tc.wantReason, result.Response.Result)
			}
			if tc.name == "unsafe wildcard cycle" && !strings.Contains(result.Response.Result.Message, "cycle") {
				t.Fatalf("cycle error missing rule: %+v", result.Response.Result)
			}
		})
	}
}
