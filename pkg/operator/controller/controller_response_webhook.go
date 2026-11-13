package controller

import (
	"context"
	"fmt"
	configv1 "github.com/openshift/api/config/v1"
	"reflect"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	responseWebhookName       = "dns-response-rules-webhook"
	responseWebhookSecretName = "dns-response-rules-webhook-cert"
	responseWebhookCAName     = "dns-response-rules-webhook-ca"
)

func (r *reconciler) removeResponseWebhook(ctx context.Context) error {
	acceptsRules, err := r.dnsSchemaAcceptsResponseRules(ctx)
	if err != nil {
		return err
	}
	if acceptsRules {
		return fmt.Errorf("cannot remove response validating webhook while DNS schema still accepts responseRules")
	}
	configuration := &admissionv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: responseWebhookName}}
	if err := r.client.Delete(ctx, configuration); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("remove response validating webhook configuration: %w", err)
	}
	if err := r.client.Get(ctx, types.NamespacedName{Name: responseWebhookName}, configuration); err == nil {
		return fmt.Errorf("waiting for response validating webhook configuration removal")
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("confirm response validating webhook configuration removal: %w", err)
	}
	namespace := r.Config.OperatorNamespace
	for _, object := range []client.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: responseWebhookName, Namespace: namespace}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: responseWebhookName, Namespace: namespace}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: responseWebhookCAName, Namespace: namespace}},
	} {
		if err := r.client.Delete(ctx, object); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("remove response webhook resource %s: %w", object.GetName(), err)
		}
	}
	return nil
}

func (r *reconciler) dnsSchemaAcceptsResponseRules(ctx context.Context) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: "dnses.operator.openshift.io"}, crd); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("read DNS schema before webhook removal: %w", err)
	}
	for _, version := range crd.Spec.Versions {
		if !version.Served {
			continue
		}
		if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
			return true, nil
		}
		spec, found := version.Schema.OpenAPIV3Schema.Properties["spec"]
		if !found {
			return true, nil
		}
		if _, found := spec.Properties["responseRules"]; found {
			return true, nil
		}
	}
	return false, nil
}

func (r *reconciler) ensureResponseWebhook(ctx context.Context) error {
	namespace := r.Config.OperatorNamespace
	if namespace == "" {
		return fmt.Errorf("operator namespace is empty")
	}
	infrastructure := &configv1.Infrastructure{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: "cluster"}, infrastructure); err != nil {
		return fmt.Errorf("get cluster infrastructure for response webhook: %w", err)
	}
	if responseRulesUnsupported(infrastructure) {
		// Response rules are not served on this topology yet, so the
		// webhook is not deployed.
		return nil
	}
	name := types.NamespacedName{Namespace: namespace, Name: responseWebhookName}
	service := &corev1.Service{}
	err := r.client.Get(ctx, name, service)
	if apierrors.IsNotFound(err) {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: responseWebhookName, Namespace: namespace,
				Labels:      map[string]string{"app": responseWebhookName},
				Annotations: map[string]string{"service.beta.openshift.io/serving-cert-secret-name": responseWebhookSecretName},
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": responseWebhookName},
				Ports:    []corev1.ServicePort{{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(9443)}},
			},
		}
		if err := r.client.Create(ctx, service); err != nil {
			return fmt.Errorf("create response webhook service: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get response webhook service: %w", err)
	} else {
		changed := false
		if service.Labels == nil {
			service.Labels = map[string]string{}
		}
		if service.Labels["app"] != responseWebhookName {
			service.Labels["app"] = responseWebhookName
			changed = true
		}
		if service.Annotations == nil {
			service.Annotations = map[string]string{}
		}
		if service.Annotations["service.beta.openshift.io/serving-cert-secret-name"] != responseWebhookSecretName {
			service.Annotations["service.beta.openshift.io/serving-cert-secret-name"] = responseWebhookSecretName
			changed = true
		}
		selector := map[string]string{"app": responseWebhookName}
		ports := []corev1.ServicePort{{Name: "https", Port: 443, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(9443)}}
		if !reflect.DeepEqual(service.Spec.Selector, selector) || !reflect.DeepEqual(service.Spec.Ports, ports) {
			service.Spec.Selector = selector
			service.Spec.Ports = ports
			changed = true
		}
		if changed {
			if err := r.client.Update(ctx, service); err != nil {
				return fmt.Errorf("update response webhook service: %w", err)
			}
		}
	}

	ca := &corev1.ConfigMap{}
	caName := types.NamespacedName{Namespace: namespace, Name: responseWebhookCAName}
	err = r.client.Get(ctx, caName, ca)
	if apierrors.IsNotFound(err) {
		ca = &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: responseWebhookCAName, Namespace: namespace,
			Annotations: map[string]string{"service.beta.openshift.io/inject-cabundle": "true"},
		}}
		if err := r.client.Create(ctx, ca); err != nil {
			return fmt.Errorf("create response webhook CA configmap: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get response webhook CA configmap: %w", err)
	} else if ca.Annotations["service.beta.openshift.io/inject-cabundle"] != "true" {
		if ca.Annotations == nil {
			ca.Annotations = map[string]string{}
		}
		ca.Annotations["service.beta.openshift.io/inject-cabundle"] = "true"
		if err := r.client.Update(ctx, ca); err != nil {
			return fmt.Errorf("update response webhook CA configmap: %w", err)
		}
	}

	if r.Config.ResponseWebhookImage == "" {
		return fmt.Errorf("the response webhook image is not configured; set the RESPONSE_WEBHOOK_IMAGE environment variable")
	}
	// Schedule the webhook like the operator when the operator's own
	// Deployment is in this cluster. On a hosted control plane it is not,
	// and the webhook runs on the guest cluster's worker nodes.
	scheduling := corev1.PodSpec{NodeSelector: map[string]string{"kubernetes.io/os": "linux"}}
	operatorDeployment := &appsv1.Deployment{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "dns-operator"}, operatorDeployment); err == nil {
		scheduling = operatorDeployment.Spec.Template.Spec
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get operator deployment for webhook scheduling: %w", err)
	}
	desired := responseWebhookDeployment(namespace, r.Config.ResponseWebhookImage, scheduling)
	deployment := &appsv1.Deployment{}
	err = r.client.Get(ctx, name, deployment)
	if apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, desired); err != nil {
			return fmt.Errorf("create response webhook deployment: %w", err)
		}
		deployment = desired
	} else if err != nil {
		return fmt.Errorf("get response webhook deployment: %w", err)
	} else {
		changed := false
		if !reflect.DeepEqual(deployment.Spec.Template, desired.Spec.Template) {
			deployment.Spec.Template = desired.Spec.Template
			changed = true
		}
		if !reflect.DeepEqual(deployment.Spec.Replicas, desired.Spec.Replicas) {
			deployment.Spec.Replicas = desired.Spec.Replicas
			changed = true
		}
		if changed {
			if err := r.client.Update(ctx, deployment); err != nil {
				return fmt.Errorf("update response webhook deployment: %w", err)
			}
		}
	}

	secret := &corev1.Secret{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: responseWebhookSecretName}, secret); err != nil {
		return fmt.Errorf("wait for response webhook serving certificate: %w", err)
	}
	if len(secret.Data["tls.crt"]) == 0 || len(secret.Data["tls.key"]) == 0 || len(ca.Data["service-ca.crt"]) == 0 {
		return fmt.Errorf("response webhook serving certificate or CA bundle is not ready")
	}
	if deployment.Status.AvailableReplicas < 1 {
		return fmt.Errorf("response webhook deployment is not ready")
	}

	desiredConfiguration := responseWebhookConfiguration(namespace, []byte(ca.Data["service-ca.crt"]))
	currentConfiguration := &admissionv1.ValidatingWebhookConfiguration{}
	err = r.client.Get(ctx, types.NamespacedName{Name: responseWebhookName}, currentConfiguration)
	if apierrors.IsNotFound(err) {
		if err := r.client.Create(ctx, desiredConfiguration); err != nil {
			return fmt.Errorf("create response validating webhook configuration: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("get response validating webhook configuration: %w", err)
	} else {
		changed := false
		if !reflect.DeepEqual(currentConfiguration.Webhooks, desiredConfiguration.Webhooks) {
			currentConfiguration.Webhooks = desiredConfiguration.Webhooks
			changed = true
		}
		if currentConfiguration.Annotations == nil {
			currentConfiguration.Annotations = map[string]string{}
		}
		if currentConfiguration.Annotations["service.beta.openshift.io/inject-cabundle"] != "true" {
			currentConfiguration.Annotations["service.beta.openshift.io/inject-cabundle"] = "true"
			changed = true
		}
		if changed {
			if err := r.client.Update(ctx, currentConfiguration); err != nil {
				return fmt.Errorf("update response validating webhook configuration: %w", err)
			}
		}
	}
	return nil
}

func responseWebhookDeployment(namespace, image string, pod corev1.PodSpec) *appsv1.Deployment {
	one := int32(1)
	nonRoot := true
	noPrivilege := false
	readOnly := true
	user := int64(1000)
	labels := map[string]string{"app": responseWebhookName}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: responseWebhookName, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: &one,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: responseWebhookName,
					NodeSelector:       pod.NodeSelector,
					Tolerations:        pod.Tolerations,
					PriorityClassName:  pod.PriorityClassName,
					SecurityContext:    &corev1.PodSecurityContext{RunAsNonRoot: &nonRoot, RunAsUser: &user, RunAsGroup: &user, FSGroup: &user},
					Containers: []corev1.Container{{
						Name:    "webhook",
						Image:   image,
						Command: []string{"dns-operator"},
						Args:    []string{"--response-webhook-only", "--response-webhook-addr=:9443"},
						Ports:   []corev1.ContainerPort{{Name: "https", ContainerPort: 9443}},
						Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("32Mi"),
						}},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &noPrivilege, ReadOnlyRootFilesystem: &readOnly,
							Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
						VolumeMounts: []corev1.VolumeMount{{Name: "tls", MountPath: "/etc/tls/webhook", ReadOnly: true}},
						ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz", Port: intstr.FromInt32(9443), Scheme: corev1.URISchemeHTTPS,
						}}},
					}},
					Volumes: []corev1.Volume{{Name: "tls", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
						SecretName: responseWebhookSecretName,
					}}}},
				},
			},
		},
	}
}

func responseWebhookConfiguration(namespace string, caBundle []byte) *admissionv1.ValidatingWebhookConfiguration {
	fail := admissionv1.Fail
	none := admissionv1.SideEffectClassNone
	equivalent := admissionv1.Equivalent
	timeout := int32(5)
	port := int32(443)
	path := "/validate"
	return &admissionv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:        responseWebhookName,
			Annotations: map[string]string{"service.beta.openshift.io/inject-cabundle": "true"},
		},
		Webhooks: []admissionv1.ValidatingWebhook{{
			Name:                    "response-rules.dns.operator.openshift.io",
			AdmissionReviewVersions: []string{"v1"},
			SideEffects:             &none,
			FailurePolicy:           &fail,
			MatchPolicy:             &equivalent,
			TimeoutSeconds:          &timeout,
			ClientConfig: admissionv1.WebhookClientConfig{
				CABundle: caBundle,
				Service: &admissionv1.ServiceReference{
					Name: responseWebhookName, Namespace: namespace, Path: &path, Port: &port,
				},
			},
			Rules: []admissionv1.RuleWithOperations{{
				Operations: []admissionv1.OperationType{admissionv1.Create, admissionv1.Update},
				Rule: admissionv1.Rule{
					APIGroups: []string{"operator.openshift.io"}, APIVersions: []string{"v1"},
					Resources: []string{"dnses"}, Scope: scopePtr(admissionv1.ClusterScope),
				},
			}},
			MatchConditions: []admissionv1.MatchCondition{{
				Name:       "has-response-rules",
				Expression: "object.metadata.name == 'default' && has(object.spec) && has(object.spec.responseRules) && size(object.spec.responseRules) > 0",
			}},
		}},
	}
}

func scopePtr(scope admissionv1.ScopeType) *admissionv1.ScopeType { return &scope }
