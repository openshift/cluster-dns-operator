package controller

import (
	"context"
	"testing"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	configv1 "github.com/openshift/api/config/v1"
	opclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	operatorconfig "github.com/openshift/cluster-dns-operator/pkg/operator/config"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResponseWebhookRegistrationWaitsForReadiness(t *testing.T) {
	const namespace = "openshift-dns-operator"
	ctx := context.Background()
	operatorDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "dns-operator", Namespace: namespace},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "operator", Image: "example.com/dns-operator:test"}},
		}}},
	}
	client := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).
		WithObjects(operatorDeployment, &configv1.Infrastructure{ObjectMeta: metav1.ObjectMeta{Name: "cluster"}}).
		WithStatusSubresource(&appsv1.Deployment{}).
		Build()
	r := &reconciler{Config: operatorconfig.Config{OperatorNamespace: namespace}, client: client}
	if err := r.ensureResponseWebhook(ctx); err == nil {
		t.Fatal("expected the webhook to require a configured image")
	}
	r.Config.ResponseWebhookImage = "example.com/dns-operator:webhook"
	if err := r.ensureResponseWebhook(ctx); err == nil {
		t.Fatal("expected registration to wait for the serving certificate")
	}
	service := &corev1.Service{}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName, Namespace: namespace}, service); err != nil {
		t.Fatal(err)
	}
	if service.Annotations["service.beta.openshift.io/serving-cert-secret-name"] != responseWebhookSecretName {
		t.Fatalf("service does not request its serving certificate: %v", service.Annotations)
	}
	configuration := &admissionv1.ValidatingWebhookConfiguration{}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName}, configuration); err == nil {
		t.Fatal("webhook was registered before its endpoint was ready")
	}

	if err := client.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: responseWebhookSecretName, Namespace: namespace},
		Data:       map[string][]byte{"tls.crt": []byte("cert"), "tls.key": []byte("key")},
	}); err != nil {
		t.Fatal(err)
	}
	ca := &corev1.ConfigMap{}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookCAName, Namespace: namespace}, ca); err != nil {
		t.Fatal(err)
	}
	ca.Data = map[string]string{"service-ca.crt": "CA"}
	if err := client.Update(ctx, ca); err != nil {
		t.Fatal(err)
	}
	deployment := &appsv1.Deployment{}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName, Namespace: namespace}, deployment); err != nil {
		t.Fatal(err)
	}
	if image := deployment.Spec.Template.Spec.Containers[0].Image; image != "example.com/dns-operator:webhook" {
		t.Fatalf("webhook image must come from the configured image, got %q", image)
	}
	deployment.Status.AvailableReplicas = 1
	if err := client.Status().Update(ctx, deployment); err != nil {
		t.Fatal(err)
	}
	if err := r.ensureResponseWebhook(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName}, configuration); err != nil {
		t.Fatal(err)
	}
	if len(configuration.Webhooks) != 1 || *configuration.Webhooks[0].FailurePolicy != admissionv1.Fail {
		t.Fatalf("expected one fail-closed webhook, got %+v", configuration.Webhooks)
	}
	if string(configuration.Webhooks[0].ClientConfig.CABundle) != "CA" {
		t.Fatalf("webhook has wrong CA bundle: %q", configuration.Webhooks[0].ClientConfig.CABundle)
	}
	if len(configuration.Webhooks[0].MatchConditions) != 1 {
		t.Fatalf("webhook must exclude DNS objects without response rules: %+v", configuration.Webhooks[0])
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName, Namespace: namespace}, deployment); err != nil {
		t.Fatal(err)
	}
	zero := int32(0)
	deployment.Spec.Replicas = &zero
	if err := client.Update(ctx, deployment); err != nil {
		t.Fatal(err)
	}
	if err := r.ensureResponseWebhook(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName, Namespace: namespace}, deployment); err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 1 {
		t.Fatalf("webhook replica count was not restored: %+v", deployment.Spec.Replicas)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookCAName, Namespace: namespace}, ca); err != nil {
		t.Fatal(err)
	}
	ca.Data["service-ca.crt"] = "rotated CA"
	if err := client.Update(ctx, ca); err != nil {
		t.Fatal(err)
	}
	if err := r.ensureResponseWebhook(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName}, configuration); err != nil {
		t.Fatal(err)
	}
	if string(configuration.Webhooks[0].ClientConfig.CABundle) != "rotated CA" {
		t.Fatalf("webhook CA bundle was not rotated: %q", configuration.Webhooks[0].ClientConfig.CABundle)
	}
	if err := r.removeResponseWebhook(ctx); err != nil {
		t.Fatal(err)
	}
	if err := r.removeResponseWebhook(ctx); err != nil {
		t.Fatalf("webhook removal should be idempotent: %v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName}, configuration); err == nil {
		t.Fatal("validating webhook configuration remained after removal")
	}
}

func TestResponseWebhookNotDeployedOnExternalTopology(t *testing.T) {
	const namespace = "openshift-dns-operator"
	ctx := context.Background()
	client := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).
		WithObjects(&configv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
			Status:     configv1.InfrastructureStatus{ControlPlaneTopology: configv1.ExternalTopologyMode},
		}).Build()
	r := &reconciler{Config: operatorconfig.Config{OperatorNamespace: namespace, ResponseWebhookImage: "example.com/dns-operator:webhook"}, client: client}
	if err := r.ensureResponseWebhook(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName, Namespace: namespace}, &corev1.Service{}); err == nil {
		t.Fatal("webhook resources were created on an External control plane topology")
	}
}

func TestResponseWebhookRemovalWaitsForSchema(t *testing.T) {
	ctx := context.Background()
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "dnses.operator.openshift.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
			Name: "v1", Served: true,
			Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
				Properties: map[string]apiextensionsv1.JSONSchemaProps{"spec": {
					Properties: map[string]apiextensionsv1.JSONSchemaProps{"responseRules": {Type: "array"}},
				}},
			}},
		}}},
	}
	configuration := responseWebhookConfiguration("openshift-dns-operator", []byte("CA"))
	client := fake.NewClientBuilder().WithScheme(opclient.GetScheme()).WithObjects(crd, configuration).Build()
	r := &reconciler{Config: operatorconfig.Config{OperatorNamespace: "openshift-dns-operator"}, client: client}
	if err := r.removeResponseWebhook(ctx); err == nil {
		t.Fatal("webhook removal succeeded while DNS schema still accepts response rules")
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName}, configuration); err != nil {
		t.Fatalf("registration was removed before the API stopped accepting response rules: %v", err)
	}
	crd.Spec.Versions[0].Schema = nil
	if err := client.Update(ctx, crd); err != nil {
		t.Fatal(err)
	}
	if err := r.removeResponseWebhook(ctx); err == nil {
		t.Fatal("webhook removal succeeded with an unknown DNS schema")
	}
	crd.Spec.Versions[0].Schema = &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
		Properties: map[string]apiextensionsv1.JSONSchemaProps{"spec": {}},
	}}
	if err := client.Update(ctx, crd); err != nil {
		t.Fatal(err)
	}
	if err := r.removeResponseWebhook(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: responseWebhookName}, configuration); err == nil {
		t.Fatal("registration remained after the API stopped accepting response rules")
	}
}
