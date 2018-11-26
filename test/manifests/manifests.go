package manifests

import (
	coremanifests "github.com/openshift/cluster-dns-operator/pkg/manifests"
	"github.com/openshift/cluster-dns-operator/pkg/operator"
)

// Factory knows how to create dns-related cluster resources from manifest
// files. It provides a point of control to mutate the static resources with
// provided configuration.
type Factory struct {
	*coremanifests.Factory
}

func NewFactory(config operator.Config) *Factory {
	return &Factory{
		Factory: coremanifests.NewFactory(config),
	}
}
