package controller

import (
	"testing"

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
