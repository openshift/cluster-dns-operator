package responsewebhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/operator/responsevalidation"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	Reader client.Reader
}

var (
	webhookRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openshift_dns_response_rules_webhook_requests_total",
		Help: "DNS response rule admission requests by result.",
	}, []string{"result"})
	webhookDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "openshift_dns_response_rules_webhook_duration_seconds",
		Help:    "Duration of DNS response rule admission requests.",
		Buckets: prometheus.DefBuckets,
	})
)

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	result := "bad_request"
	defer func() {
		webhookRequests.WithLabelValues(result).Inc()
		webhookDuration.Observe(time.Since(started).Seconds())
	}()
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&review); err != nil {
		http.Error(w, fmt.Sprintf("invalid admission review: %v", err), http.StatusBadRequest)
		return
	}
	if review.Request == nil {
		http.Error(w, "admission review has no request", http.StatusBadRequest)
		return
	}
	response := h.validate(r, review.Request)
	if response.Allowed {
		result = "allowed"
	} else if response.Result != nil && response.Result.Reason == metav1.StatusReasonInvalid {
		result = "invalid"
	} else {
		result = "unavailable"
	}
	response.UID = review.Request.UID
	out := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{APIVersion: admissionv1.SchemeGroupVersion.String(), Kind: "AdmissionReview"},
		Response: &response,
	}
	if err := json.NewEncoder(w).Encode(out); err != nil {
		return
	}
}

func (h Handler) validate(r *http.Request, request *admissionv1.AdmissionRequest) admissionv1.AdmissionResponse {
	allow := admissionv1.AdmissionResponse{Allowed: true}
	if request.Operation != admissionv1.Create && request.Operation != admissionv1.Update {
		return allow
	}
	if request.SubResource != "" || request.Resource.Group != "operator.openshift.io" || request.Resource.Resource != "dnses" {
		return allow
	}
	var dns operatorv1.DNS
	if err := json.Unmarshal(request.Object.Raw, &dns); err != nil {
		return deny(metav1.StatusReasonBadRequest, http.StatusBadRequest, fmt.Sprintf("invalid DNS object: %v", err))
	}
	if dns.Name != "default" || len(dns.Spec.ResponseRules) == 0 {
		return allow
	}
	if h.Reader == nil {
		return deny(metav1.StatusReasonServiceUnavailable, http.StatusServiceUnavailable, "response rule validation is unavailable")
	}
	ingress := &configv1.Ingress{}
	if err := h.Reader.Get(r.Context(), types.NamespacedName{Name: "cluster"}, ingress); err != nil {
		return deny(metav1.StatusReasonServiceUnavailable, http.StatusServiceUnavailable, fmt.Sprintf("cannot read Ingress/cluster: %v", err))
	}
	infrastructure := &configv1.Infrastructure{}
	if err := h.Reader.Get(r.Context(), types.NamespacedName{Name: "cluster"}, infrastructure); err != nil {
		return deny(metav1.StatusReasonServiceUnavailable, http.StatusServiceUnavailable, fmt.Sprintf("cannot read Infrastructure/cluster: %v", err))
	}
	violations, err := responsevalidation.Validate(&dns, ingress, infrastructure)
	if err != nil {
		return deny(metav1.StatusReasonServiceUnavailable, http.StatusServiceUnavailable, err.Error())
	}
	if len(violations) == 0 {
		return allow
	}
	messages := make([]string, 0, len(violations))
	for _, violation := range violations {
		messages = append(messages, violation.Error())
	}
	return deny(metav1.StatusReasonInvalid, http.StatusUnprocessableEntity, strings.Join(messages, "; "))
}

func deny(reason metav1.StatusReason, code int, message string) admissionv1.AdmissionResponse {
	return admissionv1.AdmissionResponse{
		Allowed: false,
		Result:  &metav1.Status{Status: metav1.StatusFailure, Reason: reason, Code: int32(code), Message: message},
	}
}
