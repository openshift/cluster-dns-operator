// +build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	"github.com/operator-framework/operator-sdk/pkg/sdk"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestOperatorAvailable(t *testing.T) {
	var co *configv1.ClusterOperator
	err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		co = &configv1.ClusterOperator{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterOperator",
				APIVersion: "config.openshift.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "dns",
			},
		}
		if err := sdk.Get(co); err != nil {
			return false, nil
		}

		for _, cond := range co.Status.Conditions {
			if cond.Type == configv1.OperatorAvailable &&
				cond.Status == configv1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("did not get expected available condition for ClusterOperator %s: %v", co.Name, err)
	}
}

func TestDefaultClusterDNSExists(t *testing.T) {
	var dns *operatorv1.DNS
	err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		dns = &operatorv1.DNS{
			TypeMeta: metav1.TypeMeta{
				Kind:       "DNS",
				APIVersion: "operator.openshift.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
		}
		if err := sdk.Get(dns); err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to get default ClusterDNS: %v", err)
	}
}

func TestVersionReporting(t *testing.T) {
	var deployment *appsv1.Deployment
	err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		deployment = &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dns-operator",
				Namespace: "openshift-dns-operator",
			},
		}
		if err := sdk.Get(deployment); err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to get deployment %s/%s: %v", deployment.Namespace, deployment.Name, err)
	}

	var curVersion string
	for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "RELEASE_VERSION" {
			curVersion = env.Value
			break
		}
	}
	if len(curVersion) == 0 {
		t.Errorf("env RELEASE_VERSION not found in the operator deployment")
	}

	newVersion := "0.0.1-test"
	patch := []byte(fmt.Sprintf(`{"spec": {"template": {"spec": {"containers": [{"name":"dns-operator","env":[{"name":"RELEASE_VERSION","value":"%s"}]}]}}}}`, newVersion))
	if err := sdk.Patch(deployment, types.StrategicMergePatchType, patch); err != nil {
		t.Fatalf("failed to patch dns operator to new version: %v", err)
	}
	defer func() {
		patch := []byte(fmt.Sprintf(`{"spec": {"template": {"spec": {"containers": [{"name":"dns-operator","env":[{"name":"RELEASE_VERSION","value":"%s"}]}]}}}}`, curVersion))
		if err := sdk.Patch(deployment, types.StrategicMergePatchType, patch); err != nil {
			t.Fatalf("failed to restore dns operator to old release version: %v", err)
		}
	}()

	err = wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		co := &configv1.ClusterOperator{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ClusterOperator",
				APIVersion: "config.openshift.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "dns",
			},
		}
		if err := sdk.Get(co); err != nil {
			return false, nil
		}

		for _, v := range co.Status.Versions {
			if v.Name == "operator" {
				if v.Version == newVersion {
					return true, nil
				}
				break
			}
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("failed to observe updated version reported in dns clusteroperator status: %v", err)
	}
}

func TestCoreDNSImageUpgrade(t *testing.T) {
	var deployment *appsv1.Deployment
	err := wait.PollImmediate(1*time.Second, 10*time.Minute, func() (bool, error) {
		deployment = &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Deployment",
				APIVersion: "apps/v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dns-operator",
				Namespace: "openshift-dns-operator",
			},
		}
		if err := sdk.Get(deployment); err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to get deployment %s/%s: %v", deployment.Namespace, deployment.Name, err)
	}

	var curImage string
	for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "IMAGE" {
			curImage = env.Value
			break
		}
	}
	if len(curImage) == 0 {
		t.Errorf("env IMAGE not found in the operator deployment")
	}

	newImage := "openshift/origin-coredns:latest"
	patch := []byte(fmt.Sprintf(`{"spec": {"template": {"spec": {"containers": [{"name":"dns-operator","env":[{"name":"IMAGE","value":"%s"}]}]}}}}`, newImage))
	if err := sdk.Patch(deployment, types.StrategicMergePatchType, patch); err != nil {
		t.Fatalf("failed to patch dns operator to new coredns image: %v", err)
	}
	defer func() {
		patch := []byte(fmt.Sprintf(`{"spec": {"template": {"spec": {"containers": [{"name":"dns-operator","env":[{"name":"IMAGE","value":"%s"}]}]}}}}`, curImage))
		if err := sdk.Patch(deployment, types.StrategicMergePatchType, patch); err != nil {
			t.Fatalf("failed to restore dns operator to old coredns image: %v", err)
		}
	}()

	err = wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		podList := &corev1.PodList{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: "v1",
			},
		}
		if err := sdk.List("openshift-dns", podList, sdk.WithListOptions(&metav1.ListOptions{})); err != nil {
			return false, nil
		}

		for _, pod := range podList.Items {
			for _, container := range pod.Spec.Containers {
				if container.Name == "dns" {
					if container.Image == newImage {
						return true, nil
					}
					break
				}
			}
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("failed to observe updated coreDNS image: %v", err)
	}
}
