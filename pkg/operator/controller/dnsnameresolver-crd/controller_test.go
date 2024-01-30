package dnsnameresolvercrd

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/stretchr/testify/assert"

	configv1 "github.com/openshift/api/config/v1"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache/informertest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func Test_Reconcile(t *testing.T) {
	crd := func(name, key, value string) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					key: value,
				},
				Name: name,
			},
		}
	}
	tests := []struct {
		name                   string
		dnsNameResolverEnabled bool
		existingObjects        []runtime.Object
		expectCreate           []client.Object
		expectUpdate           []client.Object
		expectDelete           []client.Object
		expectStartCtrl        bool
	}{
		{
			name:                   "dnsNameResolver feature disabled",
			dnsNameResolverEnabled: false,
			expectCreate:           []client.Object{},
			expectUpdate:           []client.Object{},
			expectDelete:           []client.Object{},
			expectStartCtrl:        false,
		},
		{
			name:                   "dnsNameResolver feature enabled through TechPreviewNoUpgrade feature set",
			dnsNameResolverEnabled: true,
			existingObjects: []runtime.Object{
				&configv1.FeatureGate{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
					Spec: configv1.FeatureGateSpec{
						FeatureGateSelection: configv1.FeatureGateSelection{
							FeatureSet: configv1.TechPreviewNoUpgrade,
						},
					},
				},
			},
			expectCreate: []client.Object{
				crd("dnsnameresolvers.network.openshift.io", "release.openshift.io/feature-set", "TechPreviewNoUpgrade"),
			},
			expectUpdate:    []client.Object{},
			expectDelete:    []client.Object{},
			expectStartCtrl: true,
		},
		{
			name:                   "dnsNameResolver feature enabled through CustomNoUpgrade feature set",
			dnsNameResolverEnabled: true,
			existingObjects: []runtime.Object{
				&configv1.FeatureGate{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
					Spec: configv1.FeatureGateSpec{
						FeatureGateSelection: configv1.FeatureGateSelection{
							FeatureSet:      configv1.CustomNoUpgrade,
							CustomNoUpgrade: &configv1.CustomFeatureGates{Enabled: []configv1.FeatureGateName{configv1.FeatureGateDNSNameResolver}},
						},
					},
				},
			},
			expectCreate: []client.Object{
				crd("dnsnameresolvers.network.openshift.io", "release.openshift.io/feature-set", "CustomNoUpgrade"),
			},
			expectUpdate:    []client.Object{},
			expectDelete:    []client.Object{},
			expectStartCtrl: true,
		},
	}

	scheme := runtime.NewScheme()
	configv1.Install(scheme)
	apiextensionsv1.AddToScheme(scheme)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(tc.existingObjects...).
				Build()
			informer := informertest.FakeInformers{Scheme: scheme}
			fcache := fakeCache{Informers: &informer, Reader: fakeClient}
			cl := &fakeClientRecorder{fakeClient, t, []client.Object{}, []client.Object{}, []client.Object{}}
			ctrl := &fakeController{t, false, nil}
			fCacheWithStartAndSync := &fakeCacheWithStartAndSync{
				FakeInformers:         &informertest.FakeInformers{},
				Reader:                fake.NewClientBuilder().Build(),
				T:                     t,
				started:               false,
				startNotificationChan: nil,
				synced:                false,
				syncNotificationChan:  nil,
			}
			reconciler := &reconciler{
				cache:  fcache,
				client: cl,
				config: Config{
					DNSNameResolverEnabled: tc.dnsNameResolverEnabled,
					DependentCaches:        []cache.Cache{fCacheWithStartAndSync},
					DependentControllers:   []controller.Controller{ctrl},
				},
			}
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "cluster",
				},
			}

			res, err := reconciler.Reconcile(context.Background(), req)
			assert.NoError(t, err)
			assert.Equal(t, reconcile.Result{}, res)

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			select {
			case <-fCacheWithStartAndSync.syncNotificationChan:
				t.Log("WaitForCacheSync() was called")
			case <-ctx.Done():
				t.Log(ctx.Err())
			}
			cancel()

			ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			select {
			case <-fCacheWithStartAndSync.startNotificationChan:
				t.Log("Start() was called")
			case <-ctx.Done():
				t.Log(ctx.Err())
			}
			cancel()

			ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			select {
			case <-ctrl.startNotificationChan:
				t.Log("Start() was called")
			case <-ctx.Done():
				t.Log(ctx.Err())
			}
			cancel()

			assert.Equal(t, ctrl.started, tc.expectStartCtrl, "fake controller should have been started")
			cmpOpts := []cmp.Option{
				cmpopts.EquateEmpty(),
				cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion"),
				cmpopts.IgnoreMapEntries(func(key, value string) bool {
					return key != "release.openshift.io/feature-set"
				}),
				cmpopts.IgnoreFields(metav1.TypeMeta{}, "Kind", "APIVersion"),
				cmpopts.IgnoreFields(apiextensionsv1.CustomResourceDefinition{}, "Spec"),
			}
			if diff := cmp.Diff(tc.expectCreate, cl.added, cmpOpts...); diff != "" {
				t.Fatalf("found diff between expected and actual creates: %s", diff)
			}
			if diff := cmp.Diff(tc.expectUpdate, cl.updated, cmpOpts...); diff != "" {
				t.Fatalf("found diff between expected and actual updates: %s", diff)
			}
			if diff := cmp.Diff(tc.expectDelete, cl.deleted, cmpOpts...); diff != "" {
				t.Fatalf("found diff between expected and actual deletes: %s", diff)
			}
		})
	}
}

func TestReconcileOnlyStartsControllerOnce(t *testing.T) {
	scheme := runtime.NewScheme()
	configv1.Install(scheme)
	apiextensionsv1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(
			[]runtime.Object{
				&configv1.FeatureGate{
					ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
					Spec: configv1.FeatureGateSpec{
						FeatureGateSelection: configv1.FeatureGateSelection{FeatureSet: configv1.TechPreviewNoUpgrade},
					},
				},
			}...).
		Build()
	informer := informertest.FakeInformers{Scheme: scheme}
	fcache := fakeCache{Informers: &informer, Reader: fakeClient}
	cl := &fakeClientRecorder{fakeClient, t, []client.Object{}, []client.Object{}, []client.Object{}}
	fCacheWithStartAndSync := &fakeCacheWithStartAndSync{
		FakeInformers:         &informertest.FakeInformers{},
		Reader:                fake.NewClientBuilder().Build(),
		T:                     t,
		started:               false,
		startNotificationChan: make(chan struct{}),
		synced:                false,
		syncNotificationChan:  make(chan struct{}),
	}
	ctrl := &fakeController{t, false, make(chan struct{})}
	reconciler := &reconciler{
		cache:  fcache,
		client: cl,
		config: Config{
			DNSNameResolverEnabled: true,
			DependentCaches:        []cache.Cache{fCacheWithStartAndSync},
			DependentControllers:   []controller.Controller{ctrl},
		},
	}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: "cluster"}}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		select {
		case <-fCacheWithStartAndSync.syncNotificationChan:
			t.Log("cache WaitForCacheSync() was called for the first reconcile request")
		case <-ctx.Done():
			t.Error(ctx.Err())
		}
		cancel()
	}()

	// Reconcile once and verify Start() is called.
	res, err := reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	select {
	case <-fCacheWithStartAndSync.startNotificationChan:
		t.Log("cache Start() was called for the first reconcile request")
	case <-ctx.Done():
		t.Error(ctx.Err())
	}
	cancel()

	assert.True(t, fCacheWithStartAndSync.started, "fake cache should have been started")
	assert.True(t, fCacheWithStartAndSync.synced, "fake cache should have been synced")

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	select {
	case <-ctrl.startNotificationChan:
		t.Log("controller Start() was called for the first reconcile request")
	case <-ctx.Done():
		t.Error(ctx.Err())
	}
	cancel()
	assert.True(t, ctrl.started, "fake controller should have been started")

	// Reconcile again and verify Start() isn't called again.
	res, err = reconciler.Reconcile(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, res)

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	select {
	case <-fCacheWithStartAndSync.syncNotificationChan:
		t.Error("cache WaitForCacheSync() was called again for the second reconcile request")
	case <-ctx.Done():
		t.Log(ctx.Err())
	}
	cancel()

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	select {
	case <-fCacheWithStartAndSync.syncNotificationChan:
		t.Error("cache WaitForCacheSync() was called again for the second reconcile request")
	case <-ctx.Done():
		t.Log(ctx.Err())
	}
	cancel()

	assert.True(t, fCacheWithStartAndSync.started, "fake cache should have been started")
	assert.True(t, fCacheWithStartAndSync.synced, "fake cache should have been synced")

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	select {
	case <-ctrl.startNotificationChan:
		t.Error("controller Start() was called again for the second reconcile request")
	case <-ctx.Done():
		t.Log(ctx.Err())
	}
	cancel()
	assert.True(t, ctrl.started, "fake controller should have been started")
}

type fakeCache struct {
	cache.Informers
	client.Reader
}

type fakeClientRecorder struct {
	client.Client
	*testing.T

	added   []client.Object
	updated []client.Object
	deleted []client.Object
}

func (c *fakeClientRecorder) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *fakeClientRecorder) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	return c.Client.List(ctx, obj, opts...)
}

func (c *fakeClientRecorder) Scheme() *runtime.Scheme {
	return c.Client.Scheme()
}

func (c *fakeClientRecorder) RESTMapper() meta.RESTMapper {
	return c.Client.RESTMapper()
}

func (c *fakeClientRecorder) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.added = append(c.added, obj)
	return c.Client.Create(ctx, obj, opts...)
}

func (c *fakeClientRecorder) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	c.deleted = append(c.deleted, obj)
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *fakeClientRecorder) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.Client.DeleteAllOf(ctx, obj, opts...)
}

func (c *fakeClientRecorder) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.updated = append(c.updated, obj)
	return c.Client.Update(ctx, obj, opts...)
}

func (c *fakeClientRecorder) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *fakeClientRecorder) Status() client.StatusWriter {
	return c.Client.Status()
}

type fakeController struct {
	*testing.T
	// started indicates whether Start() has been called.
	started bool
	// startNotificationChan is an optional channel by which a test can
	// receive a notification when Start() is called.
	startNotificationChan chan struct{}
}

func (_ *fakeController) Reconcile(context.Context, reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (_ *fakeController) Watch(_ source.Source, _ handler.EventHandler, _ ...predicate.Predicate) error {
	return nil
}

func (c *fakeController) Start(_ context.Context) error {
	if c.started {
		c.T.Fatal("controller was started twice!")
	}
	c.started = true
	if c.startNotificationChan != nil {
		c.startNotificationChan <- struct{}{}
	}
	return nil
}

func (_ *fakeController) GetLogger() logr.Logger {
	return logr.Logger{}

}

type fakeCacheWithStartAndSync struct {
	*informertest.FakeInformers
	client.Reader

	*testing.T
	// started indicates whether Start() has been called.
	started bool
	// startNotificationChan is an optional channel by which a test can
	// receive a notification when Start() is called.
	startNotificationChan chan struct{}
	// synced indicates whether WaitForCacheSync() has been called.
	synced bool
	// syncNotificationChan is an optional channel by which a test can
	// receive a notification when WaitForCacheSync() is called.
	syncNotificationChan chan struct{}
}

func (fcache *fakeCacheWithStartAndSync) Start(ctx context.Context) error {
	if fcache.started {
		fcache.T.Fatal("cache was started twice!")
	}
	fcache.started = true
	if fcache.startNotificationChan != nil {
		fcache.startNotificationChan <- struct{}{}
	}
	return fcache.FakeInformers.Start(ctx)
}

func (fcache *fakeCacheWithStartAndSync) WaitForCacheSync(ctx context.Context) bool {
	if fcache.synced {
		fcache.T.Fatal("cache was syced twice!")
	}
	fcache.synced = true
	if fcache.syncNotificationChan != nil {
		fcache.syncNotificationChan <- struct{}{}
	}
	return fcache.FakeInformers.WaitForCacheSync(ctx)
}

func (fcache *fakeCacheWithStartAndSync) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return fcache.Reader.Get(ctx, key, obj, opts...)
}

func (fcache *fakeCacheWithStartAndSync) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	return fcache.Reader.List(ctx, obj, opts...)
}
