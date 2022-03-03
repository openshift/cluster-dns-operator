package controller

import (
	"errors"
	"io/ioutil"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
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
	}

	clusterDomain := "cluster.local"
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if cm, err := desiredDNSConfigMap(tc.dns, clusterDomain); err != nil {
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
	}

	clusterDomain := "cluster.local"

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if cm, err := desiredDNSConfigMap(tc.dns, clusterDomain); err != nil {
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
