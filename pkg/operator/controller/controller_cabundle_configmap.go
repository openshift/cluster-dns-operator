package controller

import (
	"context"
	"fmt"
	"reflect"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// cmLabels is the labels that the operator applies to the CA bundle
	// configmaps that it creates so that it can later select them.
	cmLabels = map[string]string{
		"dns.operator.openshift.io/ca-bundle": "true",
	}
	// cmLabelSelector is the label selector that the operator uses to identify
	// CA bundle configmaps that it owns.
	cmLabelSelector = metav1.LabelSelector{
		MatchLabels: cmLabels,
	}
	// cmSelector is a labels.Selector built from cmLabelSelector.
	cmSelector = func() labels.Selector {
		v, err := metav1.LabelSelectorAsSelector(&cmLabelSelector)
		if err != nil {
			panic(err)
		}
		return v
	}()
)

// ensureCABundleConfigMaps syncs CA bundle configmaps for a DNS
// between the openshift-config and openshift-dns namespaces if the user has
// configured a CA bundle configmap. While syncing the configmaps, ca- is
// prepended to the name of the configmap in openshift-dns namespace
// to make it understandable that it is a CA bundle.
func (r *reconciler) ensureCABundleConfigMaps(dns *operatorv1.DNS) error {
	var configmapNames []string
	transportConfig := dns.Spec.UpstreamResolvers.TransportConfig
	if transportConfig.Transport == operatorv1.TLSTransport && transportConfig.TLS != nil && transportConfig.TLS.CABundle.Name != "" {
		configmapNames = append(configmapNames, transportConfig.TLS.CABundle.Name)
	}
	for _, server := range dns.Spec.Servers {
		transportConfig := server.ForwardPlugin.TransportConfig
		if transportConfig.Transport == operatorv1.TLSTransport && transportConfig.TLS != nil && transportConfig.TLS.CABundle.Name != "" {
			configmapNames = append(configmapNames, transportConfig.TLS.CABundle.Name)
		}
	}

	var errs []error
	for _, name := range configmapNames {
		sourceName := types.NamespacedName{
			Namespace: GlobalUserSpecifiedConfigNamespace,
			Name:      name,
		}
		haveSource, source, err := r.currentCABundleConfigMap(sourceName)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get source ca bundle configmap %s: %w", sourceName.Name, err))
			continue
		}
		if !haveSource {
			logrus.Warningf("source ca bundle configmap %s does not exist", sourceName.Name)
			continue
		}

		destName := CABundleConfigMapName(source.Name)
		have, current, err := r.currentCABundleConfigMap(destName)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get destination ca bundle configmap %s: %w", destName.Name, err))
			continue
		}

		want, desired, err := desiredCABundleConfigMap(dns, haveSource, source, destName)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to generate desired ca bundle config map: %w", err))
			continue
		}

		switch {
		case !want && !have:
			continue
		case !want && have:
			if err := r.client.Delete(context.TODO(), current); err != nil {
				if !errors.IsNotFound(err) {
					errs = append(errs, fmt.Errorf("failed to delete configmap: %w", err))
				}
			} else {
				logrus.Infof("deleted configmap %s/%s", current.Namespace, current.Name)
			}
		case want && !have:
			if err := r.client.Create(context.TODO(), desired); err != nil {
				errs = append(errs, fmt.Errorf("failed to create configmap: %w", err))
			} else {
				logrus.Infof("created configmap %s/%s", desired.Namespace, desired.Name)
			}
		case want && have:
			if updated, err := r.updateCABundleConfigMap(current, desired); err != nil {
				errs = append(errs, fmt.Errorf("failed to update configmap: %w", err))
			} else if updated {
				logrus.Infof("updated configmap %s/%s", desired.Namespace, desired.Name)
			}
		}
	}

	// remove ca bundle configmaps that are not referred in dns anymore.
	cmListOpts := []client.ListOption{
		client.MatchingLabelsSelector{
			Selector: cmSelector,
		},
		client.InNamespace(DefaultOperandNamespace),
	}
	var cmList corev1.ConfigMapList
	if err := r.cache.List(context.TODO(), &cmList, cmListOpts...); err != nil {
		errs = append(errs, fmt.Errorf("failed to list ca bundle configmaps: %w", err))
	}
	for _, cm := range cmList.Items {
		referredInDNS := false
		for _, name := range configmapNames {
			destName := CABundleConfigMapName(name)
			if cm.Name == destName.Name {
				referredInDNS = true
				break
			}
		}
		if !referredInDNS {
			if err := r.client.Delete(context.TODO(), &cm); err != nil {
				if !errors.IsNotFound(err) {
					errs = append(errs, fmt.Errorf("failed to delete configmap: %w", err))
				}
			} else {
				logrus.Infof("deleted configmap %s/%s", cm.Namespace, cm.Name)
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

// desiredCABundleConfigMap returns the desired CA bundle configmap.  Returns a
// Boolean indicating whether a configmap is desired, as well as the configmap
// if one is desired.
func desiredCABundleConfigMap(dns *operatorv1.DNS, haveSource bool, sourceConfigmap *corev1.ConfigMap, name types.NamespacedName) (bool, *corev1.ConfigMap, error) {
	if !haveSource {
		return false, nil, nil
	}
	if dns.DeletionTimestamp != nil {
		return false, nil, nil
	}
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			Labels:    cmLabels,
		},
		Data: sourceConfigmap.Data,
	}
	cm.SetOwnerReferences([]metav1.OwnerReference{dnsOwnerRef(dns)})

	return true, &cm, nil
}

// currentCABundleConfigMap returns the current configmap.  Returns a Boolean
// indicating whether the configmap existed, the configmap if it did exist, and
// an error value.
func (r *reconciler) currentCABundleConfigMap(name types.NamespacedName) (bool, *corev1.ConfigMap, error) {
	if len(name.Name) == 0 {
		return false, nil, nil
	}
	cm := &corev1.ConfigMap{}
	if err := r.client.Get(context.TODO(), name, cm); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, cm, nil
}

// updateCABundleConfigMap updates a configmap.  Returns a Boolean indicating
// whether the configmap was updated, and an error value.
func (r *reconciler) updateCABundleConfigMap(current, desired *corev1.ConfigMap) (bool, error) {
	if caBundleConfigmapsEqual(current, desired) {
		return false, nil
	}
	updated := current.DeepCopy()
	updated.Data = desired.Data
	if err := r.client.Update(context.TODO(), updated); err != nil {
		return false, err
	}
	return true, nil
}

// caBundleConfigmapsEqual compares two CA bundle configmaps.  Returns true if
// the configmaps should be considered equal for the purpose of determining
// whether an update is necessary, false otherwise.
func caBundleConfigmapsEqual(a, b *corev1.ConfigMap) bool {
	return reflect.DeepEqual(a.Data, b.Data)
}
