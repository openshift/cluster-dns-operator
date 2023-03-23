package controller

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"text/template"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/cluster-dns-operator/pkg/manifests"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	resolvConf       = "/etc/resolv.conf"
	defaultDNSPort   = 53
	lameDuckDuration = 20 * time.Second

	// cacheDefaultMaxPositiveTTLSeconds is the default maximum TTL that the
	// operator configures CoreDNS to enforce for positive (NOERROR)
	// responses.
	cacheDefaultMaxPositiveTTLSeconds = 900
	// cacheDefaultMaxNegativeTTLSeconds is the default maximum TTL that the
	// operator configures CoreDNS to enforce for negative (NXDOMAIN)
	// responses.
	cacheDefaultMaxNegativeTTLSeconds = 30
)

var errInvalidNetworkUpstream = fmt.Errorf("The address field is mandatory for upstream of type Network, but was not provided")
var errTransportTLSConfiguredWithoutServerName = fmt.Errorf("The ServerName field is mandatory when configuring TLS as the DNS Transport")
var errTransportTLSConfiguredForNonIP = fmt.Errorf("Only IP addresses are allowed when configuring TLS as the DNS Transport")
var errTransportTLSConfiguredForSysResConf = fmt.Errorf("Using system resolv config is not allowed when configuring TLS as the DNS Transport")
var corefileTemplate = template.Must(template.New("Corefile").Funcs(template.FuncMap{
	"CoreDNSForwardingPolicy": coreDNSPolicy, "UpstreamResolver": coreDNSResolver,
}).Parse(`{{range .Servers -}}
# {{.Name}}
{{range .Zones}}{{.}}:5353 {{end}}{
    {{with $fp:=.ForwardPlugin -}}
    prometheus 127.0.0.1:9153
    forward .{{range $fp.Upstreams}} {{if eq "TLS" $fp.TransportConfig.Transport}}tls://{{end}}{{.}}{{end}} {
        {{- with $tls := .TransportConfig.TLS }}
        {{- with $serverName := $tls.ServerName }}
        tls_servername {{$serverName}}
        tls {{- with $.CABundleRevisionMap }}{{- with $cm := (index $.CABundleRevisionMap $tls.CABundle.Name) }} /etc/pki/{{$serverName}}-{{ $cm }}/{{ $.CABundleFileName }}{{end}}{{end}}
        {{- end}}
        {{- end}}
        policy {{ CoreDNSForwardingPolicy .Policy }}
        {{- if eq "TCP" $fp.ProtocolStrategy }}
        force_tcp
        {{- end}}
    }
    {{- end}}
    errors
    log . {
        {{$.LogLevel}}
    }
    bufsize 512
    cache {{ $.PositiveTTL }} {
        denial 9984 {{ $.NegativeTTL }}
    }
}
{{end -}}
.:5353 {
    bufsize 512
    errors
    log . {
        {{.LogLevel}}
    }
    health {
        lameduck {{.LameDuckDuration}}
    }
    ready
    kubernetes {{.ClusterDomain}} in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    {{- with .UpstreamResolvers }}
    forward .{{range .Upstreams}} {{if eq "TLS" $.UpstreamResolvers.TransportConfig.Transport}}tls://{{end}}{{UpstreamResolver .}}{{end}} {
        {{- with $tls := .TransportConfig.TLS }}
        {{- with $serverName := $tls.ServerName }}
        tls_servername {{$serverName}}
        tls {{- with $.CABundleRevisionMap }}{{- with $cm := (index $.CABundleRevisionMap $tls.CABundle.Name) }} /etc/pki/{{$serverName}}-{{ $cm }}/{{ $.CABundleFileName }}{{end}}{{end}}
        {{- end}}
        {{- end}}
        policy {{ CoreDNSForwardingPolicy .Policy }}
        {{- if eq "TCP" $.UpstreamResolvers.ProtocolStrategy }}
        force_tcp
        {{- end}}
    }
    {{- end}}
    cache {{ .PositiveTTL }} {
        denial 9984 {{ .NegativeTTL }}
    }
    reload
}
hostname.bind:5353 {
    chaos
}
`))

// ensureDNSConfigMap ensures that a configmap exists for a given DNS.
func (r *reconciler) ensureDNSConfigMap(dns *operatorv1.DNS, clusterDomain string, caBundleRevisionMap map[string]string) (bool, *corev1.ConfigMap, error) {
	haveCM, current, err := r.currentDNSConfigMap(dns)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get configmap: %v", err)
	}
	desired, err := desiredDNSConfigMap(dns, clusterDomain, caBundleRevisionMap)
	if err != nil {
		return haveCM, current, fmt.Errorf("failed to build configmap: %v", err)
	}

	switch {
	case !haveCM:
		if err := r.client.Create(context.TODO(), desired); err != nil {
			return false, nil, fmt.Errorf("failed to create configmap: %v", err)
		}
		logrus.Infof("created configmap: %s", desired.Name)
		return r.currentDNSConfigMap(dns)
	case haveCM:
		if updated, err := r.updateDNSConfigMap(current, desired); err != nil {
			return true, current, err
		} else if updated {
			return r.currentDNSConfigMap(dns)
		}
	}
	return true, current, nil
}

func (r *reconciler) currentDNSConfigMap(dns *operatorv1.DNS) (bool, *corev1.ConfigMap, error) {
	current := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), DNSConfigMapName(dns), current)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, current, nil
}

func desiredDNSConfigMap(dns *operatorv1.DNS, clusterDomain string, caBundleRevisionMap map[string]string) (*corev1.ConfigMap, error) {
	if len(clusterDomain) == 0 {
		clusterDomain = "cluster.local"
	}

	dns, err := sanitizeTLSSettings(dns)
	if err != nil {
		return nil, err
	}

	upstreamResolvers := operatorv1.UpstreamResolvers{
		Upstreams: []operatorv1.Upstream{
			{
				Type: operatorv1.SystemResolveConfType,
			},
		},
		Policy:           operatorv1.SequentialForwardingPolicy,
		TransportConfig:  dns.Spec.UpstreamResolvers.TransportConfig,
		ProtocolStrategy: dns.Spec.UpstreamResolvers.ProtocolStrategy,
	}

	if len(dns.Spec.UpstreamResolvers.Upstreams) > 0 {
		//Upstreams are defined, we can remove the default one
		upstreamResolvers.Upstreams = []operatorv1.Upstream{}

		for _, upstream := range dns.Spec.UpstreamResolvers.Upstreams {
			if upstream.Type == operatorv1.NetworkResolverType {
				if upstream.Address == "" {
					return nil, errInvalidNetworkUpstream
				}
			}
			upstreamCopy := *upstream.DeepCopy()
			//appending only if there are no duplicates
			if !contains(upstreamResolvers.Upstreams, upstream) {
				upstreamResolvers.Upstreams = append(upstreamResolvers.Upstreams, upstreamCopy)
			}
		}
	}

	if dns.Spec.UpstreamResolvers.Policy != "" {
		upstreamResolvers.Policy = dns.Spec.UpstreamResolvers.Policy
	}

	// Calculate the caching values (in seconds) for use in the Corefile
	pTTL, nTTL := coreDNSCache(dns)

	corefileParameters := struct {
		ClusterDomain       string
		Servers             interface{}
		UpstreamResolvers   operatorv1.UpstreamResolvers
		PolicyStr           func(policy operatorv1.ForwardingPolicy) string
		LogLevel            string
		CABundleRevisionMap map[string]string
		CABundleFileName    string
		LameDuckDuration    time.Duration
		PositiveTTL         uint32
		NegativeTTL         uint32
	}{
		ClusterDomain:       clusterDomain,
		Servers:             dns.Spec.Servers,
		UpstreamResolvers:   upstreamResolvers,
		PolicyStr:           coreDNSPolicy,
		LogLevel:            coreDNSLogLevel(dns),
		CABundleRevisionMap: caBundleRevisionMap,
		CABundleFileName:    caBundleFileName,
		LameDuckDuration:    lameDuckDuration,
		PositiveTTL:         pTTL,
		NegativeTTL:         nTTL,
	}
	corefile := new(bytes.Buffer)
	if err := corefileTemplate.Execute(corefile, corefileParameters); err != nil {
		return nil, err
	}

	name := DNSConfigMapName(dns)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			Labels: map[string]string{
				manifests.OwningDNSLabel: DNSDaemonSetLabel(dns),
			},
		},
		Data: map[string]string{
			"Corefile": corefile.String(),
		},
	}
	cm.SetOwnerReferences([]metav1.OwnerReference{dnsOwnerRef(dns)})

	return cm, nil
}

// sanitizeTLSSettings sanitizes TLS settings by setting ServerName to empty string when TLS is not configured, and by
// setting ClearText as the default value when Transport is not set. It also makes sure TLS is configured only for IP addresses.
func sanitizeTLSSettings(dns *operatorv1.DNS) (*operatorv1.DNS, error) {
	updated := dns.DeepCopy()
	for i, server := range updated.Spec.Servers {
		transport := server.ForwardPlugin.TransportConfig.Transport
		tls := server.ForwardPlugin.TransportConfig.TLS
		if transport == operatorv1.TLSTransport {
			// tls cannot be configured without a ServerName
			if tls == nil || tls.ServerName == "" {
				return updated, errTransportTLSConfiguredWithoutServerName
			}
			// tls can only be configured for ip addresses
			for _, upstream := range server.ForwardPlugin.Upstreams {
				addr := upstream
				if v, _, err := net.SplitHostPort(upstream); err == nil {
					addr = v
				}
				if res := net.ParseIP(addr); res == nil {
					return updated, errTransportTLSConfiguredForNonIP
				}
			}
		}
		// When Transport is "", set the default as "Cleartext".
		if transport == "" {
			transport = operatorv1.CleartextTransport
			updated.Spec.Servers[i].ForwardPlugin.TransportConfig.Transport = transport
		}
		// When Transport is "Cleartext" and a ServerName is set, the Corefile will ignore any other TLS settings.
		if transport == operatorv1.CleartextTransport && tls != nil && tls.ServerName != "" {
			logrus.Warningf("ServerName is set in server %q but Transport is not set to TLS. ServerName will be ignored", server.Name)
			updated.Spec.Servers[i].ForwardPlugin.TransportConfig.TLS.ServerName = ""
		}
	}
	transport := updated.Spec.UpstreamResolvers.TransportConfig.Transport
	tls := updated.Spec.UpstreamResolvers.TransportConfig.TLS
	if transport == operatorv1.TLSTransport {
		// tls cannot be configured without a ServerName
		if tls == nil || tls.ServerName == "" {
			return updated, errTransportTLSConfiguredWithoutServerName
		}
		// if there is no upstream, system resolv conf will be used as default,
		// and tls can only be configured for ip addresses
		if len(updated.Spec.UpstreamResolvers.Upstreams) < 1 {
			return updated, errTransportTLSConfiguredForSysResConf
		}
		// tls can only be configured for ip addresses
		for _, upstream := range updated.Spec.UpstreamResolvers.Upstreams {
			if upstream.Type == operatorv1.SystemResolveConfType {
				return updated, errTransportTLSConfiguredForSysResConf
			}
		}
	}

	// When Transport is "", set the default as cleartext.
	if transport == "" {
		transport = operatorv1.CleartextTransport
		updated.Spec.UpstreamResolvers.TransportConfig.Transport = transport
	}
	// When Transport is "" or "Cleartext" and a ServerName is set, the Corefile will ignore any other TLS settings.
	if transport == operatorv1.CleartextTransport && tls != nil && tls.ServerName != "" {
		logrus.Warningf("ServerName is set but Transport is not set to tls. ServerName will be ignored")
		updated.Spec.UpstreamResolvers.TransportConfig.TLS.ServerName = ""
	}

	return updated, nil
}

func (r *reconciler) updateDNSConfigMap(current, desired *corev1.ConfigMap) (bool, error) {
	changed, updated := corefileChanged(current, desired)
	if !changed {
		return false, nil
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, fmt.Errorf("failed to update configmap: %v", err)
	}
	logrus.Infof("updated configmap %s/%s: %v", updated.Namespace, updated.Name, diff)
	return true, nil
}

func corefileChanged(current, expected *corev1.ConfigMap) (bool, *corev1.ConfigMap) {
	if cmp.Equal(current.Data, expected.Data, cmpopts.EquateEmpty()) {
		return false, current
	}
	updated := current.DeepCopy()
	updated.Data = expected.Data
	return true, updated
}

func coreDNSResolver(upstream operatorv1.Upstream) (string, error) {
	if upstream.Type == operatorv1.NetworkResolverType {
		if upstream.Address == "" {
			return "", errInvalidNetworkUpstream
		}

		if upstream.Port > 0 {
			return net.JoinHostPort(strings.ToUpper(upstream.Address), fmt.Sprintf("%d", upstream.Port)), nil
		} else {
			return strings.ToUpper(upstream.Address), nil
		}
	}
	return resolvConf, nil
}

func coreDNSPolicy(policy operatorv1.ForwardingPolicy) string {
	switch policy {
	case operatorv1.RandomForwardingPolicy:
		return "random"
	case operatorv1.RoundRobinForwardingPolicy:
		return "round_robin"
	case operatorv1.SequentialForwardingPolicy:
		return "sequential"
	}
	return "random"
}

func coreDNSLogLevel(dns *operatorv1.DNS) string {
	switch dns.Spec.LogLevel {
	case operatorv1.DNSLogLevelNormal:
		return "class error"
	case operatorv1.DNSLogLevelDebug:
		return "class denial error"
	case operatorv1.DNSLogLevelTrace:
		return "class all"
	}
	return "class error"
}

// coreDNSCache reads the TTL values set in spec.cache and returns integer representations of those values in seconds.
// This returns uint32 values because CoreDNS won't allow setting the maxTTLs to 0 and there's no use case for setting a
// negative maxTTL value.
func coreDNSCache(dns *operatorv1.DNS) (positiveTTL, negativeTTL uint32) {
	// We're using Round() here so if a fractional value is provided it'll abide by
	// normal rounding rules (e.g. halfway or greater, round up, else round down). Seconds()
	// returns the number of seconds as a float64, and then we convert that to an integer for
	// use in the Corefile.
	configuredPosTTL := uint32(dns.Spec.Cache.PositiveTTL.Round(time.Second).Seconds())
	configuredNegTTL := uint32(dns.Spec.Cache.NegativeTTL.Round(time.Second).Seconds())

	// For values <=0, Round() will return them as-is so we'll use the default if that happens
	// because CoreDNS won't allow setting a negative value.
	if configuredPosTTL <= 0 {
		positiveTTL = cacheDefaultMaxPositiveTTLSeconds
	} else {
		positiveTTL = configuredPosTTL
	}

	if configuredNegTTL <= 0 {
		negativeTTL = cacheDefaultMaxNegativeTTLSeconds
	} else {
		negativeTTL = configuredNegTTL
	}

	return positiveTTL, negativeTTL
}

func contains(upstreams []operatorv1.Upstream, upstream operatorv1.Upstream) bool {
	for _, anUpstream := range upstreams {
		if cmp.Equal(upstream, anUpstream, cmp.Comparer(cmpPort), cmp.Comparer(cmpAddress), cmp.Comparer(cmpUpstreamType)) {
			return true
		}
	}
	return false
}

func cmpUpstreamType(a, b operatorv1.UpstreamType) bool {
	if a == "" {
		a = operatorv1.SystemResolveConfType
	}
	if b == "" {
		b = operatorv1.SystemResolveConfType
	}
	return a == b
}

func cmpPort(a, b uint32) bool {
	aVal := uint32(defaultDNSPort)
	if a != 0 {
		aVal = a
	}
	bVal := uint32(defaultDNSPort)
	if b != 0 {
		bVal = b
	}
	return aVal == bVal
}

func cmpAddress(a, b string) bool {
	return strings.EqualFold(a, b)
}
