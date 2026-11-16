package controller

import (
	"context"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	opclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// responseRuleFakeCache reads from the same in-memory objects as the fake client.
type responseRuleFakeCache struct {
	client.Reader
}

func (responseRuleFakeCache) GetInformer(ctx context.Context, obj client.Object, opts ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}
func (responseRuleFakeCache) GetInformerForKind(ctx context.Context, gvk schema.GroupVersionKind, opts ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}
func (responseRuleFakeCache) RemoveInformer(ctx context.Context, obj client.Object) error { return nil }
func (responseRuleFakeCache) Start(ctx context.Context) error                             { return nil }
func (responseRuleFakeCache) WaitForCacheSync(ctx context.Context) bool                   { return true }
func (responseRuleFakeCache) IndexField(ctx context.Context, obj client.Object, field string, extractValue client.IndexerFunc) error {
	return nil
}

func TestReconcileWithResponseRules(t *testing.T) {
	scheme := opclient.GetScheme()

	network := &configv1.Network{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Status: configv1.NetworkStatus{
			ServiceNetwork: []string{"172.30.0.0/16"},
		},
	}
	ingress := &configv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}, Spec: configv1.IngressSpec{Domain: "apps.cluster.example.com"}}
	infrastructure := &configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}

	dns := &operatorv1.DNS{
		ObjectMeta: metav1.ObjectMeta{Name: DefaultDNSController},
		Spec: operatorv1.DNSSpec{
			ResponseRules: []operatorv1.DNSResponseRule{
				{
					Name:  "wildcard-apps",
					Query: operatorv1.DNSResponseQuery{Name: "*.apps.customer.com"},
					Action: operatorv1.DNSResponseAction{
						Type: operatorv1.DNSResponseActionTypeA,
						A:    operatorv1.DNSResponseActionA{Addresses: []operatorv1.IPv4Address{"203.0.113.10"}},
					},
				},
				{
					Name:  "blocked-host",
					Query: operatorv1.DNSResponseQuery{Name: "blocked.customer.com"},
					Action: operatorv1.DNSResponseAction{
						Type:     operatorv1.DNSResponseActionTypeNXDOMAIN,
						NXDOMAIN: &operatorv1.DNSResponseActionNXDOMAIN{},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(network, ingress, infrastructure, dns).
		WithStatusSubresource(&operatorv1.DNS{}).
		Build()

	r := &reconciler{
		Config: operatorconfig.Config{
			OperatorReleaseVersion: "test",
			OperatorNamespace:      "openshift-dns-operator",
			CoreDNSImage:           "example.com/coredns:test",
			OpenshiftCLIImage:      "example.com/cli:test",
			KubeRBACProxyImage:     "example.com/kube-rbac-proxy:test",
		},
		client: fakeClient,
		cache:  responseRuleFakeCache{Reader: fakeClient},
	}

	ctx := context.Background()
	// The fake cluster has no pods, so status reconciliation reports degraded.
	_, _ = r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: DefaultDNSController}})

	cm := &corev1.ConfigMap{}
	if err := fakeClient.Get(ctx, DNSConfigMapName(dns), cm); err != nil {
		t.Fatalf("failed to get configmap produced by a real Reconcile() call: %v", err)
	}

	corefile := cm.Data["Corefile"]
	for _, want := range []string{
		`template IN A . {`,
		`match "^[^.]+\.apps\.customer\.com\.$"`,
		`answer "{{ .Name }} 60 IN A 203.0.113.10"`,
		`template ANY ANY . {`,
		`match "^blocked\.customer\.com\.$"`,
		`rcode NXDOMAIN`,
	} {
		if !strings.Contains(corefile, want) {
			t.Errorf("expected Corefile produced by a real Reconcile() call to contain %q; got:\n%s", want, corefile)
		}
	}
}
