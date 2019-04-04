// +build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	operatorclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

func getClient() (client.Client, error) {
	// Get a kube client.
	kubeConfig, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kube config: %v", err)
	}
	kubeClient, err := operatorclient.NewClient(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kube client: %v", err)
	}
	return kubeClient, nil
}

func TestOperatorAvailable(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		co := &configv1.ClusterOperator{}
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: operatorcontroller.DNSClusterOperatorName}, co); err != nil {
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
		t.Errorf("did not get expected available condition: %v", err)
	}
}

func TestDefaultClusterDNSExists(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		dns := &operatorv1.DNS{}
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: "default"}, dns); err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to get default cluster dns: %v", err)
	}
}

func TestVersionReporting(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	deployment := &appsv1.Deployment{}
	namespacedName := types.NamespacedName{Namespace: "openshift-dns-operator", Name: "dns-operator"}
	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		if err := cl.Get(context.TODO(), namespacedName, deployment); err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to get deployment: %v", err)
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
	setVersion(deployment, newVersion)
	if err := cl.Update(context.TODO(), deployment); err != nil {
		t.Fatalf("failed to update dns operator to new version: %v", err)
	}
	defer func() {
		if err := cl.Get(context.TODO(), namespacedName, deployment); err != nil {
			t.Fatalf("failed to get latest deployment: %v", err)
		}
		setVersion(deployment, curVersion)
		if err := cl.Update(context.TODO(), deployment); err != nil {
			t.Fatalf("failed to restore dns operator to old release version: %v", err)
		}
	}()

	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		co := &configv1.ClusterOperator{}
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: operatorcontroller.DNSClusterOperatorName}, co); err != nil {
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
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	deployment := &appsv1.Deployment{}
	namespacedName := types.NamespacedName{Namespace: "openshift-dns-operator", Name: "dns-operator"}
	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		if err := cl.Get(context.TODO(), namespacedName, deployment); err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to get deployment: %v", err)
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
	setImage(deployment, newImage)
	if err := cl.Update(context.TODO(), deployment); err != nil {
		t.Fatalf("failed to update dns operator to new coredns image: %v", err)
	}
	defer func() {
		if err := cl.Get(context.TODO(), namespacedName, deployment); err != nil {
			t.Fatalf("failed to get latest deployment: %v", err)
		}
		setImage(deployment, curImage)
		if err := cl.Update(context.TODO(), deployment); err != nil {
			t.Fatalf("failed to restore dns operator to old coredns image: %v", err)
		}
	}()

	err = wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		podList := &corev1.PodList{}
		if err := cl.List(context.TODO(), podList, client.InNamespace("openshift-dns")); err != nil {
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
		t.Errorf("failed to observe updated coredns image: %v", err)
	}
}

func setVersion(deployment *appsv1.Deployment, version string) {
	for i, env := range deployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "RELEASE_VERSION" {
			deployment.Spec.Template.Spec.Containers[0].Env[i].Value = version
			break
		}
	}
}

func setImage(deployment *appsv1.Deployment, image string) {
	for i, env := range deployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "IMAGE" {
			deployment.Spec.Template.Spec.Containers[0].Env[i].Value = image
			break
		}
	}
}
