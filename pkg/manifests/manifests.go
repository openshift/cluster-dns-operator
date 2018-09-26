package manifests

import (
	"bytes"
	"io"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/yaml"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
)

const (
	DNSNamespace          = "assets/dns/namespace.yaml"
	DNSServiceAccount     = "assets/dns/service-account.yaml"
	DNSClusterRole        = "assets/dns/cluster-role.yaml"
	DNSClusterRoleBinding = "assets/dns/cluster-role-binding.yaml"
	DNSConfigMap          = "assets/dns/configmap.yaml"
	DNSDaemonSet          = "assets/dns/daemonset.yaml"
	DNSService            = "assets/dns/service.yaml"

	OperatorCustomResourceDefinition = "manifests/00-custom-resource-definition.yaml"
	OperatorNamespace                = "manifests/00-namespace.yaml"
	OperatorClusterRole              = "manifests/cluster-role.yaml"
	OperatorClusterRoleBinding       = "manifests/cluster-role-binding.yaml"
	OperatorServiceAccount           = "manifests/service-account.yaml"
	OperatorDeployment               = "manifests/deployment.yaml"
)

func MustAssetReader(asset string) io.Reader {
	return bytes.NewReader(MustAsset(asset))
}

// Factory knows how to create dns-related cluster resources from manifest
// files. It provides a point of control to mutate the static resources with
// provided configuration.
type Factory struct {
}

func NewFactory() *Factory {
	return &Factory{}
}

func (f *Factory) OperatorCustomResourceDefinition() (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	crd, err := NewCustomResourceDefinition(MustAssetReader(OperatorCustomResourceDefinition))
	if err != nil {
		return nil, err
	}
	return crd, nil
}

func (f *Factory) OperatorNamespace() (*corev1.Namespace, error) {
	ns, err := NewNamespace(MustAssetReader(OperatorNamespace))
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (f *Factory) OperatorServiceAccount() (*corev1.ServiceAccount, error) {
	sa, err := NewServiceAccount(MustAssetReader(OperatorServiceAccount))
	if err != nil {
		return nil, err
	}
	return sa, nil
}

func (f *Factory) OperatorClusterRole() (*rbacv1.ClusterRole, error) {
	cr, err := NewClusterRole(MustAssetReader(OperatorClusterRole))
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (f *Factory) OperatorClusterRoleBinding() (*rbacv1.ClusterRoleBinding, error) {
	crb, err := NewClusterRoleBinding(MustAssetReader(OperatorClusterRoleBinding))
	if err != nil {
		return nil, err
	}
	return crb, nil
}

func (f *Factory) OperatorDeployment() (*appsv1.Deployment, error) {
	d, err := NewDeployment(MustAssetReader(OperatorDeployment))
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (f *Factory) DNSNamespace() (*corev1.Namespace, error) {
	ns, err := NewNamespace(MustAssetReader(DNSNamespace))
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (f *Factory) DNSServiceAccount() (*corev1.ServiceAccount, error) {
	sa, err := NewServiceAccount(MustAssetReader(DNSServiceAccount))
	if err != nil {
		return nil, err
	}
	return sa, nil
}

func (f *Factory) DNSClusterRole() (*rbacv1.ClusterRole, error) {
	cr, err := NewClusterRole(MustAssetReader(DNSClusterRole))
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (f *Factory) DNSClusterRoleBinding() (*rbacv1.ClusterRoleBinding, error) {
	crb, err := NewClusterRoleBinding(MustAssetReader(DNSClusterRoleBinding))
	if err != nil {
		return nil, err
	}
	return crb, nil
}

func (f *Factory) DNSConfigMap(dns *dnsv1alpha1.ClusterDNS) (*corev1.ConfigMap, error) {
	cm, err := NewConfigMap(MustAssetReader(DNSConfigMap))
	if err != nil {
		return nil, err
	}

	if dns.Spec.ClusterDomain != nil {
		cm.Data["Corefile"] = strings.Replace(cm.Data["Corefile"], "cluster.local", *dns.Spec.ClusterDomain, -1)
	}
	return cm, nil
}

func (f *Factory) DNSDaemonSet() (*appsv1.DaemonSet, error) {
	ds, err := NewDaemonSet(MustAssetReader(DNSDaemonSet))
	if err != nil {
		return nil, err
	}
	return ds, nil
}

func (f *Factory) DNSService(dns *dnsv1alpha1.ClusterDNS) (*corev1.Service, error) {
	s, err := NewService(MustAssetReader(DNSService))
	if err != nil {
		return nil, err
	}

	if dns.Spec.ClusterIP != nil {
		s.Spec.ClusterIP = *dns.Spec.ClusterIP
	}
	return s, nil
}

func NewServiceAccount(manifest io.Reader) (*corev1.ServiceAccount, error) {
	sa := corev1.ServiceAccount{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&sa); err != nil {
		return nil, err
	}
	return &sa, nil
}

func NewClusterRole(manifest io.Reader) (*rbacv1.ClusterRole, error) {
	cr := rbacv1.ClusterRole{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

func NewClusterRoleBinding(manifest io.Reader) (*rbacv1.ClusterRoleBinding, error) {
	crb := rbacv1.ClusterRoleBinding{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&crb); err != nil {
		return nil, err
	}
	return &crb, nil
}

func NewConfigMap(manifest io.Reader) (*corev1.ConfigMap, error) {
	cm := corev1.ConfigMap{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&cm); err != nil {
		return nil, err
	}
	return &cm, nil
}

func NewDaemonSet(manifest io.Reader) (*appsv1.DaemonSet, error) {
	ds := appsv1.DaemonSet{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&ds); err != nil {
		return nil, err
	}
	return &ds, nil
}

func NewService(manifest io.Reader) (*corev1.Service, error) {
	s := corev1.Service{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func NewNamespace(manifest io.Reader) (*corev1.Namespace, error) {
	ns := corev1.Namespace{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&ns); err != nil {
		return nil, err
	}
	return &ns, nil
}

func NewDeployment(manifest io.Reader) (*appsv1.Deployment, error) {
	o := appsv1.Deployment{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&o); err != nil {
		return nil, err
	}
	return &o, nil
}

func NewClusterDNS(manifest io.Reader) (*dnsv1alpha1.ClusterDNS, error) {
	o := dnsv1alpha1.ClusterDNS{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&o); err != nil {
		return nil, err
	}
	return &o, nil
}

func NewCustomResourceDefinition(manifest io.Reader) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	crd := apiextensionsv1beta1.CustomResourceDefinition{}
	if err := yaml.NewYAMLOrJSONDecoder(manifest, 100).Decode(&crd); err != nil {
		return nil, err
	}
	return &crd, nil
}
