package manifests

import (
	"bytes"
	"io"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"

	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	DNSNamespaceAsset          = "assets/dns/namespace.yaml"
	DNSServiceAccountAsset     = "assets/dns/service-account.yaml"
	DNSClusterRoleAsset        = "assets/dns/cluster-role.yaml"
	DNSClusterRoleBindingAsset = "assets/dns/cluster-role-binding.yaml"
	DNSConfigMapAsset          = "assets/dns/configmap.yaml"
	DNSDaemonSetAsset          = "assets/dns/daemonset.yaml"
	DNSServiceAsset            = "assets/dns/service.yaml"

	// OwningDNSLabel should be applied to any objects "owned by" a
	// dns to aid in selection (especially in cases where an ownerref
	// can't be established due to namespace boundaries).
	OwningDNSLabel = "dns.operator.openshift.io/cluster-dns"
)

func MustAssetReader(asset string) io.Reader {
	return bytes.NewReader(MustAsset(asset))
}

func DNSNamespace() *corev1.Namespace {
	ns, err := NewNamespace(MustAssetReader(DNSNamespaceAsset))
	if err != nil {
		panic(err)
	}
	return ns
}

func DNSServiceAccount() *corev1.ServiceAccount {
	sa, err := NewServiceAccount(MustAssetReader(DNSServiceAccountAsset))
	if err != nil {
		panic(err)
	}
	return sa
}

func DNSClusterRole() *rbacv1.ClusterRole {
	cr, err := NewClusterRole(MustAssetReader(DNSClusterRoleAsset))
	if err != nil {
		panic(err)
	}
	return cr
}

func DNSClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	crb, err := NewClusterRoleBinding(MustAssetReader(DNSClusterRoleBindingAsset))
	if err != nil {
		panic(err)
	}
	return crb
}

func DNSConfigMap() *corev1.ConfigMap {
	cm, err := NewConfigMap(MustAssetReader(DNSConfigMapAsset))
	if err != nil {
		panic(err)
	}
	return cm
}

func DNSDaemonSet() *appsv1.DaemonSet {
	ds, err := NewDaemonSet(MustAssetReader(DNSDaemonSetAsset))
	if err != nil {
		panic(err)
	}
	return ds
}

func DNSService() *corev1.Service {
	s, err := NewService(MustAssetReader(DNSServiceAsset))
	if err != nil {
		panic(err)
	}
	return s
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
