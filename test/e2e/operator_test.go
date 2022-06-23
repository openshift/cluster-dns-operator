//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"

	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"
	statuscontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller/status"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// upstreamPodName is the name of the upstream CoreDNS server
	// used for testing DNS forwarding.
	upstreamPodName = "test-upstream"
	// upstreamPodNs is the namespace of the upstream CoreDNS server
	// used for testing DNS forwarding.
	upstreamPodNs = "openshift-dns"
	// upstreamCorefile is the Corefile used by the upstream CoreDNS server
	// used for testing DNS forwarding.
	upstreamCorefile = `.:5353 {
    hosts {
      1.2.3.4 www.foo.com
    }
    health
    errors
    log
}
`
	// upstreamCorefile is the Corefile used by the upstream CoreDNS server
	// used for testing DNS forwarding.
	upstreamTLSCorefile = `tls://.:5353 {
    hosts {
      4.3.2.1 www.tls.com
    }
	tls /etc/coredns/cert /etc/coredns/key
    health
    errors
    log
}
`
)

var (
	dnsName = types.NamespacedName{Name: operatorcontroller.DefaultDNSName}
	opName  = types.NamespacedName{Name: operatorcontroller.DefaultOperatorName}

	defaultAvailableDNSConditions = []operatorv1.OperatorCondition{
		{Type: operatorv1.OperatorStatusTypeAvailable, Status: operatorv1.ConditionTrue},
		{Type: operatorv1.OperatorStatusTypeProgressing, Status: operatorv1.ConditionFalse},
		{Type: operatorv1.OperatorStatusTypeDegraded, Status: operatorv1.ConditionFalse},
	}
)

func TestOperatorAvailable(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		co := &configv1.ClusterOperator{}
		if err := cl.Get(context.TODO(), opName, co); err != nil {
			t.Logf("failed to get DNS cluster operator %s: %v", opName.Name, err)
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

// TestClusterOperatorStatusRelatedObjects verifies that the dns clusteroperator
// reports the expected related objects.
func TestClusterOperatorStatusRelatedObjects(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	expected := []configv1.ObjectReference{
		{
			Resource: "namespaces",
			Name:     "openshift-dns-operator",
		},
		{
			Group:    operatorv1.GroupName,
			Resource: "dnses",
			Name:     "default",
		},
		{
			Resource: "namespaces",
			Name:     "openshift-dns",
		},
	}
	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		co := &configv1.ClusterOperator{}
		if err := cl.Get(context.TODO(), opName, co); err != nil {
			t.Logf("failed to get clusteroperator %q: %v", opName.Name, err)
			return false, nil
		}

		if !reflect.DeepEqual(expected, co.Status.RelatedObjects) {
			t.Logf("did not observe expected status.relatedObjects for clusteroperator %q: expected %+v, got %+v", opName.Name, expected, co.Status.RelatedObjects)
			return false, nil
		}

		return true, nil
	})
	if err != nil {
		t.Errorf("did not get expected status related objects: %v", err)
	}
}

func TestDefaultDNSExists(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		dns := &operatorv1.DNS{}
		if err := cl.Get(context.TODO(), dnsName, dns); err != nil {
			t.Logf("failed to get DNS operator %s: %v", dnsName.Name, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to get default dns: %v", err)
	}
}

func TestOperatorSteadyConditions(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}
	expected := []configv1.ClusterOperatorStatusCondition{
		{Type: configv1.OperatorAvailable, Status: configv1.ConditionTrue},
	}
	if err := waitForClusterOperatorConditions(t, cl, 10*time.Second, expected...); err != nil {
		t.Errorf("did not get expected available condition: %v", err)
	}
}

func TestDefaultDNSSteadyConditions(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}
	if err := waitForDNSConditions(t, cl, 10*time.Second, dnsName, defaultAvailableDNSConditions...); err != nil {
		t.Errorf("did not get expected conditions: %v", err)
	}
}

// TestCoreDNSDaemonSetReconciliation verifies that the operator reconciles the
// dns-default daemonset.  The test modifies the daemonset and verifies that the
// operator reverts the change.
func TestCoreDNSDaemonSetReconciliation(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	defaultDNS := &operatorv1.DNS{}
	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: operatorcontroller.DefaultDNSController}, defaultDNS); err != nil {
			t.Logf("failed to get dns %q: %v", operatorcontroller.DefaultDNSController, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("failed to get dns %q: %v", operatorcontroller.DefaultDNSController, err)
	}

	newNodeSelector := "foo"
	namespacedName := operatorcontroller.DNSDaemonSetName(defaultDNS)
	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		dnsDaemonSet := &appsv1.DaemonSet{}
		if err := cl.Get(context.TODO(), namespacedName, dnsDaemonSet); err != nil {
			t.Logf("failed to get daemonset %s: %v", namespacedName, err)
			return false, nil
		}
		dnsDaemonSet.Spec.Template.Spec.NodeSelector[newNodeSelector] = ""
		if err := cl.Update(context.TODO(), dnsDaemonSet); err != nil {
			t.Logf("failed to update daemonset %s: %v", namespacedName, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to update daemonset %s: %v", namespacedName, err)
	}

	err = wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		dnsDaemonSet := &appsv1.DaemonSet{}
		if err := cl.Get(context.TODO(), namespacedName, dnsDaemonSet); err != nil {
			t.Logf("failed to get daemonset %s: %v", namespacedName, err)
			return false, nil
		}
		for k := range dnsDaemonSet.Spec.Template.Spec.NodeSelector {
			if k == newNodeSelector {
				t.Logf("found %q node selector on daemonset %s: %v", newNodeSelector, namespacedName, err)
				return false, nil
			}
		}
		t.Logf("observed absence of %q node selector on daemonset %s: %v", newNodeSelector, namespacedName, err)
		return true, nil
	})
	if err != nil {
		t.Errorf("failed to observe reversion of update to daemonset %s: %v", namespacedName, err)
	}
}

// TestOperatorRecreatesItsClusterOperator verifies that the DNS operator
// recreates the "dns" clusteroperator if the clusteroperator is deleted.
func TestOperatorRecreatesItsClusterOperator(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	co := &configv1.ClusterOperator{}
	if err := cl.Get(context.TODO(), opName, co); err != nil {
		t.Fatalf("failed to get clusteroperator %q: %v", opName.Name, err)
	}
	if err := cl.Delete(context.TODO(), co); err != nil {
		t.Fatalf("failed to delete clusteroperator %q: %v", opName.Name, err)
	}
	err = wait.PollImmediate(1*time.Second, 30*time.Second, func() (bool, error) {
		if err := cl.Get(context.TODO(), opName, co); err != nil {
			t.Logf("failed to get clusteroperator %q: %v", opName.Name, err)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("failed to observe recreation of clusteroperator %q: %v", opName.Name, err)
	}
}

func TestDNSForwarding(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	// Create the upstream resolver ConfigMap.
	upstreamCfgMap := buildConfigMap(upstreamPodName, upstreamPodNs, "Corefile", upstreamCorefile)
	if err := cl.Create(context.TODO(), upstreamCfgMap); err != nil {
		t.Fatalf("failed to create configmap %s/%s: %v", upstreamCfgMap.Namespace, upstreamCfgMap.Name, err)
	}
	defer func() {
		if err := cl.Delete(context.TODO(), upstreamCfgMap); err != nil {
			t.Fatalf("failed to delete configmap %s/%s: %v", upstreamCfgMap.Namespace, upstreamCfgMap.Name, err)
		}
	}()

	// Get the CoreDNS image used by the test upstream resolver.
	co := &configv1.ClusterOperator{}
	if err := cl.Get(context.TODO(), opName, co); err != nil {
		t.Fatalf("failed to get clusteroperator %s: %v", opName, err)
	}
	var (
		coreImage      string
		coreImageFound bool
	)
	for _, ver := range co.Status.Versions {
		if ver.Name == statuscontroller.CoreDNSVersionName {
			if len(ver.Version) == 0 {
				t.Fatalf("clusteroperator %s has empty coredns version", opName)
			}
			coreImageFound = true
			coreImage = ver.Version
			break
		}
	}
	if !coreImageFound {
		t.Fatalf("version %s not found for clusteroperator %s", statuscontroller.CoreDNSVersionName, opName)
	}

	// Create the upstream resolver Pod.
	upstreamResolver := upstreamPod(upstreamPodName, upstreamPodNs, coreImage, upstreamPodName)
	if err := cl.Create(context.TODO(), upstreamResolver); err != nil {
		t.Fatalf("failed to create pod %s/%s: %v", upstreamResolver.Namespace, upstreamResolver.Name, err)
	}
	defer func() {
		if err := cl.Delete(context.TODO(), upstreamResolver); err != nil {
			t.Fatalf("failed to delete pod %s/%s: %v", upstreamResolver.Namespace, upstreamResolver.Name, err)
		}
	}()

	// Wait for the upstream resolver Pod to be ready.
	name := types.NamespacedName{Namespace: upstreamResolver.Namespace, Name: upstreamResolver.Name}
	err = wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
		if err := cl.Get(context.TODO(), name, upstreamResolver); err != nil {
			t.Logf("failed to get pod %s/%s: %v", name.Namespace, name.Name, err)
			return false, nil
		}
		for _, cond := range upstreamResolver.Status.Conditions {
			if cond.Type == corev1.ContainersReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to observe ContainersReady condition for pod %s/%s: %v", upstreamResolver.Namespace, upstreamResolver.Name, err)
	}

	// Create the upstream resolver Service and get the ClusterIP.
	upstreamSvc := upstreamService(upstreamPodName, upstreamPodNs)
	if err := cl.Create(context.TODO(), upstreamSvc); err != nil {
		t.Fatalf("failed to create service %s/%s: %v", upstreamSvc.Namespace, upstreamSvc.Name, err)
	}
	defer func() {
		if err := cl.Delete(context.TODO(), upstreamSvc); err != nil {
			t.Fatalf("failed to delete service %s/%s: %v", upstreamSvc.Namespace, upstreamSvc.Name, err)
		}
	}()
	if err := cl.Get(context.TODO(), types.NamespacedName{Namespace: upstreamSvc.Namespace, Name: upstreamSvc.Name}, upstreamSvc); err != nil {
		t.Fatalf("failed to get service %s/%s: %v", upstreamSvc.Namespace, upstreamSvc.Name, err)
	}
	upstreamIP := upstreamSvc.Spec.ClusterIP
	if len(upstreamIP) == 0 {
		t.Fatalf("failed to get clusterIP for service %s/%s", upstreamSvc.Namespace, upstreamSvc.Name)
	}

	// Update cluster DNS forwarding with the upstream resolver's Service IP address.
	defaultDNS := &operatorv1.DNS{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Name: operatorcontroller.DefaultDNSController}, defaultDNS); err != nil {
		t.Fatalf("failed to get default dns: %v", err)
	}
	upstream := operatorv1.Server{
		Name:  "test",
		Zones: []string{"foo.com"},
		ForwardPlugin: operatorv1.ForwardPlugin{
			Upstreams: []string{upstreamIP},
		},
	}
	defaultDNS.Spec.Servers = []operatorv1.Server{upstream}
	if err := cl.Update(context.TODO(), defaultDNS); err != nil {
		t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
	}
	defer func() {
		defaultDNS = &operatorv1.DNS{}
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: "default"}, defaultDNS); err != nil {
			t.Fatalf("failed to get default dns: %v", err)
		}
		if len(defaultDNS.Spec.Servers) != 0 {
			// dnses.operator/default has a nil spec by default.
			defaultDNS.Spec = operatorv1.DNSSpec{}
			if err := cl.Update(context.TODO(), defaultDNS); err != nil {
				t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
			}
		}
	}()

	// Verify that default DNS pods are all available before inspecting them.
	if err := waitForDNSConditions(t, cl, 5*time.Minute, dnsName, defaultAvailableDNSConditions...); err != nil {
		t.Errorf("expected default DNS pods to be available: %v", err)
	}

	// Verify that the Corefile of DNS DaemonSet pods have been updated.
	dnsDaemonSet := &appsv1.DaemonSet{}
	if err := cl.Get(context.TODO(), operatorcontroller.DNSDaemonSetName(defaultDNS), dnsDaemonSet); err != nil {
		_ = fmt.Errorf("failed to get daemonset %s/%s: %v", dnsDaemonSet.Namespace, dnsDaemonSet.Name, err)
	}
	selector, err := metav1.LabelSelectorAsSelector(dnsDaemonSet.Spec.Selector)
	if err != nil {
		t.Fatalf("daemonset %s/%s has invalid spec.selector: %v", dnsDaemonSet.Namespace, dnsDaemonSet.Name, err)
	}
	defaultDNSPods := &corev1.PodList{}
	if err := cl.List(context.TODO(), defaultDNSPods, client.MatchingLabelsSelector{Selector: selector}, client.InNamespace(dnsDaemonSet.Namespace)); err != nil {
		t.Fatalf("failed to list pods for dns daemonset %s/%s: %v", dnsDaemonSet.Namespace, dnsDaemonSet.Name, err)
	}
	catCmd := []string{"cat", "/etc/coredns/Corefile"}
	for _, pod := range defaultDNSPods.Items {
		if err := lookForStringInPodExec(pod.Namespace, pod.Name, "dns", catCmd, upstreamIP, 2*time.Minute); err != nil {
			// If we failed to find the expected IP in the pod's corefile, log the pod's status.
			currPod := &corev1.Pod{}
			if err := cl.Get(context.TODO(), types.NamespacedName{Name: pod.Name, Namespace: pod.Namespace}, currPod); err != nil {
				t.Logf("failed to get pod %s: %v", pod.Name, err)
			}
			t.Fatalf("failed to find %s in %s of pod %s/%s: %v, pod status: %v", upstreamIP, catCmd[1], pod.Namespace, pod.Name, err, currPod.Status)
		}
	}

	// Get the openshift-cli image.
	var (
		cliImage      string
		cliImageFound bool
	)
	for _, ver := range co.Status.Versions {
		if ver.Name == statuscontroller.OpenshiftCLIVersionName {
			if len(ver.Version) == 0 {
				break
			}
			cliImage = ver.Version
			cliImageFound = true
			break
		}
	}
	if !cliImageFound {
		t.Fatalf("failed to find the %s version for clusteroperator %s", statuscontroller.OpenshiftCLIVersionName, co.Name)
	}

	// Create the client Pod.
	testClient := buildPod("test-client", "default", cliImage, []string{"sleep", "3600"})
	if err := cl.Create(context.TODO(), testClient); err != nil {
		t.Fatalf("failed to create pod %s/%s: %v", testClient.Namespace, testClient.Name, err)
	}
	defer func() {
		if err := cl.Delete(context.TODO(), testClient); err != nil {
			t.Fatalf("failed to delete pod %s/%s: %v", testClient.Namespace, testClient.Name, err)
		}
	}()
	// Wait for the client Pod to be ready.
	name = types.NamespacedName{Namespace: testClient.Namespace, Name: testClient.Name}
	err = wait.PollImmediate(1*time.Second, 60*time.Second, func() (bool, error) {
		if err := cl.Get(context.TODO(), name, testClient); err != nil {
			t.Logf("failed to get pod %s/%s: %v", name.Namespace, name.Name, err)
			return false, nil
		}
		for _, cond := range testClient.Status.Conditions {
			if cond.Type == corev1.ContainersReady &&
				cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to observe ContainersReady condition for pod %s/%s: %v", testClient.Namespace, testClient.Name, err)
	}
	// Dig the example dns forwarding host.
	digCmd := []string{"dig", "+short", "www.foo.com", "A"}
	fooHost := "1.2.3.4"
	if err := lookForStringInPodExec(testClient.Namespace, testClient.Name, testClient.Name, digCmd, fooHost, 30*time.Second); err != nil {
		t.Fatalf("failed to dig %s: %v", upstreamIP, err)
	}
	// Scrape the upstream resolver logs for the "NOERROR" message.
	logMsg := "NOERROR"
	if err := lookForStringInPodLog(upstreamResolver.Namespace, upstreamResolver.Name, upstreamResolver.Name, logMsg, 120*time.Second); err != nil {
		t.Fatalf("failed to parse %q from pod %s/%s logs: %v", logMsg, upstreamResolver.Namespace, upstreamResolver.Name, err)
	}
}

func TestDNSOverTLSForwarding(t *testing.T) {
	tlsUpstreamName := "upstream-tls"

	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	// Ensure that DNS is stable before starting the test.
	if err := waitForDNSConditions(t, cl, 5*time.Minute, dnsName, defaultAvailableDNSConditions...); err != nil {
		t.Errorf("expected default DNS pods to be available: %v", err)
	}

	// Create the CA
	caCert, caKey, err := generateServerCA()
	if err != nil {
		t.Fatal(err)
	}

	// Create the server cert
	serverCert, serverKey, err := generateServerCertificate(caCert, caKey, tlsUpstreamName)
	if err != nil {
		t.Fatal(err)
	}

	// PEM encode the server cert and key
	pemServerCert := encodeCert(serverCert)
	pemServerKey := encodeKey(serverKey)

	// Create a separate namespace for the upstream resolver. Deleting this namespace will clean up the resources created.
	tlsUpstreamNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tlsUpstreamName,
		},
	}
	if err := cl.Create(context.TODO(), tlsUpstreamNamespace); err != nil {
		t.Fatalf("failed to create namespace for the upstream resolver namespace/%s: %v", tlsUpstreamNamespace.Name, err)
	}
	t.Cleanup(func() {
		if err := cl.Delete(context.TODO(), tlsUpstreamNamespace); err != nil {
			t.Fatalf("failed to delete upstream resolver namespace namespace/%s: %v", tlsUpstreamNamespace.Name, err)
		}
	})

	// upstreamTLSConfigMapData holds the server cert, server key, and Corefile for the upstream resolver.
	upstreamTLSConfigMapData := make(map[string]string)
	upstreamTLSConfigMapData["cert"] = pemServerCert
	upstreamTLSConfigMapData["key"] = pemServerKey
	upstreamTLSConfigMapData["Corefile"] = upstreamTLSCorefile

	upstreamTLSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tlsUpstreamName,
			Namespace: tlsUpstreamNamespace.Name,
		},
		Data: upstreamTLSConfigMapData,
	}

	// Create the upstream resolver TLS ConfigMap.
	if err := cl.Create(context.TODO(), upstreamTLSConfigMap); err != nil {
		t.Fatalf("failed to create configmap %s/%s: %v", upstreamTLSConfigMap.Namespace, upstreamTLSConfigMap.Name, err)
	}

	// Get the CoreDNS and openshift-cli images used by the test upstream resolver.
	// These are used to create the upstream resolver pods and the test-client-tls pod.
	co := &configv1.ClusterOperator{}
	if err := cl.Get(context.TODO(), opName, co); err != nil {
		t.Fatalf("failed to get clusteroperator %s: %v", opName, err)
	}
	var (
		coreImage      string
		cliImage       string
		coreImageFound bool
		cliImageFound  bool
	)
	for _, ver := range co.Status.Versions {
		if ver.Name == statuscontroller.CoreDNSVersionName {
			if len(ver.Version) == 0 {
				t.Fatalf("clusteroperator %s has empty coredns version", opName)
			}
			coreImageFound = true
			coreImage = ver.Version
		}
		if ver.Name == statuscontroller.OpenshiftCLIVersionName {
			if len(ver.Version) == 0 {
				t.Fatalf("clusteroperator %s has empty openshift-cli version", opName)
			}
			cliImageFound = true
			cliImage = ver.Version
		}
	}
	if !coreImageFound {
		t.Fatalf("version %s not found for clusteroperator %s", statuscontroller.CoreDNSVersionName, opName)
	}
	if !cliImageFound {
		t.Fatalf("version %s not found for clusteroperator %s", statuscontroller.OpenshiftCLIVersionName, opName)
	}

	// Create the upstream resolver Pods.
	upstreamResolver1 := upstreamTLSPod(tlsUpstreamName+"-1", tlsUpstreamNamespace.Name, coreImage, upstreamTLSConfigMap)
	if err := cl.Create(context.TODO(), upstreamResolver1); err != nil {
		t.Fatalf("failed to create pod %s/%s: %v", upstreamResolver1.Namespace, upstreamResolver1.Name, err)
	}
	upstreamResolver2 := upstreamTLSPod(tlsUpstreamName+"-2", tlsUpstreamNamespace.Name, coreImage, upstreamTLSConfigMap)
	if err := cl.Create(context.TODO(), upstreamResolver2); err != nil {
		t.Fatalf("failed to create pod %s/%s: %v", upstreamResolver2.Namespace, upstreamResolver2.Name, err)
	}

	// Wait for the first upstream resolver Pod to be ready.
	name := types.NamespacedName{Namespace: upstreamResolver1.Namespace, Name: upstreamResolver1.Name}
	err = wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
		if err := cl.Get(context.TODO(), name, upstreamResolver1); err != nil {
			t.Logf("failed to get pod %s/%s: %v", name.Namespace, name.Name, err)
			return false, nil
		}
		for _, cond := range upstreamResolver1.Status.Conditions {
			if cond.Type == corev1.ContainersReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to observe ContainersReady condition for pod %s/%s: %v", upstreamResolver1.Namespace, upstreamResolver1.Name, err)
	}

	// Wait for the second upstream resolver Pod to be ready.
	name = types.NamespacedName{Namespace: upstreamResolver2.Namespace, Name: upstreamResolver2.Name}
	err = wait.PollImmediate(1*time.Second, 2*time.Minute, func() (bool, error) {
		if err := cl.Get(context.TODO(), name, upstreamResolver2); err != nil {
			t.Logf("failed to get pod %s/%s: %v", name.Namespace, name.Name, err)
			return false, nil
		}
		for _, cond := range upstreamResolver2.Status.Conditions {
			if cond.Type == corev1.ContainersReady && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to observe ContainersReady condition for pod %s/%s: %v", upstreamResolver2.Namespace, upstreamResolver2.Name, err)
	}

	// Create the ConfigMap to hold the cert and key data for the operator configuration
	pemCACert := encodeCert(caCert)
	downstreamTLSConfigMapData := map[string]string{"ca-bundle.crt": pemCACert}
	downstreamTLSConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dns-over-tls-ca",
			Namespace: "openshift-config",
		},
		Data: downstreamTLSConfigMapData,
	}

	// Create the downstream resolver TLS ConfigMap.
	if err := cl.Create(context.TODO(), downstreamTLSConfigMap); err != nil {
		t.Fatalf("failed to create configmap %s/%s: %v", downstreamTLSConfigMap.Namespace, downstreamTLSConfigMap.Name, err)
	}
	t.Cleanup(func() {
		if err := cl.Delete(context.TODO(), downstreamTLSConfigMap); err != nil {
			t.Fatalf("failed to delete configmap %s/%s: %v", downstreamTLSConfigMap.Namespace, downstreamTLSConfigMap.Name, err)
		}
	})

	// Update cluster DNS forwarding with the upstream resolver's Service IP address and hostname.
	defaultDNS := &operatorv1.DNS{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Name: operatorcontroller.DefaultDNSController}, defaultDNS); err != nil {
		t.Fatalf("failed to get default dns: %v", err)
	}

	upstream := operatorv1.Server{
		Name:  "test",
		Zones: []string{"tls.com"},
		ForwardPlugin: operatorv1.ForwardPlugin{
			TransportConfig: operatorv1.DNSTransportConfig{
				Transport: operatorv1.TLSTransport,
				TLS: &operatorv1.DNSOverTLSConfig{
					ServerName: tlsUpstreamName,
					CABundle:   configv1.ConfigMapNameReference{Name: "dns-over-tls-ca"},
				},
			},
			Upstreams: []string{upstreamResolver1.Status.PodIP + ":5353", upstreamResolver2.Status.PodIP + ":5353"},
		},
	}
	defaultDNS.Spec.Servers = []operatorv1.Server{upstream}
	if err := cl.Update(context.TODO(), defaultDNS); err != nil {
		t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
	}
	t.Cleanup(func() {
		defaultDNS = &operatorv1.DNS{}
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: "default"}, defaultDNS); err != nil {
			t.Fatalf("failed to get default dns: %v", err)
		}
		if len(defaultDNS.Spec.Servers) != 0 {
			// dnses.operator/default has a nil spec by default.
			defaultDNS.Spec = operatorv1.DNSSpec{}
			if err := cl.Update(context.TODO(), defaultDNS); err != nil {
				t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
			}
		}
	})

	// Verify that default DNS pods are all available before inspecting them.
	if err := waitForDNSConditions(t, cl, 5*time.Minute, dnsName, defaultAvailableDNSConditions...); err != nil {
		t.Errorf("expected default DNS pods to be available: %v", err)
	}

	// Create the client Pod.
	testClient := buildPod("test-client-tls", "default", cliImage, []string{"sleep", "3600"})
	if err := cl.Create(context.TODO(), testClient); err != nil {
		t.Fatalf("failed to create pod %s/%s: %v", testClient.Namespace, testClient.Name, err)
	}
	t.Cleanup(func() {
		if err := cl.Delete(context.TODO(), testClient); err != nil {
			t.Fatalf("failed to delete pod %s/%s: %v", testClient.Namespace, testClient.Name, err)
		}
	})

	// Wait for the client Pod to be ready.
	name = types.NamespacedName{Namespace: testClient.Namespace, Name: testClient.Name}
	err = wait.PollImmediate(1*time.Second, 60*time.Second, func() (bool, error) {
		if err := cl.Get(context.TODO(), name, testClient); err != nil {
			t.Logf("failed to get pod %s/%s: %v", name.Namespace, name.Name, err)
			return false, nil
		}
		for _, cond := range testClient.Status.Conditions {
			if cond.Type == corev1.ContainersReady &&
				cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to observe ContainersReady condition for pod %s/%s: %v", testClient.Namespace, testClient.Name, err)
	}

	// Dig the Corefile host.
	digCmd := []string{"dig", "+short", "www.tls.com", "A"}
	fooHost := "4.3.2.1"
	if err = lookForStringInPodExec(testClient.Namespace, testClient.Name, testClient.Name, digCmd, fooHost, 30*time.Second); err != nil {
		t.Fatalf("failed to forward request to %s or %s: %v", upstreamResolver1.Status.PodIP, upstreamResolver2.Status.PodIP, err)
	}

	// Scrape the upstream resolver logs for the "NOERROR" message.
	// This looks in both upstream resolvers because the forwarding policy could be random. This also serves the purpose
	// of testing that one server certificate will work for multiple upstreams using the same ServerName.
	upstreamResolverLogMsg := "NOERROR"
	var firstResolver *corev1.Pod
	var secondResolver *corev1.Pod
	if err = lookForStringInPodLog(upstreamResolver1.Namespace, upstreamResolver1.Name, upstreamResolver1.Name, upstreamResolverLogMsg, 30*time.Second); err != nil {
		t.Logf("%s/%s: %v", upstreamResolver1.Namespace, upstreamResolver1.Name, err)
	} else {
		firstResolver = upstreamResolver1
	}

	if err = lookForStringInPodLog(upstreamResolver2.Namespace, upstreamResolver2.Name, upstreamResolver2.Name, upstreamResolverLogMsg, 30*time.Second); err != nil {
		t.Logf("%s/%s: %v", upstreamResolver2.Namespace, upstreamResolver2.Name, err)
	} else {
		firstResolver = upstreamResolver2
	}

	// Neither of the upstreams resolved the request. Fail now.
	if firstResolver == nil {
		t.Fatalf("failed to parse %q from upstream resolver pods: %v", upstreamResolverLogMsg, err)
	}

	// Take down this resolver and retry to ensure the next resolver gets the request
	if err = cl.Delete(context.TODO(), firstResolver); err != nil {
		t.Fatalf("failed to delete pod %s/%s: %v", firstResolver.Namespace, firstResolver.Name, err)
	}

	// Dig the Corefile host. This will trigger another NOERROR log in the remaining upstream.
	if err = lookForStringInPodExec(testClient.Namespace, testClient.Name, testClient.Name, digCmd, fooHost, 30*time.Second); err != nil {
		t.Fatalf("failed to forward request to %s or %s: %v", upstreamResolver1.Status.PodIP, upstreamResolver2.Status.PodIP, err)
	}

	// Look for the NOERROR message again. Check both resolvers because we don't know which one is still up.
	if err = lookForStringInPodLog(upstreamResolver1.Namespace, upstreamResolver1.Name, upstreamResolver1.Name, upstreamResolverLogMsg, 30*time.Second); err != nil {
		t.Logf("%s/%s: %v", upstreamResolver1.Namespace, upstreamResolver1.Name, err)
	} else {
		secondResolver = upstreamResolver1
	}

	if err = lookForStringInPodLog(upstreamResolver2.Namespace, upstreamResolver2.Name, upstreamResolver2.Name, upstreamResolverLogMsg, 30*time.Second); err != nil {
		t.Logf("%s/%s: %v", upstreamResolver2.Namespace, upstreamResolver2.Name, err)
	} else {
		secondResolver = upstreamResolver2
	}

	// Neither of the upstreams resolved the request. Fail now.
	if secondResolver == nil {
		t.Fatalf("failed to parse %q from upstream resolver pods: %v", upstreamResolverLogMsg, err)
	}
}

func TestDNSOverTLSToleratesMissingSourceCM(t *testing.T) {
	missingCMName := "missing-cm"
	missingCMLog := missingCMName + " does not exist"

	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	// Ensure that DNS is stable before starting the test.
	if err := waitForDNSConditions(t, cl, 5*time.Minute, dnsName, defaultAvailableDNSConditions...); err != nil {
		t.Errorf("expected default DNS pods to be available: %v", err)
	}

	// Update cluster DNS forwarding with the missing configmap.
	defaultDNS := &operatorv1.DNS{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Name: operatorcontroller.DefaultDNSController}, defaultDNS); err != nil {
		t.Fatalf("failed to get default dns: %v", err)
	}

	upstream := operatorv1.Server{
		Name:  "test",
		Zones: []string{"tls.com"},
		ForwardPlugin: operatorv1.ForwardPlugin{
			TransportConfig: operatorv1.DNSTransportConfig{
				Transport: operatorv1.TLSTransport,
				TLS: &operatorv1.DNSOverTLSConfig{
					ServerName: "dns.tls.com",
					CABundle:   configv1.ConfigMapNameReference{Name: missingCMName},
				},
			},
			Upstreams: []string{"1.1.1.1"},
		},
	}

	defaultDNS.Spec.Servers = []operatorv1.Server{upstream}
	if err := cl.Update(context.TODO(), defaultDNS); err != nil {
		t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
	}
	t.Cleanup(func() {
		defaultDNS = &operatorv1.DNS{}
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: "default"}, defaultDNS); err != nil {
			t.Fatalf("failed to get default dns: %v", err)
		}
		if len(defaultDNS.Spec.Servers) != 0 {
			// dnses.operator/default has a nil spec by default.
			defaultDNS.Spec = operatorv1.DNSSpec{}
			if err := cl.Update(context.TODO(), defaultDNS); err != nil {
				t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
			}
		}
	})

	// Verify that default DNS pods are all available before inspecting them.
	if err := waitForDNSConditions(t, cl, 5*time.Minute, dnsName, defaultAvailableDNSConditions...); err != nil {
		t.Errorf("expected default DNS pods to be available: %v", err)
	}

	dnsOperatorPods, err := getClusterDNSOperatorPods(cl)
	if err != nil {
		t.Fatalf("unable to get operator pods: %v", err)
	}

	for _, dnsOperatorPod := range dnsOperatorPods.Items {
		if err := lookForStringInPodLog(operatorcontroller.DefaultOperatorNamespace, dnsOperatorPod.Name, operatorcontroller.ContainerNameOfDNSOperator, missingCMLog, 30*time.Second); err != nil {
			t.Fatalf("could not find pod logs: %v", err)
		}
	}
}

func TestDNSLogging(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	// Get the CoreDNS image used by the test upstream resolver.
	co := &configv1.ClusterOperator{}
	if err := cl.Get(context.TODO(), opName, co); err != nil {
		t.Fatalf("failed to get clusteroperator %s: %v", opName, err)
	}

	// Update cluster DNS forwarding with the upstream resolver's Service IP address.
	defaultDNS := &operatorv1.DNS{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Name: operatorcontroller.DefaultDNSController}, defaultDNS); err != nil {
		t.Fatalf("failed to get default dns: %v", err)
	}

	logLevel := operatorv1.DNSSpec{
		LogLevel:         "Debug",
		OperatorLogLevel: "Debug",
	}

	defaultDNS.Spec.LogLevel = logLevel.LogLevel
	defaultDNS.Spec.OperatorLogLevel = logLevel.OperatorLogLevel
	if err := cl.Update(context.TODO(), defaultDNS); err != nil {
		t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
	}
	defer func() {
		defaultDNS = &operatorv1.DNS{}
		if err := cl.Get(context.TODO(), types.NamespacedName{Name: "default"}, defaultDNS); err != nil {
			t.Fatalf("failed to get default dns: %v", err)
		}

		if defaultDNS.Spec.LogLevel != "" && defaultDNS.Spec.OperatorLogLevel != "" {
			// dnses.operator/default has a nil spec by default.
			defaultDNS.Spec = operatorv1.DNSSpec{}
			if err := cl.Update(context.TODO(), defaultDNS); err != nil {
				t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
			}
		}
	}()

	// Verify that default DNS pods are all available before inspecting them.
	if err := waitForDNSConditions(t, cl, 5*time.Minute, dnsName, defaultAvailableDNSConditions...); err != nil {
		t.Errorf("expected default DNS pods to be available: %v", err)
	}

	// Verify that the Corefile of DNS DaemonSet pods have been updated.
	dnsDaemonSet := &appsv1.DaemonSet{}
	if err := cl.Get(context.TODO(), operatorcontroller.DNSDaemonSetName(defaultDNS), dnsDaemonSet); err != nil {
		_ = fmt.Errorf("failed to get daemonset %s/%s: %v", dnsDaemonSet.Namespace, dnsDaemonSet.Name, err)
	}
	selector, err := metav1.LabelSelectorAsSelector(dnsDaemonSet.Spec.Selector)
	if err != nil {
		t.Fatalf("daemonset %s/%s has invalid spec.selector: %v", dnsDaemonSet.Namespace, dnsDaemonSet.Name, err)
	}
	coreDNSPods := &corev1.PodList{}
	if err := cl.List(context.TODO(), coreDNSPods, client.MatchingLabelsSelector{Selector: selector}, client.InNamespace(dnsDaemonSet.Namespace)); err != nil {
		t.Fatalf("failed to list pods for dns daemonset %s/%s: %v", dnsDaemonSet.Namespace, dnsDaemonSet.Name, err)
	}
	catCmd := []string{"cat", "/etc/coredns/Corefile"}

	// Get the openshift-cli image.
	var (
		cliImage      string
		cliImageFound bool
	)
	for _, ver := range co.Status.Versions {
		if ver.Name == statuscontroller.OpenshiftCLIVersionName {
			if len(ver.Version) == 0 {
				break
			}
			cliImage = ver.Version
			cliImageFound = true
			break
		}
	}
	if !cliImageFound {
		t.Fatalf("failed to find the %s version for clusteroperator %s", statuscontroller.OpenshiftCLIVersionName, co.Name)
	}

	// Create the client Pod.
	testClientForDNSLogging := buildPod("test-client-dnslogging", "default", cliImage, []string{"sleep", "3600"})
	if err := cl.Create(context.TODO(), testClientForDNSLogging); err != nil {
		t.Fatalf("failed to create pod %s/%s: %v", testClientForDNSLogging.Namespace, testClientForDNSLogging.Name, err)
	}
	defer func() {
		if err := cl.Delete(context.TODO(), testClientForDNSLogging); err != nil {
			t.Fatalf("failed to delete pod %s/%s: %v", testClientForDNSLogging.Namespace, testClientForDNSLogging.Name, err)
		}
	}()
	// Wait for the client Pod to be ready.
	name := types.NamespacedName{Namespace: testClientForDNSLogging.Namespace, Name: testClientForDNSLogging.Name}
	err = wait.PollImmediate(1*time.Second, 60*time.Second, func() (bool, error) {
		if err := cl.Get(context.TODO(), name, testClientForDNSLogging); err != nil {
			t.Logf("failed to get pod %s/%s: %v", name.Namespace, name.Name, err)
			return false, nil
		}
		for _, cond := range testClientForDNSLogging.Status.Conditions {
			if cond.Type == corev1.ContainersReady &&
				cond.Status == corev1.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("failed to observe ContainersReady condition for pod %s/%s: %v", testClientForDNSLogging.Namespace, testClientForDNSLogging.Name, err)
	}

	found := 0
	for _, corednspod := range coreDNSPods.Items {

		// Dig the example dns forwarding host.
		digCmd := []string{"dig", "test.svc.cluster.local"}
		if err := lookForStringInPodExec(testClientForDNSLogging.Namespace, testClientForDNSLogging.Name, testClientForDNSLogging.Name, digCmd, "NXDOMAIN", 2*time.Minute); err != nil {
			t.Fatalf("failed to dig %v", err)
		}

		if err := lookForStringInPodExec(corednspod.Namespace, corednspod.Name, "dns", catCmd, "class denial error", 2*time.Minute); err != nil {
			t.Fatalf("failed to set Debug logLevel for operator %s: %v", opName, err)
		}

		// Get the CoreDNS image used by the test upstream resolver.
		co := &configv1.ClusterOperator{}
		if err := cl.Get(context.TODO(), opName, co); err != nil {
			t.Fatalf("failed to get clusteroperator %s: %v", opName, err)
		}

		if found == 0 {
			if err := lookForSubStringsInPodLog(corednspod.Namespace, corednspod.Name, "dns", 2*time.Minute, "A IN test.svc.cluster.local.", "NXDOMAIN"); err != nil {
				found = 0
			} else {
				found = 1
			}
		}

	}

	if found == 0 {
		t.Fatalf("failed to get NXDOMAIN entry for test.svc.cluster.local. which does not exist")
	}

	dns := &operatorv1.DNS{}
	if err := cl.Get(context.TODO(), dnsName, dns); err != nil {
		t.Fatalf("failed to get DNS operator %s: %v", dnsName.Name, err)
	}

	dnsOperatorDeployment := &appsv1.Deployment{}
	if err := cl.Get(context.TODO(), operatorcontroller.DefaultDNSOperatorDeploymentName(), dnsOperatorDeployment); err != nil {
		_ = fmt.Errorf("failed to get deployment %s/%s: %v", dnsOperatorDeployment.Namespace, dnsOperatorDeployment.Name, err)
	}
	operatorSelector, err := metav1.LabelSelectorAsSelector(dnsOperatorDeployment.Spec.Selector)
	if err != nil {
		t.Fatalf("daemonset %s/%s has invalid spec.selector: %v", dnsOperatorDeployment.Namespace, dnsOperatorDeployment.Name, err)
	}

	dnsOperatorPods := &corev1.PodList{}
	if err := cl.List(context.TODO(), dnsOperatorPods, client.MatchingLabelsSelector{Selector: operatorSelector}, client.InNamespace(dnsOperatorDeployment.Namespace)); err != nil {
		t.Fatalf("failed to list pods for dns deployment %s/%s: %v", dnsOperatorDeployment.Namespace, dnsOperatorDeployment.Name, err)
	}

	for _, dnsOperatorPod := range dnsOperatorPods.Items {
		if err := lookForStringInPodLog(operatorcontroller.DefaultOperatorNamespace, dnsOperatorPod.Name, operatorcontroller.ContainerNameOfDNSOperator, "level=info", 30*time.Second); err != nil {
			t.Fatal(" failed to set Debug logLevel for operator")
		}
	}
}

// TestDNSNodePlacement verifies that the node placement API works properly by
// first configuring DNS pods to run only on master nodes and verifying that
// this configuration results in having the expected number of DNS pods, then
// configuring DNS pods with a label selector that selects 0 nodes and verifying
// that the invalid label selector is ignored.
func TestDNSNodePlacement(t *testing.T) {
	cl, err := getClient()
	if err != nil {
		t.Fatal(err)
	}

	// Configure DNS pods to run only on master nodes.
	defaultDNS := &operatorv1.DNS{}
	name := types.NamespacedName{Name: operatorcontroller.DefaultDNSController}
	if err := cl.Get(context.TODO(), name, defaultDNS); err != nil {
		t.Fatalf("failed to get default dns: %v", err)
	}
	defaultDNS.Spec.NodePlacement.NodeSelector = map[string]string{
		"node-role.kubernetes.io/master": "",
	}
	if err := cl.Update(context.TODO(), defaultDNS); err != nil {
		t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
	}
	defer func() {
		defaultDNS = &operatorv1.DNS{}
		if err := cl.Get(context.TODO(), name, defaultDNS); err != nil {
			t.Fatalf("failed to get default dns: %v", err)
		}
		if defaultDNS.Spec.NodePlacement.NodeSelector != nil {
			defaultDNS.Spec.NodePlacement.NodeSelector = nil
			if err := cl.Update(context.TODO(), defaultDNS); err != nil {
				t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
			}
		}
	}()

	// Verify that all remaining DNS pods are running on master nodes.
	dnsDaemonSet := &appsv1.DaemonSet{}
	dnsDaemonSetName := operatorcontroller.DNSDaemonSetName(defaultDNS)
	if err := cl.Get(context.TODO(), dnsDaemonSetName, dnsDaemonSet); err != nil {
		fmt.Printf("failed to get daemonset %s: %v", dnsDaemonSetName, err)
	}
	selector, err := metav1.LabelSelectorAsSelector(dnsDaemonSet.Spec.Selector)
	if err != nil {
		t.Fatalf("daemonset %s has invalid spec.selector: %v", dnsDaemonSetName, err)
	}
	listOpts := []client.ListOption{
		client.MatchingLabelsSelector{Selector: selector},
		client.InNamespace(dnsDaemonSet.Namespace),
	}
	nodeList := &corev1.NodeList{}
	if err := cl.List(context.TODO(), nodeList); err != nil {
		t.Fatalf("failed to list nodes: %v", err)
	}
	masterNodes := sets.NewString()
	for _, node := range nodeList.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			masterNodes.Insert(node.Name)
		}
	}
	podList := &corev1.PodList{}
	err = wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		if err := cl.List(context.TODO(), podList, listOpts...); err != nil {
			t.Logf("failed to list pods for dns daemonset %s: %v", dnsDaemonSetName, err)
			return false, nil
		}
		for _, pod := range podList.Items {
			if !masterNodes.Has(pod.Spec.NodeName) {
				return false, nil
			}
		}
		return true, nil
	})
	if err != nil {
		nodes := sets.NewString()
		for _, node := range nodeList.Items {
			nodes.Insert(node.Name)
		}
		t.Errorf("failed to observe updated set of nodes with dns pods; expected %s; got %s", strings.Join(masterNodes.List(), ","), strings.Join(nodes.List(), ","))
	}

	// Configure DNS pods to run only on nodes with label foo=bar (which we
	// assume no nodes have).
	if err := cl.Get(context.TODO(), name, defaultDNS); err != nil {
		t.Fatalf("failed to get default dns: %v", err)
	}
	defaultDNS.Spec.NodePlacement.NodeSelector = map[string]string{
		"foo": "bar",
	}
	if err := cl.Update(context.TODO(), defaultDNS); err != nil {
		t.Fatalf("failed to update dns %s: %v", defaultDNS.Name, err)
	}

	// Verify that DNS pods continue to run (meaning the label selector that
	// matched no nodes is being ignored).
	err = wait.PollImmediate(1*time.Second, 3*time.Minute, func() (bool, error) {
		if err := cl.List(context.TODO(), podList, listOpts...); err != nil {
			t.Logf("failed to list pods for dns daemonset %s: %v", dnsDaemonSetName, err)
			return false, nil
		}
		return len(podList.Items) == 0, nil
	})
	if len(podList.Items) == 0 {
		t.Errorf("expected label selector matching 0 nodes to be ignored; found 0 dns pods")
	}
}
