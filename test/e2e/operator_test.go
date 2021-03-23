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

	operatorclient "github.com/openshift/cluster-dns-operator/pkg/operator/client"
	operatorcontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller"
	statuscontroller "github.com/openshift/cluster-dns-operator/pkg/operator/controller/status"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

var (
	dnsName = types.NamespacedName{Name: operatorcontroller.DefaultDNSName}
	opName  = types.NamespacedName{Name: operatorcontroller.DefaultOperatorName}

	defaultAvailableDNSConditions = []operatorv1.OperatorCondition{
		{Type: operatorv1.OperatorStatusTypeAvailable, Status: operatorv1.ConditionTrue},
	}
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
			t.Logf("failed to get DNS cluster operator %s: %v", opName.Name, err)
			return false, nil
		}

		if reflect.DeepEqual(expected, co.Status.RelatedObjects) {
			return true, nil
		}

		return false, nil
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
	if err := waitForDNSConditions(t, cl, 1*time.Minute, dnsName, defaultAvailableDNSConditions...); err != nil {
		t.Errorf("expected default DNS pods to be available: %v", err)
	}

	// Verify that the Corefile of DNS DaemonSet pods have been updated.
	dnsDaemonSet := &appsv1.DaemonSet{}
	if err := cl.Get(context.TODO(), operatorcontroller.DNSDaemonSetName(defaultDNS), dnsDaemonSet); err != nil {
		fmt.Errorf("failed to get daemonset %s/%s: %v", dnsDaemonSet.Namespace, dnsDaemonSet.Name, err)
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
			if err := cl.Get(context.TODO(), types.NamespacedName{pod.Name, pod.Namespace}, currPod); err != nil {
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
	if err := lookForStringInPodLog(upstreamResolver.Namespace, upstreamResolver.Name, upstreamResolver.Name, logMsg, 30*time.Second); err != nil {
		t.Fatalf("failed to parse %q from pod %s/%s logs: %v", logMsg, upstreamResolver.Namespace, upstreamResolver.Name, err)
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
		fmt.Errorf("failed to get daemonset %s: %v", dnsDaemonSetName, err)
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
