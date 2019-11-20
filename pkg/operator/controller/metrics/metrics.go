package metrics

import (
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// DNSesTotal is a Prometheus gauge metric which holds the total number
	// of DNSes.
	DNSesTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dns_operator_dnses_total",
		Help: "Total number of DNSes",
	}, []string{"dnses"})

	// DNSesUsingForwardingAPI is a Prometheus gauge metric which indicates
	// how many DNS operands are using the Forwarding API.
	DNSesUsingForwardingAPI = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dns_operator_dnses_using_forwarding_api",
		Help: "Number of DNSes using the forwarding API",
	}, []string{"dnses"})
)

func Update(dnses []operatorv1.DNS) {
	DNSesTotal.WithLabelValues("dnses").Set(float64(len(dnses)))

	numUsingForwarding := 0
	for _, dns := range dnses {
		for _, server := range dns.Spec.Servers {
			if len(server.ForwardPlugin.Upstreams) > 0 {
				numUsingForwarding++
				break
			}
		}
	}
	DNSesUsingForwardingAPI.WithLabelValues("dnses").Set(float64(numUsingForwarding))
}

func init() {
	metrics.Registry.MustRegister(
		DNSesTotal,
		DNSesUsingForwardingAPI,
	)
}
