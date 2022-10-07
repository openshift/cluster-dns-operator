package controller

import (
	"errors"
	"io/ioutil"
	"path"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	v1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDesiredDNSConfigmap(t *testing.T) {
	testCases := []struct {
		name             string
		dns              *operatorv1.DNS
		expectedCoreFile string
		expectedError    error
	}{
		{
			name: "Check if Corefile is rendered correctly",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
						{
							Name:  "bar",
							Zones: []string{"bar.com", "example.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"3.3.3.3"},
								Policy:    operatorv1.RandomForwardingPolicy,
							},
						},
						{
							Name:  "fizz",
							Zones: []string{"fizz.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"5.5.5.5", "6.6.6.6"},
								Policy:    operatorv1.SequentialForwardingPolicy,
							},
						},
						{
							Name:  "buzz",
							Zones: []string{"buzz.com", "example.buzz.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"4.4.4.4"},
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "default_corefile"),
		},
		{
			name: "Check if log level is set to normal",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
					LogLevel: operatorv1.DNSLogLevelNormal,
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "normal_loglevel"),
		},
		{
			name: "Check if log level is set to debug",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
					LogLevel: operatorv1.DNSLogLevelDebug,
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "debug_loglevel"),
		},
		{
			name: "Check if log level is set to trace",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
					LogLevel: operatorv1.DNSLogLevelTrace,
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "trace_loglevel"),
		},
		{
			name: "Check the expected DNS-over-TLS settings",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								TransportConfig: operatorv1.DNSTransportConfig{
									Transport: operatorv1.TLSTransport,
									TLS: &operatorv1.DNSOverTLSConfig{
										ServerName: "dns.foo.com",
									},
								},
								Policy: operatorv1.RoundRobinForwardingPolicy,
							},
						},
						{
							Name:  "bar",
							Zones: []string{"bar.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								TransportConfig: operatorv1.DNSTransportConfig{
									Transport: operatorv1.TLSTransport,
									TLS: &operatorv1.DNSOverTLSConfig{
										ServerName: "dns.bar.com",
										CABundle: v1.ConfigMapNameReference{
											Name: "cacerts",
										},
									},
								},
								Policy: operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "forwardplugin_tls"),
		},
		{
			name: "Check the default cache settings",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{},
						NegativeTTL: metav1.Duration{},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "default_corefile_no_cache_configured"),
		},
		{
			name: "Default Corefile with valid cache configured",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{Duration: 9999 * time.Second},
						NegativeTTL: metav1.Duration{Duration: 29 * time.Second},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "default_corefile_cache_configured"),
		},
		{
			name: "Default Corefile with fractional cache values configured",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{Duration: 999 * time.Millisecond},
						NegativeTTL: metav1.Duration{Duration: 444 * time.Millisecond},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "default_corefile_cache_with_fractional_values_configured"),
		},
	}

	clusterDomain := "cluster.local"
	cmMap := make(map[string]string)
	cmMap["cacerts"] = "ca-cacerts-2"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if cm, err := desiredDNSConfigMap(tc.dns, clusterDomain, cmMap); err != nil {
				if !errors.Is(err, tc.expectedError) {
					t.Errorf("Unexpected error : %v", err)
				}
			} else if tc.expectedError != nil {
				t.Errorf("Error %v was expected", tc.expectedError)
			} else if diff := cmp.Diff(cm.Data["Corefile"], tc.expectedCoreFile); diff != "" {
				t.Errorf("unexpected Corefile;\n%s", diff)
			}
		})
	}
}

func TestDesiredDNSConfigmapUpstreamResolvers(t *testing.T) {
	testCases := []struct {
		name             string
		dns              *operatorv1.DNS
		expectedCoreFile string
		expectedError    error
	}{
		{
			name: "CR of 5 upstreams in upstreamResolvers should return a coreFile with 5 upstreams defined",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						//{"1.2.3.4", "9.8.7.6", "6.4.3.2:53", "2.3.4.5:5353", "127.0.0.53"},
						Upstreams: []operatorv1.Upstream{
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1.2.3.4",
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "9.8.7.6",
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1001:AAAA:bbbb:cCcC::2222",
								Port:    53,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "2.3.4.5",
								Port:    5353,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "127.0.0.53",
							},
						},
						Policy: operatorv1.RoundRobinForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "5upstreams"),
		},
		{
			name: "CR with upstreamResolvers containing just policy should return coreFile with default upstream /etc/resolv.conf and that policy",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Policy: operatorv1.RoundRobinForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "just_policy"),
		},
		{
			name: "CR with upstreamResolvers containing empty Upstreams array should return coreFile with default upstream /etc/resolv.conf and that policy",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{},
						Policy:    operatorv1.RoundRobinForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "empty_upstreams_array"),
		},
		{
			name: "CR with upstreamResolvers containing a network upstream without address should return error",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type: operatorv1.NetworkResolverType,
							},
						},
						Policy: operatorv1.RoundRobinForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: "",
			expectedError:    errInvalidNetworkUpstream,
		},
		{
			name: "CR with upstreamResolvers containing 1 Network NS and no policy should return coreFile with 1 upstream and policy sequential",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1.2.3.4",
							},
						},
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "1ns_no_policy"),
		},
		{
			name: "CR with upstreamResolvers containing 1 Network NS and policy should return coreFile with 1 upstream defined and that policy",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1.2.3.4",
							},
						},
						Policy: operatorv1.RandomForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "1ns_and_policy"),
		},
		{
			name: "CR with duplicates in upstreamResolvers.upstreams should return coreFile without duplicates",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type: operatorv1.SystemResolveConfType,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1001:AAAA:BBBB:CCCC::2222",
								Port:    5353,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1001:AAAA:BBBB:CCCC::2222",
								Port:    5353,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "10.0.0.1",
								Port:    53,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "10.0.0.1",
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1001:AAAA:BBBB:CCCC::2222",
								Port:    5354,
							},
							{
								Type: operatorv1.SystemResolveConfType,
								Port: 53,
							},
						},
						Policy: operatorv1.RandomForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "duplicate_upstreams"),
		},
		{
			name: "CR with upstreamResolvers.upstreams of type empty should return coreFile without duplicate /etc/resolv.conf",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type: operatorv1.SystemResolveConfType,
								Port: 53,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "100.1.1.1",
								Port:    5500,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "100.1.1.1",
								Port:    5500,
							},
							{
								Type: "",
								Port: 53,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1000::100",
								Port:    53,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1000::100",
								Port:    53,
							},
						},
						Policy: operatorv1.RandomForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "upstreams_type_empty"),
		},
		{
			name: "CR with upstreamResolvers containing 1 SystemResolvConf NS and policy should return a coreFile with 1 upstream defined and that policy",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type: operatorv1.SystemResolveConfType,
							},
						},
						Policy: operatorv1.RandomForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "1sysresconf"),
		},
		{
			name: "CR without upstreamResolvers defined should return coreFile with forwardPlugin upstream /etc/resolv.conf in sequential policy",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "without_upstreamresolvers"),
		},
		{
			name: "CR with multiple upstreamResolvers, of which /etc/resolv.conf is one, should return Corefile with all the upstreams",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type: operatorv1.SystemResolveConfType,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1.3.4.5",
							},
						},
						Policy: operatorv1.RandomForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "mult_upstreamresolvers"),
		},
		{
			name: "CR with upstreamResolvers containing 1 SystemResolvConf NS with Address should return coreFile with 1 /etc/resolv.conf ignoring Address",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type:    operatorv1.SystemResolveConfType,
								Address: "1.2.3.4",
							},
						},
						Policy: operatorv1.RandomForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "1sysresconf"),
		},
		{
			name: "CR with upstreamResolvers containing 1 SystemResolvConf NS with Port should return coreFile with 1 /etc/resolv.conf ignoring port",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type: operatorv1.SystemResolveConfType,
								Port: 54,
							},
						},
						Policy: operatorv1.RandomForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "1sysresconf"),
		},
		{
			name: "CR with upstreamResolvers containing 1 Network IPv6 NS and policy should return coreFile with 1 upstream defined and that policy",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1001:AAAA:bbbb:cCcC::2222",
							},
						},
						Policy: operatorv1.RandomForwardingPolicy,
					},
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								Policy:    operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "1ipv6_and_policy"),
		},
		{
			name: "CR of TLS-enabled upstreamResolvers including system resolve config should fail",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type: operatorv1.SystemResolveConfType,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "9.8.7.6",
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1001:AAAA:bbbb:cCcC::2222",
								Port:    53,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "2.3.4.5",
								Port:    5353,
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "127.0.0.53",
							},
						},
						Policy: operatorv1.RoundRobinForwardingPolicy,
						TransportConfig: operatorv1.DNSTransportConfig{
							Transport: operatorv1.TLSTransport,
							TLS: &operatorv1.DNSOverTLSConfig{
								ServerName: "example.com",
							},
						},
					},
				},
			},
			expectedError: errTransportTLSConfiguredForSysResConf,
		},
		{
			name: "CR of TLS-enabled forwardPlugin including a non-ip should fail",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"non-ip", "2.2.2.2:5353"},
								TransportConfig: operatorv1.DNSTransportConfig{
									Transport: operatorv1.TLSTransport,
									TLS: &operatorv1.DNSOverTLSConfig{
										ServerName: "example.com",
									},
								},
								Policy: operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedError: errTransportTLSConfiguredForNonIP,
		},
		{
			name: "CR of TLS-enabled forwardPlugin including a non-ip with port should fail",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"non-ip:5353", "2.2.2.2:5353"},
								TransportConfig: operatorv1.DNSTransportConfig{
									Transport: operatorv1.TLSTransport,
									TLS: &operatorv1.DNSOverTLSConfig{
										ServerName: "example.com",
									},
								},
								Policy: operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedError: errTransportTLSConfiguredForNonIP,
		},
		{
			name: "CR of TLS-enabled forwardPlugin having no serverName should fail",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					Servers: []operatorv1.Server{
						{
							Name:  "foo",
							Zones: []string{"foo.com"},
							ForwardPlugin: operatorv1.ForwardPlugin{
								Upstreams: []string{"1.1.1.1", "2.2.2.2:5353"},
								TransportConfig: operatorv1.DNSTransportConfig{
									Transport: operatorv1.TLSTransport,
								},
								Policy: operatorv1.RoundRobinForwardingPolicy,
							},
						},
					},
				},
			},
			expectedError: errTransportTLSConfiguredWithoutServerName,
		},
		{
			name: "CR with upstreamResolvers using TLS defining a CA bundle should return a coreFile containing upstreams with TLS and CA bundle",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "9.8.7.6",
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1001:AAAA:bbbb:cCcC::2222",
								Port:    53,
							},
						},
						Policy: operatorv1.RoundRobinForwardingPolicy,
						TransportConfig: operatorv1.DNSTransportConfig{
							Transport: operatorv1.TLSTransport,
							TLS: &operatorv1.DNSOverTLSConfig{
								ServerName: "example.com",
								CABundle: v1.ConfigMapNameReference{
									Name: "ca-bundle-config",
								},
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "upstreamresolvers_with_cabundle"),
		},
		{
			name: "CR with upstreamResolvers using TLS defining a non-existing CA bundle should return a coreFile containing upstreams with TLS and no CA bundle",
			dns: &operatorv1.DNS{
				ObjectMeta: metav1.ObjectMeta{
					Name: DefaultDNSController,
				},
				Spec: operatorv1.DNSSpec{
					UpstreamResolvers: operatorv1.UpstreamResolvers{
						Upstreams: []operatorv1.Upstream{
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "9.8.7.6",
							},
							{
								Type:    operatorv1.NetworkResolverType,
								Address: "1001:AAAA:bbbb:cCcC::2222",
								Port:    53,
							},
						},
						Policy: operatorv1.RoundRobinForwardingPolicy,
						TransportConfig: operatorv1.DNSTransportConfig{
							Transport: operatorv1.TLSTransport,
							TLS: &operatorv1.DNSOverTLSConfig{
								ServerName: "example.com",
								CABundle: v1.ConfigMapNameReference{
									Name: "ca-bundle-config-2",
								},
							},
						},
					},
				},
			},
			expectedCoreFile: mustLoadTestFile(t, "tls_with_non_existing_cabundle"),
		},
	}

	clusterDomain := "cluster.local"
	cmMap := make(map[string]string)
	cmMap["ca-bundle-config"] = "ca-ca-bundle-config-1"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if cm, err := desiredDNSConfigMap(tc.dns, clusterDomain, cmMap); err != nil {
				if !errors.Is(err, tc.expectedError) {
					t.Errorf("Unexpected error : %v", err)
				}
			} else if tc.expectedError != nil {
				t.Errorf("Error %v was expected", tc.expectedError)
			} else if diff := cmp.Diff(cm.Data["Corefile"], tc.expectedCoreFile); diff != "" {
				t.Errorf("unexpected Corefile;\n%s", diff)
			}
		})
	}
}

func Test_coreDNSCache(t *testing.T) {
	testcases := []struct {
		name         string
		dns          *operatorv1.DNS
		expectedPTTL uint32
		expectedNTTL uint32
	}{
		{
			name: "no configured cache values results in default settings",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{},
				},
			},
			expectedPTTL: cacheDefaultMaxPositiveTTLSeconds,
			expectedNTTL: cacheDefaultMaxNegativeTTLSeconds,
		},
		{
			name: "1s",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{1 * time.Second},
					},
				},
			},
			expectedPTTL: 1,
			expectedNTTL: cacheDefaultMaxNegativeTTLSeconds,
		},
		{
			name: "1m",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{1 * time.Minute},
					},
				},
			},
			expectedPTTL: 60,
			expectedNTTL: cacheDefaultMaxNegativeTTLSeconds,
		},
		{
			name: "1h",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						NegativeTTL: metav1.Duration{1 * time.Hour},
					},
				},
			},
			expectedPTTL: cacheDefaultMaxPositiveTTLSeconds,
			expectedNTTL: 3600,
		},
		{
			name: "1h10m5s",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						NegativeTTL: metav1.Duration{1*time.Hour + 10*time.Minute + 5*time.Second},
					},
				},
			},
			expectedPTTL: cacheDefaultMaxPositiveTTLSeconds,
			expectedNTTL: 4205,
		},
		{
			name: "999ms rounds to 1",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{999 * time.Millisecond},
						NegativeTTL: metav1.Duration{999 * time.Millisecond},
					},
				},
			},
			expectedPTTL: 1,
			expectedNTTL: 1,
		},
		{
			name: "1.1s rounds to 1",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{1*time.Second + 100*time.Millisecond},
						NegativeTTL: metav1.Duration{1*time.Second + 100*time.Millisecond},
					},
				},
			},
			expectedPTTL: 1,
			expectedNTTL: 1,
		},
		{
			name: "5.9s rounds to 6",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{5*time.Second + 900*time.Millisecond},
						NegativeTTL: metav1.Duration{5*time.Second + 900*time.Millisecond},
					},
				},
			},
			expectedPTTL: 6,
			expectedNTTL: 6,
		},
		{
			name: "2.009s rounds to 2",
			dns: &operatorv1.DNS{
				Spec: operatorv1.DNSSpec{
					Cache: operatorv1.DNSCache{
						PositiveTTL: metav1.Duration{2*time.Second + 9*time.Millisecond},
						NegativeTTL: metav1.Duration{2*time.Second + 9*time.Millisecond},
					},
				},
			},
			expectedPTTL: 2,
			expectedNTTL: 2,
		},
	}

	for _, tc := range testcases {
		pTTL, nTTL := coreDNSCache(tc.dns)
		if tc.expectedPTTL != pTTL {
			t.Errorf("test case %s failed: expected %d PositiveTTL, got %d", tc.name, tc.expectedPTTL, pTTL)
		}
		if tc.expectedNTTL != nTTL {
			t.Errorf("test case %s failed: expected %d NegativeTTL, got %d", tc.name, tc.expectedNTTL, nTTL)
		}
	}
}

// mustLoadTestFile looks in the default directory of ./testdata for a file matching the name argument
// and returns the file contents as a string.
func mustLoadTestFile(t *testing.T, name string) string {
	t.Helper()
	corefile, err := ioutil.ReadFile(path.Join("testdata", name))
	if err != nil {
		t.Fatalf("Failed to read Corefile %s: %v", name, err)
	}
	return string(corefile)
}
