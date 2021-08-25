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
					},
				},
				{
					Name:  "bar",
					Zones: []string{"bar.com", "example.com"},
					ForwardPlugin: operatorv1.ForwardPlugin{
						Upstreams: []string{"3.3.3.3"},
					},
				},
			},
		},
	}
	expectedCorefile := `# foo
foo.com:5353 {
    forward . 1.1.1.1 2.2.2.2:5353
    errors
    bufsize 512
}
# bar
bar.com:5353 example.com:5353 {
    forward . 3.3.3.3
    errors
    bufsize 512
}
.:5353 {
    bufsize 512
    errors
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
}
