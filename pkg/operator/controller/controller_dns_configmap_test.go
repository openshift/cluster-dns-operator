package controller

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	operatorv1 "github.com/openshift/api/operator/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDesiredDNSConfigmap(t *testing.T) {
	clusterDomain := "cluster.local"
	dns := &operatorv1.DNS{
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
	}
	expectedCorefile := `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
# bar
bar.com:5353 example.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 3.3.3.3 {
        policy random
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
# fizz
fizz.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 5.5.5.5 6.6.6.6 {
        policy sequential
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
# buzz
buzz.com:5353 example.buzz.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 4.4.4.4 {
        policy random
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy sequential
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`
	////////// Check if Normal Log Level is Set ////////////////

	dnsToCheckNormalLogLevel := &operatorv1.DNS{
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
	}
	expectedCorefileToCheckIfNormaLogLevelIsSet := `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy sequential
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`

	//// Check if Debug Level is set //////////////////

	dnsToCheckDebugLogLevel := &operatorv1.DNS{
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
	}
	expectedCorefileToCheckIfDebugLogLevelIsSet := `# foo
		foo.com:5353 {
		    prometheus 127.0.0.1:9153
		    forward . 1.1.1.1 2.2.2.2:5353 {
		        policy round_robin
		    }
		    errors
            log . {
                class denial error
            }
		    bufsize 512
		    cache 900 {
		        denial 9984 30
		    }
		}
		.:5353 {
		    bufsize 512
		    errors
            log . {
                class denial error
            }
		    health {
		        lameduck 20s
		    }
		    ready
		    kubernetes cluster.local in-addr.arpa ip6.arpa {
		        pods insecure
		        fallthrough in-addr.arpa ip6.arpa
		    }
		    prometheus 127.0.0.1:9153
		    forward . /etc/resolv.conf {
		        policy sequential
		    }
		    cache 900 {
		        denial 9984 30
		    }
		    reload
		}
		`
	//// Check if Trace Level is set //////////////////
	dnsToCheckTraceLogLevel := &operatorv1.DNS{
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
	}
	expectedCorefileToCheckIfTraceLogLevelIsSet := `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class all
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class all
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy sequential
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`

	if cm, err := desiredDNSConfigMap(dns, clusterDomain); err != nil {
		t.Errorf("invalid dns configmap: %v", err)
	} else if cm.Data["Corefile"] != expectedCorefile {
		t.Errorf("unexpected Corefile; got:\n%s\nexpected:\n%s\n", cm.Data["Corefile"], expectedCorefile)
	}

	if cmToCheckNormaLogLevel, err := desiredDNSConfigMap(dnsToCheckNormalLogLevel, clusterDomain); err != nil {
		t.Errorf("invalid dns configmap: %v", err)
	} else if cmToCheckNormaLogLevel.Data["Corefile"] != expectedCorefileToCheckIfNormaLogLevelIsSet {
		t.Errorf("unexpected Corefile; got:\n%s\nexpected:\n%s\n", cmToCheckNormaLogLevel.Data["Corefile"], expectedCorefileToCheckIfNormaLogLevelIsSet)
	}

	if cmToCheckDebugLogLevel, err := desiredDNSConfigMap(dnsToCheckDebugLogLevel, clusterDomain); err != nil {
		t.Errorf("invalid dns configmap: %v", err)
	} else if cmToCheckDebugLogLevel.Data["Corefile"] == expectedCorefileToCheckIfDebugLogLevelIsSet {
		t.Errorf("unexpected Corefile; got:\n%s\nexpected:\n%s\n", cmToCheckDebugLogLevel.Data["Corefile"], expectedCorefileToCheckIfDebugLogLevelIsSet)
	}

	if cmToCheckTraceLogLevel, err := desiredDNSConfigMap(dnsToCheckTraceLogLevel, clusterDomain); err != nil {
		t.Errorf("invalid dns configmap: %v", err)
	} else if cmToCheckTraceLogLevel.Data["Corefile"] == expectedCorefileToCheckIfTraceLogLevelIsSet {
		t.Errorf("unexpected Corefile; got:\n%s\nexpected:\n%s\n", cmToCheckTraceLogLevel.Data["Corefile"], expectedCorefileToCheckIfTraceLogLevelIsSet)
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
								Address: "6.4.3.2",
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . 1.2.3.4 9.8.7.6 6.4.3.2:53 2.3.4.5:5353 127.0.0.53 {
        policy round_robin
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy round_robin
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy round_robin
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . 1.2.3.4 {
        policy sequential
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . 1.2.3.4 {
        policy random
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf [1001:AAAA:BBBB:CCCC::2222]:5353 10.0.0.1:53 [1001:AAAA:BBBB:CCCC::2222]:5354 {
        policy random
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy random
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy sequential
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf 1.3.4.5 {
        policy random
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
		},
		{
			name: "CR with upstreamResolvers containing 1 SystemResolvConf NS with Address  should return coreFile with 1 /etc/resolv.conf ignoring Address",
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy random
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
			expectedCoreFile: `# foo
foo.com:5353 {
    prometheus 127.0.0.1:9153
    forward . 1.1.1.1 2.2.2.2:5353 {
        policy round_robin
    }
    errors
    log . {
        class error
    }
    bufsize 512
    cache 900 {
        denial 9984 30
    }
}
.:5353 {
    bufsize 512
    errors
    log . {
        class error
    }
    health {
        lameduck 20s
    }
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
        pods insecure
        fallthrough in-addr.arpa ip6.arpa
    }
    prometheus 127.0.0.1:9153
    forward . /etc/resolv.conf {
        policy random
    }
    cache 900 {
        denial 9984 30
    }
    reload
}
`,
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
