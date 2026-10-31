package util

import (
	"context"
	"fmt"
	"math/rand"
	"net/netip"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	operatorv1 "github.com/openshift/api/operator/v1"
	operatorclient "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	coreclient "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

// Timeout budgets for the DNS tests. The OTE migration guidelines ask for bounded,
// operation-specific timeouts rather than ad-hoc literals; these are the reference values
// recorded in OCPBUGS-132070.
const (
	// DefaultTimeout bounds a single API-driven operation that should settle quickly.
	DefaultTimeout = 3 * time.Minute
	// LongTimeout bounds an operation that waits on a DaemonSet or Deployment rollout.
	LongTimeout = 5 * time.Minute
	// ClusterOperatorTimeout bounds a wait for a ClusterOperator to report a stable status.
	ClusterOperatorTimeout = 2 * time.Minute
	// microShiftCheckTimeout bounds the single configmap read SkipOnMicroShift makes. A
	// skip guard runs before every spec, so it must not stall the suite when the API is
	// down; on timeout the test runs and fails on its own terms.
	microShiftCheckTimeout = 30 * time.Second
)

// payloadImagePlaceholder is the token fixtures carry in place of a hard-coded pull spec.
// ReplacePayloadImage swaps it for an image resolved from the cluster's own release
// payload, which is mirrored into disconnected lanes; fixtures must not reference an
// external registry directly (OCPBUGS-132070, pattern 2).
const (
	payloadImagePlaceholder = "PAYLOAD_TOOLS_IMAGE"
	payloadImageNamespace   = "openshift"
)

// SkipOnMicroShift skips the calling test when it runs against a MicroShift cluster.
//
// MicroShift does not serve the dns.operator.openshift.io API these tests drive. The
// runtime check is authoritative because MicroShift and platform-agnostic single-node
// OpenShift share environment selector values. MicroShift reports a clean skip instead
// of the setup failure seen in OCPBUGS-132070, pattern 1.
//
// It deliberately builds its own clientset instead of taking an *exutil.CLI. exutil.NewCLI
// registers a BeforeEach that creates a project, and Ginkgo runs BeforeEach blocks in
// declaration order, so a guard that depended on that CLI would run *after* project
// creation had already failed on MicroShift. Callers must register this as the first
// BeforeEach in the Describe body, before exutil.NewCLI is called.
//
// It also deliberately does not call exutil.IsMicroShiftCluster, which fails open to
// "MicroShift" when the cluster is unreachable. That helper polls for the configmap for
// five minutes and, on a persistent network error, leaves cm pointing at the empty
// &ConfigMap{} the typed client returns alongside the error rather than nil; its `if cm ==
// nil` guard therefore misses and it returns true, contradicting the "assuming not
// MicroShift" line it just logged. A real cluster with a five-minute API outage would skip
// all 28 tests as "MicroShift" and the suite would report green.
//
// Only a configmap we actually read counts as MicroShift. Any error - unreachable API,
// RBAC, timeout - falls through to running the test, so a broken cluster fails loudly in
// setup instead of disappearing into a silent skip.
func SkipOnMicroShift() {
	kubeClient, err := e2e.LoadClientset()
	if err != nil {
		e2e.Logf("could not build a client to check for MicroShift, continuing")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), microShiftCheckTimeout)
	defer cancel()

	cm, err := kubeClient.CoreV1().ConfigMaps("kube-public").
		Get(ctx, "microshift-version", metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		e2e.Logf("microshift-version configmap not found, not MicroShift")
		return
	case err != nil:
		e2e.Logf("could not read the MicroShift marker configmap, continuing")
		return
	case cm == nil:
		e2e.Logf("microshift-version configmap read returned no object, continuing")
		return
	}

	g.Skip("Skipping on MicroShift: the dns.operator.openshift.io API is not served there")
}

// GetPayloadImage returns the pull spec of the named tag of an imagestream in the
// "openshift" namespace. The payload imagestreams (tools, cli, network-tools, ...) are
// managed by the CVO and their spec tags point at the release registry, which is mirrored
// on disconnected clusters. This resolves without the internal image registry, which some
// of these tests disable.
func GetPayloadImage(oc *exutil.CLI, imageStream, tag string) string {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// The CLI logs command output on errors. Read through the typed client and
	// report a fixed diagnostic so pull specs and API endpoints stay out of logs.
	stream, err := oc.AdminImageClient().ImageV1().ImageStreams(payloadImageNamespace).Get(ctx, imageStream, metav1.GetOptions{})
	o.Expect(err == nil).To(o.BeTrue(),
		fmt.Sprintf("failed to read release payload imagestream %s/%s", payloadImageNamespace, imageStream))
	var image string
	for _, specTag := range stream.Spec.Tags {
		if specTag.Name == tag && specTag.From != nil {
			image = specTag.From.Name
			break
		}
	}
	o.Expect(image).NotTo(o.BeEmpty(),
		fmt.Sprintf("imagestream %s/%s has no %q tag pull spec", payloadImageNamespace, imageStream, tag))
	e2e.Logf("Resolved payload image for %s/%s:%s", payloadImageNamespace, imageStream, tag)
	return image
}

// ExpectResolvedAddressesForCluster checks that DNSNameResolver returned valid IPs
// for every address family supported by the cluster, including both on dual stack.
func ExpectResolvedAddressesForCluster(oc *exutil.CLI, addresses string) {
	ips := strings.Fields(addresses)
	o.Expect(ips).NotTo(o.BeEmpty(), "DNSNameResolver must return resolved IP addresses")
	var hasIPv4, hasIPv6 bool
	for _, value := range ips {
		ip, err := netip.ParseAddr(value)
		o.Expect(err).NotTo(o.HaveOccurred(), "DNSNameResolver must return valid IP addresses")
		hasIPv4 = hasIPv4 || ip.Is4()
		hasIPv6 = hasIPv6 || ip.Is6()
	}
	stackType := compat_otp.GetIPVersionStackType(oc)
	o.Expect(stackType).To(o.BeElementOf("ipv4single", "ipv6single", "dualstack"), "cluster network stack must be known")
	if stackType != "ipv6single" {
		o.Expect(hasIPv4).To(o.BeTrue(), "DNSNameResolver must return an IPv4 address on IPv4 and dual-stack clusters")
	}
	if stackType != "ipv4single" {
		o.Expect(hasIPv6).To(o.BeTrue(), "DNSNameResolver must return an IPv6 address on IPv6 and dual-stack clusters")
	}
}

// ReplacePayloadImage rewrites the payload image placeholder in an extracted fixture with
// the release payload's "tools" image. Fixtures are extracted to a per-run temp directory
// by FixturePath, so rewriting in place is safe. Calling it twice on the same file is a
// no-op.
func ReplacePayloadImage(oc *exutil.CLI, file string) {
	data, err := os.ReadFile(file)
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("failed to read fixture %s", file))
	if !strings.Contains(string(data), payloadImagePlaceholder) {
		e2e.Logf("fixture %s already has its image resolved, continuing", file)
		return
	}
	image := GetPayloadImage(oc, "tools", "latest")
	updated := strings.ReplaceAll(string(data), payloadImagePlaceholder, image)
	o.Expect(os.WriteFile(file, []byte(updated), 0644)).NotTo(o.HaveOccurred(),
		fmt.Sprintf("failed to write fixture %s", file))
}

func GetRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[rand.Intn(len(chars))]
	}
	return string(buffer)
}

func WaitForPodWithLabelReady(oc *exutil.CLI, ns, label string) error {
	return wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label, `-ojsonpath={.items[*].status.conditions[?(@.type=="Ready")].status}`).Output()
		e2e.Logf("the Ready status of pod is %v", status)
		if err != nil || status == "" {
			e2e.Logf("failed to get pod status: %v, retrying...", err)
			return false, nil
		}
		if strings.Contains(status, "False") {
			e2e.Logf("the pod Ready status not met; wanted True but got %v, retrying...", status)
			return false, nil
		}
		return true, nil
	})
}

func EnsurePodWithLabelReady(oc *exutil.CLI, ns, label string) {
	err := WaitForPodWithLabelReady(oc, ns, label)
	if err != nil {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label).Output()
		e2e.Logf("All pods with label %v are:\n%v", label, output)
		logs, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, "-l", label, "--tail=10").Output()
		e2e.Logf("The logs of all labeled pods are:\n%v", logs)
	}
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but the pods with label %v are not ready", label))
}

func WaitForResourceToDisappear(oc *exutil.CLI, ns, rsname string) error {
	return wait.Poll(20*time.Second, 5*time.Minute, func() (bool, error) {
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(rsname, "-n", ns).Output()
		e2e.Logf("check resource %v and got: %v", rsname, status)
		primary := false
		if err != nil {
			if strings.Contains(status, "NotFound") {
				e2e.Logf("the resource is disappeared!")
				primary = true
			} else {
				e2e.Logf("failed to get the resource: %v, retrying...", err)
			}
		} else {
			e2e.Logf("the resource is still there, retrying...")
		}
		return primary, nil
	})
}

func PatchGlobalResourceAsAdmin(oc *exutil.CLI, resource, patch string) {
	patchOut, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "--patch="+patch, "--type=json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The output from the patch is:- %q ", patchOut)
}

func GetByJsonPath(oc *exutil.CLI, ns, resource, jsonPath string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, resource, "-o=jsonpath="+jsonPath).Output()
	if err != nil {
		e2e.Logf("the error is: %v", err.Error())
	}
	e2e.Logf("the output filtered by jsonpath is: %v", output)
	return output
}

func GetByLabelAndJsonPath(oc *exutil.CLI, ns, resource, label, jsonPath string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, resource, "-l", label, "-ojsonpath="+jsonPath).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the output filtered by label and jsonpath is: %v", output)
	return output
}

func GetNodeNameByPod(oc *exutil.CLI, namespace string, podName string) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The nodename for pod %s in namespace %s is %s", podName, namespace, nodeName)
	return nodeName
}

func GetPodListByLabel(oc *exutil.CLI, namespace string, label string) []string {
	var podList []string
	podNameAll, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "pod", "-l", label, "-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podList = strings.Split(podNameAll, " ")
	e2e.Logf("The pod list is %v", podList)
	return podList
}

func GetDNSPodName(oc *exutil.CLI) string {
	ns := "openshift-dns"
	label := "dns.operator.openshift.io/daemonset-dns=default"
	var podName string
	waitErr := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pods", "-l", label, "-o=jsonpath={.items[0].metadata.name}").Output()
		if err != nil {
			e2e.Logf("DNS pods not available yet, retrying... error: %v", err)
			return false, nil
		}
		if output == "" {
			e2e.Logf("DNS pod name is empty, retrying...")
			return false, nil
		}
		podName = output
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, "timed out waiting for DNS pod to be available")
	e2e.Logf("The DNS pod name is: %v", podName)
	return podName
}

func PollReadDnsCorefile(oc *exutil.CLI, dnsPodName, searchString1, grepOption, searchString2 string) string {
	e2e.Logf("Polling and search dns Corefile")
	ns := "openshift-dns"
	// Search literal Corefile fragments; brackets in IPv6 endpoints are not regex classes.
	cmd1 := fmt.Sprintf("grep -F \"%s\" /etc/coredns/Corefile %s | grep -F \"%s\"", searchString1, grepOption, searchString2)
	cmd2 := fmt.Sprintf("grep -F \"%s\" /etc/coredns/Corefile %s", searchString1, grepOption)

	waitErr := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {
		HackAnnotatePod(oc, ns, dnsPodName)
		_, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, dnsPodName, "--", "bash", "-c", cmd1).Output()
		if err != nil {
			e2e.Logf("string not found, wait and try again...")
			return false, nil
		}
		return true, nil
	})
	if waitErr != nil {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", "dns.operator.openshift.io/daemonset-dns=default").Output()
		e2e.Logf("All current dns pods are:\n%v", output)
		output, _ = oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, dnsPodName, "--", "bash", "-c", "cat /etc/coredns/Corefile").Output()
		e2e.Logf("The existing Corefile is: %v", output)
	}
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but Corefile is not updated"))
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, dnsPodName, "--", "bash", "-c", cmd2).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the part of Corefile that matching \"%s\" is: %v", searchString1, output)
	return output
}

func HackAnnotatePod(oc *exutil.CLI, ns, podName string) {
	hackAnnotation := "ne-testing-hack=" + GetRandomString()
	oc.AsAdmin().WithoutNamespace().Run("annotate").Args("pod", podName, "-n", ns, hackAnnotation, "--overwrite").Execute()
}

func EnsureClusterOperatorNormal(oc *exutil.CLI, coName string, healthyThreshold int, totalWaitTime int) {
	count := 0
	printCount := 0
	jsonPath := `{.status.conditions[?(@.type=="Available")].status}{.status.conditions[?(@.type=="Progressing")].status}{.status.conditions[?(@.type=="Degraded")].status}`

	e2e.Logf("waiting for CO %v back to normal status......", coName)
	waitErr := wait.PollImmediate(5*time.Second, time.Duration(totalWaitTime)*time.Second, func() (bool, error) {
		status := GetByJsonPath(oc, "default", "co/"+coName, jsonPath)
		primary := false
		printCount++
		if strings.Compare(status, "TrueFalseFalse") == 0 {
			count++
			if count == healthyThreshold {
				e2e.Logf("got %v successive good status (%v), the CO is stable!", count, status)
				primary = true
			} else {
				e2e.Logf("got %v successive good status (%v), try again...", count, status)
			}
		} else {
			count = 0
			if printCount%10 == 1 {
				e2e.Logf("CO status is still abnormal (%v), wait and try again...", status)
			}
		}
		return primary, nil
	})
	if waitErr != nil {
		output := GetByJsonPath(oc, "default", "co/"+coName, "{.status.conditions}")
		e2e.Logf("The co %v is abnormal and here is status: %v", coName, output)
		if coName == "ingress" {
			output, _ = oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", "openshift-ingress", "service", "router-default").Output()
			e2e.Logf("The output of describe router-default service: %v", output)
		}
	}
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but CO %v is still abnormal.", coName))
}

func ForceOnlyOneDnsPodExist(oc *exutil.CLI) string {
	ns := "openshift-dns"
	dnsPodLabel := "dns.operator.openshift.io/daemonset-dns=default"
	dnsNodeSelector := `[{"op":"replace", "path":"/spec/nodePlacement/nodeSelector", "value":{"nid-dns-testing":"true"}}]`
	err := oc.AsAdmin().WithoutNamespace().Run("label").Args("node", "-l", "nid-dns-testing=true", "nid-dns-testing-").Execute()
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to remove nid-dns-testing label from nodes")
	podList := GetAllDNSPodsNames(oc)
	if len(podList) == 1 {
		e2e.Logf("Found only one dns-default pod and it looks like SNO cluster. Continue the test...")
	} else {
		dnsPodName := GetRandomElementFromList(podList)
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", dnsPodName, "-o=jsonpath={.spec.nodeName}", "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Find random dns pod '%s' and its node '%s' which will be used for the following testing", dnsPodName, nodeName)
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nodeName, "nid-dns-testing=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		PatchGlobalResourceAsAdmin(oc, "dnses.operator.openshift.io/default", dnsNodeSelector)
		err1 := WaitForDNSPodCount(oc, 1)
		if err1 != nil {
			e2e.Logf("DNS pods did not converge to 1, checking for maxSurge stall...")
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", "-l", dnsPodLabel, "-ojsonpath={.items[*].metadata.name}", "--sort-by=.metadata.creationTimestamp").Output()
			if output != "" {
				pods := strings.Split(output, " ")
				if len(pods) == 2 {
					e2e.Logf("DaemonSet maxSurge stall detected: 2 pods remain %v, deleting older pod %s", pods, pods[0])
					oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", ns, "pod", pods[0], "--grace-period=0").Execute()
					err1 = WaitForDNSPodCount(oc, 1)
				}
			}
			if err1 != nil {
				output2, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", dnsPodLabel).Output()
				e2e.Logf("All current dns pods are:\n%v", output2)
			}
		}
		compat_otp.AssertWaitPollNoErr(err1, "max time reached waiting for exactly 1 DNS pod to exist")
		err2 := WaitForPodWithLabelReady(oc, ns, dnsPodLabel)
		if err2 != nil {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", dnsPodLabel).Output()
			e2e.Logf("All current dns pods are:\n%v", output)
		}
		compat_otp.AssertWaitPollNoErr(err2, fmt.Sprintf("max time reached but no dns pod ready"))
	}
	return GetDNSPodName(oc)
}

const (
	defaultDNSName   = "default"
	dnsTestNodeLabel = "nid-dns-testing"
)

type nodeLabelState struct {
	value   string
	present bool
}

type dnsConfigSnapshot struct {
	spec       operatorv1.DNSSpec
	nodeLabels map[string]nodeLabelState
}

// PreserveDNSConfig captures the DNS spec and test node labels before a mutation.
// Call the returned cleanup function after restoring any stopped operators. It restores
// the original values without deleting the DNS CR or overwriting unrelated node labels.
func PreserveDNSConfig(oc *exutil.CLI) func() {
	dnsClient := oc.AdminOperatorClient().OperatorV1().DNSes()
	nodeClient := oc.AdminKubeClient().CoreV1().Nodes()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	snapshot, err := captureDNSConfig(ctx, dnsClient, nodeClient)
	cancel()
	o.Expect(err).NotTo(o.HaveOccurred(), "failed to capture DNS configuration before changing it")

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
		defer cancel()
		err := snapshot.restore(ctx, dnsClient, nodeClient)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to restore the original DNS configuration")
		EnsureClusterOperatorNormal(oc, "dns", 2, 120)
	}
}

func captureDNSConfig(ctx context.Context, dnsClient operatorclient.DNSInterface, nodeClient coreclient.NodeInterface) (*dnsConfigSnapshot, error) {
	dns, err := dnsClient.Get(ctx, defaultDNSName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("read DNS configuration: %w", err)
	}
	nodes, err := nodeClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("read DNS test node labels: %w", err)
	}
	snapshot := &dnsConfigSnapshot{
		spec:       *dns.Spec.DeepCopy(),
		nodeLabels: make(map[string]nodeLabelState, len(nodes.Items)),
	}
	for _, node := range nodes.Items {
		value, present := node.Labels[dnsTestNodeLabel]
		snapshot.nodeLabels[node.Name] = nodeLabelState{value: value, present: present}
	}
	return snapshot, nil
}

func (snapshot *dnsConfigSnapshot) restore(ctx context.Context, dnsClient operatorclient.DNSInterface, nodeClient coreclient.NodeInterface) error {
	var restoreErrors []error
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		dns, err := dnsClient.Get(ctx, defaultDNSName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		dns.Spec = *snapshot.spec.DeepCopy()
		_, err = dnsClient.Update(ctx, dns, metav1.UpdateOptions{})
		return err
	}); err != nil {
		restoreErrors = append(restoreErrors, fmt.Errorf("restore DNS spec: %w", err))
	}
	// Attempt every restoration even if one resource fails. Update the latest node
	// object so unrelated labels and other concurrent node changes are retained.
	for name, original := range snapshot.nodeLabels {
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			node, err := nodeClient.Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			value, present := node.Labels[dnsTestNodeLabel]
			if present == original.present && value == original.value {
				return nil
			}
			if original.present {
				if node.Labels == nil {
					node.Labels = make(map[string]string)
				}
				node.Labels[dnsTestNodeLabel] = original.value
			} else {
				delete(node.Labels, dnsTestNodeLabel)
			}
			_, err = nodeClient.Update(ctx, node, metav1.UpdateOptions{})
			return err
		}); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore DNS test label on node %s: %w", name, err))
		}
	}
	return errors.NewAggregate(restoreErrors)
}

func GetAllDNSPodsNames(oc *exutil.CLI) []string {
	ns := "openshift-dns"
	label := "dns.operator.openshift.io/daemonset-dns=default"
	var podList []string
	waitErr := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {
		dnsPods, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", "-l", label, "-ojsonpath={.items[*].metadata.name}").Output()
		if err != nil {
			e2e.Logf("Failed to get DNS pods, retrying... error: %v", err)
			return false, nil
		}
		if dnsPods == "" {
			e2e.Logf("No DNS pods found yet, retrying...")
			return false, nil
		}
		podList = strings.Split(dnsPods, " ")
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, "timed out waiting for DNS pods to be available")
	e2e.Logf("The DNS pod list is %v", podList)
	return podList
}

func WaitForDNSPodCount(oc *exutil.CLI, count int) error {
	ns := "openshift-dns"
	dnsPodLabel := "dns.operator.openshift.io/daemonset-dns=default"
	return wait.Poll(20*time.Second, 7*time.Minute, func() (bool, error) {
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pod", "-l", dnsPodLabel, "-ojsonpath={.items[*].metadata.name}").Output()
		if err != nil {
			e2e.Logf("failed to get DNS pods: %v, retrying...", err)
			return false, nil
		}
		if output == "" {
			e2e.Logf("no DNS pods found, retrying...")
			return false, nil
		}
		pods := strings.Split(output, " ")
		e2e.Logf("current DNS pod count: %d (want %d), pods: %v", len(pods), count, pods)
		return len(pods) == count, nil
	})
}

func GetRandomElementFromList(list []string) string {
	return list[rand.Intn(len(list))]
}

func WaitForRangeOfPodsToDisappear(oc *exutil.CLI, resource string, podList []string) {
	for _, podName := range podList {
		err := WaitForResourceToDisappear(oc, resource, "pod/"+podName)
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("%s pod %s is NOT deleted", resource, podName))
	}
}

func WaitForOutputEquals(oc *exutil.CLI, ns, resourceName, jsonPath, expected string, args ...interface{}) {
	waitDuration := 180 * time.Second
	for _, arg := range args {
		duration, ok := arg.(time.Duration)
		if ok {
			waitDuration = duration
		}
	}

	waitErr := wait.PollImmediate(5*time.Second, waitDuration, func() (bool, error) {
		output := GetByJsonPath(oc, ns, resourceName, jsonPath)
		if output == expected {
			return true, nil
		}
		e2e.Logf("The output of jsonpath does NOT equal the expected string: %v, retrying...", expected)
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but cannot find the expected string"))
}

func CheckIPStackType(oc *exutil.CLI) string {
	svcNetwork, err := oc.WithoutNamespace().AsAdmin().Run("get").Args("network.operator", "cluster", "-o=jsonpath={.spec.serviceNetwork}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	if strings.Count(svcNetwork, ":") >= 2 && strings.Count(svcNetwork, ".") >= 2 {
		return "dualstack"
	} else if strings.Count(svcNetwork, ":") >= 2 {
		return "ipv6single"
	} else if strings.Count(svcNetwork, ".") >= 2 {
		return "ipv4single"
	}
	return ""
}

func CreateResourceFromFile(oc *exutil.CLI, ns, file string) {
	err := oc.WithoutNamespace().Run("create").Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func OperateResourceFromFile(oc *exutil.CLI, oper, ns, file string) {
	err := oc.AsAdmin().WithoutNamespace().Run(oper).Args("-f", file, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func PatchResourceAsAdminAnyType(oc *exutil.CLI, ns, resource, patch, typ string) {
	err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "-p", patch, "--type="+typ, "-n", ns).Execute()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func GetAnnotation(oc *exutil.CLI, ns, resource, resourceName string) string {
	findAnnotation, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		resource, resourceName, "-n", ns, "-o=jsonpath={.metadata.annotations}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return findAnnotation
}

func DescribePodResource(oc *exutil.CLI, podName, namespace string) string {
	podDescribe, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("pod", podName, "-n", namespace).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	return podDescribe
}

func CreateConfigMapFromFile(oc *exutil.CLI, ns, name, cmFile string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("create").Args("configmap", name, "--from-file="+cmFile, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func DeleteConfigMap(oc *exutil.CLI, ns, name string) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("configmap", name, "-n", ns).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
}

func GetPodv4Address(oc *exutil.CLI, podName, namespace string) string {
	podIPv4, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.status.podIP}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("IP of the %s pod in namespace %s is %q ", podName, namespace, podIPv4)
	return podIPv4
}

func ReadDNSCorefile(oc *exutil.CLI, dnsPodName, searchString, grepOption string) string {
	ns := "openshift-dns"
	cmd := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile %s", searchString, grepOption)
	output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", ns, dnsPodName, "--", "bash", "-c", cmd).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the part of Corefile that matching \"%s\" is: %v", searchString, output)
	return output
}

func KeepSearchInAllDNSPods(oc *exutil.CLI, podList []string, expStr string) {
	cmd := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile", expStr)
	o.Expect(podList).NotTo(o.BeEmpty())
	for _, podName := range podList {
		count := 0
		waitErr := wait.Poll(15*time.Second, 360*time.Second, func() (bool, error) {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", "openshift-dns", podName, "-c", "dns", "--", "bash", "-c", cmd).Output()
			count++
			primary := false
			if strings.Contains(output, expStr) {
				e2e.Logf("find %s in the Corefile of pod %s", expStr, podName)
				primary = true
			} else {
				if count%2 == 1 {
					e2e.Logf("can't find %s in the Corefile of pod %s, wait and try again...", expStr, podName)
				}
			}
			return primary, nil
		})
		compat_otp.AssertWaitPollNoErr(waitErr, "can't find "+expStr+" in the Corefile of pod "+podName)
	}
}

func SearchLogFromDNSPods(oc *exutil.CLI, podList []string, searchStr string) string {
	o.Expect(podList).NotTo(o.BeEmpty())
	for _, podName := range podList {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args(podName, "-c", "dns", "-n", "openshift-dns").Output()
		outputList := strings.Split(output, "\n")
		for _, line := range outputList {
			if strings.Contains(line, searchStr) {
				return line
			}
		}
	}
	return "none"
}

func NslookupsAndWaitForDNSlog(oc *exutil.CLI, podName, searchLog string, dnsPodList []string, nslookupCmdPara ...string) string {
	e2e.Logf("Polling for executing nslookupCmd and waiting the dns logs appear")
	output := ""
	cmd := append([]string{podName, "--", "nslookup"}, nslookupCmdPara...)
	waitErr := wait.Poll(5*time.Second, 300*time.Second, func() (bool, error) {
		oc.Run("exec").Args(cmd...).Execute()
		output = SearchLogFromDNSPods(oc, dnsPodList, searchLog)
		primary := false
		if len(output) > 1 && output != "none" {
			primary = true
		}
		return primary, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached,but expected string \"%s\" is not found in the dns logs", searchLog))
	return output
}

func ReplaceCoreDnsImage(oc *exutil.CLI, file string) {
	coreDnsImage, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co/dns", "-o=jsonpath={.status.versions[?(.name == \"coredns\")].version}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	result, err := exec.Command("bash", "-c", fmt.Sprintf(`grep "image: " %s`, file)).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the result of grep command is: %s", result)
	if strings.Contains(string(result), coreDnsImage) {
		e2e.Logf("the image has been updated, no action and continue")
	} else {
		sedCmd := fmt.Sprintf(`sed -i'' -e 's|replaced-at-runtime|%s|g' %s`, coreDnsImage, file)
		_, err := exec.Command("bash", "-c", sedCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
	}
}

func AddTaint(oc *exutil.CLI, resource, resourceName, taint string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", resource, resourceName, taint).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring(resource + "/" + resourceName + " tainted"))
}

func DeleteTaint(oc *exutil.CLI, resource, resourceName, taint string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("adm").Args("taint", resource, resourceName, taint).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(output).To(o.ContainSubstring(resource + "/" + resourceName + " untainted"))
}

func WaitCoBecomes(oc *exutil.CLI, coName string, waitTime int, expectedStatus map[string]string) error {
	return wait.Poll(10*time.Second, time.Duration(waitTime)*time.Second, func() (bool, error) {
		gottenStatus := GetCoStatus(oc, coName, expectedStatus)
		eq := reflect.DeepEqual(expectedStatus, gottenStatus)
		if eq {
			e2e.Logf("Given operator %s becomes %s", coName, gottenStatus)
			return true, nil
		}
		return false, nil
	})
}

func GetCoStatus(oc *exutil.CLI, coName string, statusToCompare map[string]string) map[string]string {
	newStatusToCompare := make(map[string]string)
	for key := range statusToCompare {
		args := fmt.Sprintf(`-o=jsonpath={.status.conditions[?(.type == '%s')].status}`, key)
		status, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("co", args, coName).Output()
		if err != nil {
			e2e.Logf("Transient error getting CO %s status for %s: %v, will retry...", coName, key, err)
			return newStatusToCompare
		}
		newStatusToCompare[key] = status
	}
	return newStatusToCompare
}

func GetSvcClusterIPByName(oc *exutil.CLI, ns, serviceName string) string {
	clusterIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "svc", serviceName, "-o=jsonpath={.spec.clusterIP}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The '%s' service's clusterIP of '%s' namespace is: %v", serviceName, ns, clusterIP)
	return clusterIP
}

func ReverseString(str string) (result string) {
	for _, i := range str {
		result = string(i) + result
	}
	return
}

func ConvertV6AddressToPTR(ipv6Address string) string {
	addr, err := netip.ParseAddr(ipv6Address)
	o.Expect(err).NotTo(o.HaveOccurred(), fmt.Sprintf("failed to parse IPv6 address: %s", ipv6Address))
	v6AddWithoutSemicolon := strings.Join(strings.Split(addr.StringExpanded(), ":"), "")
	reversedString := ReverseString(v6AddWithoutSemicolon)
	PtrString := strings.Join(strings.SplitAfter(reversedString, ""), ".") + ".ip6.arpa"
	e2e.Logf("The PTR record is %s", PtrString)
	return PtrString
}

func WaitForDebugNodeOutputContains(oc *exutil.CLI, ns, node string, cmdList []string, expectedString string, args ...interface{}) string {
	var output string
	count := 0
	waitDuration := 180 * time.Second
	for _, arg := range args {
		duration, ok := arg.(time.Duration)
		if ok {
			waitDuration = duration
		}
	}

	e2e.Logf("The expected string is: \n%s", expectedString)
	waitErr := wait.Poll(10*time.Second, waitDuration, func() (bool, error) {
		debugOutput, err := compat_otp.DebugNodeRetryWithOptionsAndChroot(oc, node, []string{"--quiet=true", "--to-namespace=" + ns}, cmdList...)
		o.Expect(err).NotTo(o.HaveOccurred())
		if count%5 == 0 {
			e2e.Logf("The output of the cmd executed on the debug node is:\n%s", debugOutput)
		}
		count++
		if !strings.Contains(debugOutput, expectedString) {
			e2e.Logf("Failed to find the expected string, retrying...")
			return false, nil
		}
		output = debugOutput
		e2e.Logf(`Find the expected string in the debug node's output: %s`, debugOutput)
		return true, nil
	})

	compat_otp.AssertWaitPollNoErr(waitErr, "Max time reached, but the expected string was not found by checking the output of cmd executed on the debug node")
	return output
}

func WaitEgressFirewallApplied(oc *exutil.CLI, efName, ns string) string {
	var output string
	checkErr := wait.Poll(10*time.Second, 60*time.Second, func() (bool, error) {
		efOutput, efErr := oc.AsAdmin().WithoutNamespace().Run("get").Args("egressfirewall", "-n", ns, efName).Output()
		if efErr != nil {
			e2e.Logf("Failed to get egressfirewall %v, error: %s. Trying again", efName, efErr)
			return false, nil
		}
		if !strings.Contains(efOutput, "EgressFirewall Rules applied") {
			e2e.Logf("The egressfirewall was not applied, trying again. \n %s", efOutput)
			return false, nil
		}
		output = efOutput
		return true, nil
	})
	compat_otp.AssertWaitPollNoErr(checkErr, "reached max time allowed but cannot find the egressfirewall details.")
	return output
}

func CheckDomainReachability(oc *exutil.CLI, podName, ns, domainName string, passOrFail bool) {
	curlCmd := fmt.Sprintf("curl -s -I %s --connect-timeout 5 --max-time 10 ", domainName)
	if passOrFail {
		extractHost := func(domain string) string {
			h := strings.TrimPrefix(domain, "https://")
			h = strings.TrimPrefix(h, "http://")
			if idx := strings.LastIndex(h, ":"); idx != -1 {
				h = h[:idx]
			}
			return h
		}

		diagCurlOut, diagCurlErr := e2eoutput.RunHostCmd(ns, podName, curlCmd)
		diagHost := extractHost(domainName)
		diagGetentOut, diagGetentErr := e2eoutput.RunHostCmd(ns, podName, fmt.Sprintf("getent ahostsv4 %s", diagHost))
		e2e.Logf("[diag] domain=%s curl_ok=%v getent_ahostsv4_ok=%v curl_out=%q getent_out=%q",
			domainName, diagCurlErr == nil, diagGetentErr == nil, diagCurlOut, diagGetentOut)

		curlSucceeded := diagCurlErr == nil
		if !curlSucceeded {
			for start := time.Now(); time.Since(start) < 120*time.Second; time.Sleep(10 * time.Second) {
				_, err := e2eoutput.RunHostCmd(ns, podName, curlCmd)
				if err == nil {
					curlSucceeded = true
					break
				}
			}
		}

		o.Expect(curlSucceeded).To(o.BeTrue(),
			fmt.Sprintf("curl to %s failed after retries — egress firewall may be blocking the domain", domainName))

		ipStackType := CheckIPStackType(oc)
		if ipStackType == "dualstack" {
			curlCmd = fmt.Sprintf("curl -s -6 -I %s --connect-timeout 5 --max-time 10", domainName)
			diagCurlOut6, diagCurlErr6 := e2eoutput.RunHostCmd(ns, podName, curlCmd)
			diagGetentOut6, diagGetentErr6 := e2eoutput.RunHostCmd(ns, podName, fmt.Sprintf("getent ahostsv6 %s", diagHost))
			e2e.Logf("[diag-ipv6] domain=%s curl_ok=%v getent_ahostsv6_ok=%v curl_out=%q getent_out=%q",
				domainName, diagCurlErr6 == nil, diagGetentErr6 == nil, diagCurlOut6, diagGetentOut6)

			ipv6Succeeded := diagCurlErr6 == nil
			if !ipv6Succeeded {
				for start := time.Now(); time.Since(start) < 120*time.Second; time.Sleep(10 * time.Second) {
					_, err := e2eoutput.RunHostCmd(ns, podName, curlCmd)
					if err == nil {
						ipv6Succeeded = true
						break
					}
				}
			}
			o.Expect(ipv6Succeeded).To(o.BeTrue(),
				fmt.Sprintf("IPv6 curl to %s failed after retries — egress firewall may be blocking the domain", domainName))
		}
	} else {
		o.Eventually(func() error {
			_, err := e2eoutput.RunHostCmd(ns, podName, curlCmd)
			return err
		}, "20s", "10s").Should(o.HaveOccurred())
	}
}

func RepeatCmdOnClient(oc *exutil.CLI, cmd, expectOutput interface{}, duration int, repeatTimes int) (string, []int) {
	var (
		clientType       = "Internal"
		matchedTimesList = []int{}
		successCurlCount = 0
		matchedCount     = 0
		expectOutputList = []string{}
		output           = ""
	)

	cmdStr, ok := cmd.(string)
	if ok {
		clientType = "External"
	}
	cmdList, _ := cmd.([]string)

	expStr, ok := expectOutput.(string)
	if ok {
		expectOutputList = append(expectOutputList, expStr)
	}
	expList, ok := expectOutput.([]string)
	if ok {
		expectOutputList = expList
	}

	for i := 0; i < len(expectOutputList); i++ {
		matchedTimesList = append(matchedTimesList, 0)
	}

	e2e.Logf("Using client type: %v", clientType)
	e2e.Logf("The cmdStr (used by External client) is '%v' and cmdList (used by Internal client) is %v", cmdStr, cmdList)
	e2e.Logf("The expectOutputList is %v and initial matchedTimesList is %v", expectOutputList, matchedTimesList)

	waitErr := wait.Poll(1*time.Second, time.Duration(duration)*time.Second, func() (bool, error) {
		isMatch := false
		if clientType == "Internal" {
			info, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args(cmdList...).Output()
			if err != nil {
				e2e.Logf("The error is: %v", err.Error())
				searchInfo := regexp.MustCompile(expectOutputList[0]).FindStringSubmatch(err.Error())
				if len(searchInfo) > 0 {
					e2e.Logf("The expected string is included in err: %v", err)
					return true, nil
				} else {
					e2e.Logf("Failed to execute cmd and got err %v, retrying...", err.Error())
					return false, nil
				}
			}
			output = info
		} else {
			info, err := exec.Command("bash", "-c", cmdStr).CombinedOutput()
			if err != nil {
				e2e.Logf("The error is: %v", err.Error())
				searchInfo := regexp.MustCompile(expectOutputList[0]).FindStringSubmatch(err.Error())
				if len(searchInfo) > 0 {
					e2e.Logf("The expected string is included in err: %v", err)
					return true, nil
				} else {
					e2e.Logf("Failed to execute cmd and got err %v, retrying...", err.Error())
					return false, nil
				}
			}
			output = string(info)
		}

		successCurlCount++
		e2e.Logf("Executed cmd for %v times on the client and got output: %s", successCurlCount, output)

		for i := 0; i < len(expectOutputList); i++ {
			searchInfo := regexp.MustCompile(expectOutputList[i]).FindStringSubmatch(output)
			if len(searchInfo) > 0 {
				isMatch = true
				matchedCount++
				matchedTimesList[i] = matchedTimesList[i] + 1
				break
			}
		}

		if isMatch {
			e2e.Logf("Successfully executed cmd for %v times on the client, expecting %v times", matchedCount, repeatTimes)
			if matchedCount == repeatTimes {
				return true, nil
			} else {
				return false, nil
			}
		} else {
			successCurlCount--
			e2e.Logf("Failed to find a match in the output, retrying...")
			return false, nil
		}
	})

	e2e.Logf("The matchedTimesList is: %v", matchedTimesList)
	compat_otp.AssertWaitPollNoErr(waitErr, "max time reached but can't execute the cmd successfully for the desired times")

	return output, matchedTimesList
}
