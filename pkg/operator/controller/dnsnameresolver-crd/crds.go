package dnsnameresolvercrd

import (
	"context"
	"fmt"

	"github.com/openshift/cluster-dns-operator/pkg/manifests"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	configv1 "github.com/openshift/api/config/v1"
	"github.com/sirupsen/logrus"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// ensureCRD attempts to ensure that the specified CRD exists and returns a
// Boolean indicating whether it exists, the CRD if it does exist, and an error
// value.
func (r *reconciler) ensureCRD(ctx context.Context, desired *apiextensionsv1.CustomResourceDefinition) (bool, *apiextensionsv1.CustomResourceDefinition, error) {
	name := types.NamespacedName{
		Namespace: desired.Namespace,
		Name:      desired.Name,
	}
	have, current, err := r.currentCRD(ctx, name)
	if err != nil {
		return have, current, err
	}

	switch {
	case !have:
		if err := r.createCRD(ctx, desired); err != nil {
			return false, nil, err
		}
		return r.currentCRD(ctx, name)
	case have:
		if updated, err := r.updateCRD(ctx, current, desired); err != nil {
			return have, current, err
		} else if updated {
			return r.currentCRD(ctx, name)
		}
	}
	return have, current, nil
}

// ensureDNSNameResolverCRD ensures the managed DNSNameResolver CRD is created.
// If TechPreviewNoUpgrade feature set is enabled then the tech preview no upgrade CRD is created.
// If CustomNoUpgrade feature set is enabled then the custom no upgrade CRD is created.
// Otherwise an error is returned.
func (r *reconciler) ensureDNSNameResolverCRD(ctx context.Context, featureSet configv1.FeatureSet) error {
	var crd *apiextensionsv1.CustomResourceDefinition
	if featureSet == configv1.TechPreviewNoUpgrade {
		// The controller-runtime client mutates its argument, so give
		// it a copy of the CRD rather than the original.
		crd = manifests.DNSNameResolverTechPreviewNoUpgradeCRD().DeepCopy()
	} else if featureSet == configv1.CustomNoUpgrade {
		// The controller-runtime client mutates its argument, so give
		// it a copy of the CRD rather than the original.
		crd = manifests.DNSNameResolverCustomNoUpgradeCRD().DeepCopy()
	} else {
		return fmt.Errorf("no matching crd for the feature set: %s", featureSet)
	}
	_, _, err := r.ensureCRD(ctx, crd)
	return err
}

// currentCRD returns a Boolean indicating whether an CRD
// exists for the IngressController with the given name, as well as the
// CRD if it does exist and an error value.
func (r *reconciler) currentCRD(ctx context.Context, name types.NamespacedName) (bool, *apiextensionsv1.CustomResourceDefinition, error) {
	var crd apiextensionsv1.CustomResourceDefinition
	if err := r.client.Get(ctx, name, &crd); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("failed to get CRD %s: %w", name, err)
	}
	return true, &crd, nil
}

// createCRD attempts to create the specified CRD and returns an error value.
func (r *reconciler) createCRD(ctx context.Context, desired *apiextensionsv1.CustomResourceDefinition) error {
	if err := r.client.Create(ctx, desired); err != nil {
		return fmt.Errorf("failed to create CRD %s: %w", desired.Name, err)
	}

	logrus.Info("created CRD", "name", desired.Name)

	return nil
}

// updateCRD updates a CRD.  Returns a Boolean indicating
// whether the CRD was updated, and an error value.
func (r *reconciler) updateCRD(ctx context.Context, current, desired *apiextensionsv1.CustomResourceDefinition) (bool, error) {
	changed, updated := crdChanged(current, desired)
	if !changed {
		return false, nil
	}

	// Diff before updating because the client may mutate the object.
	diff := cmp.Diff(current, updated, cmpopts.EquateEmpty())
	if err := r.client.Update(ctx, updated); err != nil {
		return false, fmt.Errorf("failed to update CRD %s: %w", updated.Name, err)
	}
	logrus.Info("updated CRD", "name", updated.Name, "diff", diff)
	return true, nil
}

// crdChanged checks if the current CRD spec matches
// the expected spec and if not returns an updated one.
func crdChanged(current, expected *apiextensionsv1.CustomResourceDefinition) (bool, *apiextensionsv1.CustomResourceDefinition) {
	crdCmpOpts := []cmp.Option{
		// Ignore fields that the API may have modified.
		cmpopts.IgnoreFields(apiextensionsv1.CustomResourceDefinitionSpec{}, "Conversion"),
		cmpopts.EquateEmpty(),
	}
	if cmp.Equal(current.Spec, expected.Spec, crdCmpOpts...) {
		return false, nil
	}

	updated := current.DeepCopy()
	updated.Spec = expected.Spec

	return true, updated
}
