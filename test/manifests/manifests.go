package manifests

import (
	"bytes"
	"io"

	dnsv1alpha1 "github.com/openshift/cluster-dns-operator/pkg/apis/dns/v1alpha1"
	coremanifests "github.com/openshift/cluster-dns-operator/pkg/manifests"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	AppDNSNamespace  = "test/assets/app-dns/namespace.yaml"
	AppDNSDeployment = "test/assets/app-dns/deployment.yaml"
	AppDNSService    = "test/assets/app-dns/service.yaml"

	ClusterDNSCustomResource = "test/assets/cluster-dns-cr.yaml"
)

func MustAssetReader(asset string) io.Reader {
	return bytes.NewReader(MustAsset(asset))
}

// Factory knows how to create dns-related cluster resources from manifest
// files. It provides a point of control to mutate the static resources with
// provided configuration.
type Factory struct {
	*coremanifests.Factory
}

func NewFactory() *Factory {
	return &Factory{
		Factory: coremanifests.NewFactory(),
	}
}

func (f *Factory) AppDNSNamespace() (*corev1.Namespace, error) {
	ns, err := coremanifests.NewNamespace(MustAssetReader(AppDNSNamespace))
	if err != nil {
		return nil, err
	}
	return ns, nil
}

func (f *Factory) AppDNSDeployment() (*appsv1.Deployment, error) {
	d, err := coremanifests.NewDeployment(MustAssetReader(AppDNSDeployment))
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (f *Factory) AppDNSService() (*corev1.Service, error) {
	s, err := coremanifests.NewService(MustAssetReader(AppDNSService))
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (f *Factory) ClusterDNSCustomResource() (*dnsv1alpha1.ClusterDNS, error) {
	ci, err := coremanifests.NewClusterDNS(MustAssetReader(ClusterDNSCustomResource))
	if err != nil {
		return nil, err
	}
	return ci, nil
}
