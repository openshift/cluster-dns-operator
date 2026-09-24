package router

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	"k8s.io/apimachinery/pkg/util/wait"
	e2e "k8s.io/kubernetes/test/e2e/framework"
)

const (
	dnsNamespace                                = "openshift-dns"
	forwardHealthCheckHealthyUpstreamName       = "forward-healthcheck-alert-healthy"
	forwardHealthCheckUnreachableUpstreamOne    = "forward-healthcheck-alert-unreachable-one"
	forwardHealthCheckUnreachableUpstreamTwo    = "forward-healthcheck-alert-unreachable-two"
	forwardHealthCheckNetworkPolicyName         = "forward-healthcheck-alert-allow"
	forwardHealthCheckUpstreamLabel             = "forward-healthcheck-alert-upstream"
	forwardHealthCheckHealthyUpstreamLabelValue = "healthy"
)

type prometheusInstantQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
		} `json:"result"`
	} `json:"data"`
}

// getDNSContainerImage returns the image currently used by the managed
// CoreDNS daemonset. Reusing it keeps the test upstream compatible with the
// payload under test and avoids an external image dependency.
func getDNSContainerImage(oc *exutil.CLI) string {
	image := getByJsonPath(oc, dnsNamespace, "daemonset/dns-default", `{.spec.template.spec.containers[?(@.name=="dns")].image}`)
	o.Expect(image).NotTo(o.BeEmpty())
	return image
}

// createForwardHealthCheckAlertResources creates one healthy CoreDNS upstream
// and two ClusterIP services with no endpoints. All resources remain entirely
// in-cluster, so neither alert phase depends on public DNS reachability.
func createForwardHealthCheckAlertResources(oc *exutil.CLI, coreDNSImage string) (string, string, string) {
	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
  namespace: %s
data:
  Corefile: |
    .:5353 {
        hosts {
            192.0.2.42 healthcheck-alerts.test
        }
        health
        errors
        log
    }
---
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
  labels:
    %s: %s
    type: test-pod
spec:
  serviceAccountName: dns
  containers:
  - name: coredns-upstream
    image: %q
    command: ["coredns"]
    args: ["-conf", "/etc/coredns/Corefile"]
    ports:
    - name: dns
      containerPort: 5353
      protocol: UDP
    - name: dns-tcp
      containerPort: 5353
      protocol: TCP
    readinessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 10
      timeoutSeconds: 10
    livenessProbe:
      httpGet:
        path: /health
        port: 8080
      initialDelaySeconds: 10
      timeoutSeconds: 10
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: ["ALL"]
      runAsNonRoot: false
      seccompProfile:
        type: RuntimeDefault
    volumeMounts:
    - name: config-volume
      mountPath: /etc/coredns
      readOnly: true
  volumes:
  - name: config-volume
    configMap:
      name: %s
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    %s: %s
  ports:
  - name: dns
    port: 53
    protocol: UDP
    targetPort: 5353
  - name: dns-tcp
    port: 53
    protocol: TCP
    targetPort: 5353
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    %s: no-endpoints
  ports:
  - name: dns
    port: 53
    protocol: UDP
    targetPort: 5353
  - name: dns-tcp
    port: 53
    protocol: TCP
    targetPort: 5353
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    %s: no-endpoints
  ports:
  - name: dns
    port: 53
    protocol: UDP
    targetPort: 5353
  - name: dns-tcp
    port: 53
    protocol: TCP
    targetPort: 5353
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: %s
spec:
  podSelector:
    matchLabels:
      type: test-pod
  policyTypes: ["Ingress", "Egress"]
  ingress:
  - from:
    - ipBlock:
        cidr: 0.0.0.0/0
    - ipBlock:
        cidr: ::/0
  egress:
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
    - ipBlock:
        cidr: ::/0
`, forwardHealthCheckHealthyUpstreamName, dnsNamespace,
		forwardHealthCheckHealthyUpstreamName, dnsNamespace, forwardHealthCheckUpstreamLabel, forwardHealthCheckHealthyUpstreamLabelValue, coreDNSImage, forwardHealthCheckHealthyUpstreamName,
		forwardHealthCheckHealthyUpstreamName, dnsNamespace, forwardHealthCheckUpstreamLabel, forwardHealthCheckHealthyUpstreamLabelValue,
		forwardHealthCheckUnreachableUpstreamOne, dnsNamespace, forwardHealthCheckUpstreamLabel,
		forwardHealthCheckUnreachableUpstreamTwo, dnsNamespace, forwardHealthCheckUpstreamLabel,
		forwardHealthCheckNetworkPolicyName, dnsNamespace)
	applyManifestAsAdmin(oc, manifest)
	waitForNamedPodReady(oc, dnsNamespace, forwardHealthCheckHealthyUpstreamName)

	healthyUpstream := getServiceClusterIP(oc, forwardHealthCheckHealthyUpstreamName)
	unreachableUpstreamOne := getServiceClusterIP(oc, forwardHealthCheckUnreachableUpstreamOne)
	unreachableUpstreamTwo := getServiceClusterIP(oc, forwardHealthCheckUnreachableUpstreamTwo)
	ensureServiceHasNoEndpoints(oc, forwardHealthCheckUnreachableUpstreamOne)
	ensureServiceHasNoEndpoints(oc, forwardHealthCheckUnreachableUpstreamTwo)
	return healthyUpstream, unreachableUpstreamOne, unreachableUpstreamTwo
}

func applyManifestAsAdmin(oc *exutil.CLI, manifest string) {
	output, err := oc.AsAdmin().WithoutNamespace().Run("apply").Args("-f", "-").InputString(manifest).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("Applied forward health check alert resources: %s", output)
}

func waitForNamedPodReady(oc *exutil.CLI, namespace, podName string) {
	err := wait.PollImmediate(5*time.Second, 3*time.Minute, func() (bool, error) {
		ready := getByJsonPath(oc, namespace, "pod/"+podName, `{.status.conditions[?(@.type=="Ready")].status}`)
		return ready == "True", nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but pod %s/%s is not ready", namespace, podName))
}

func getServiceClusterIP(oc *exutil.CLI, serviceName string) string {
	ip := getByJsonPath(oc, dnsNamespace, "service/"+serviceName, "{.spec.clusterIP}")
	o.Expect(ip).NotTo(o.BeEmpty())
	return ip
}

func ensureServiceHasNoEndpoints(oc *exutil.CLI, serviceName string) {
	stdout, _, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
		"-n", dnsNamespace, "endpointslices.discovery.k8s.io",
		"-l", "kubernetes.io/service-name="+serviceName,
		"-o=jsonpath={.items[*].endpoints}").Outputs()
	o.Expect(err).NotTo(o.HaveOccurred())
	o.Expect(strings.TrimSpace(stdout)).To(o.BeEmpty(), "service %s must have no endpoints", serviceName)
}

func patchForwardHealthCheckAlertServer(oc *exutil.CLI, resourceName, zone string, upstreams []string, operation string) {
	var path string
	var value any
	if operation == "add" {
		path = "/spec/servers"
		value = []map[string]any{{
			"name":  "forward-healthcheck-alerts",
			"zones": []string{zone},
			"forwardPlugin": map[string]any{
				"policy":    "RoundRobin",
				"upstreams": upstreams,
			},
		}}
	} else {
		path = "/spec/servers/0/forwardPlugin/upstreams"
		value = upstreams
	}
	patch, err := json.Marshal([]map[string]any{{"op": operation, "path": path, "value": value}})
	o.Expect(err).NotTo(o.HaveOccurred())
	patchGlobalResourceAsAdmin(oc, resourceName, string(patch))
}

func waitForForwardHealthCheckCorefile(oc *exutil.CLI, dnsPodName, upstream string) {
	pollReadDnsCorefile(oc, dnsPodName, upstream, "-A2", upstream)
}

func issueForwardHealthCheckQueries(oc *exutil.CLI, dnsPodName, zone string) {
	for range 3 {
		output, err := oc.AsAdmin().WithoutNamespace().Run("exec").Args("-n", dnsNamespace, dnsPodName, "--", "dig", "@127.0.0.1", "-p", "5353", "+time=1", "+tries=1", zone, "A").Output()
		if err != nil {
			e2e.Logf("DNS query for %s returned an expected forwarding error: %v (%s)", zone, err, output)
		}
	}
}

func firingAlertMetrics(monitor compat_otp.Monitorer, alertName string) ([]map[string]string, error) {
	query := fmt.Sprintf(`ALERTS{alertname=%q,alertstate="firing"}`, alertName)
	response, err := monitor.SimpleQuery(query)
	if err != nil {
		return nil, err
	}
	var parsed prometheusInstantQueryResponse
	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse Prometheus response for %s: %w", alertName, err)
	}
	if parsed.Status != "success" || parsed.Data.ResultType != "vector" {
		return nil, fmt.Errorf("unexpected Prometheus response for %s: %s", alertName, response)
	}
	metrics := make([]map[string]string, 0, len(parsed.Data.Result))
	for _, result := range parsed.Data.Result {
		metrics = append(metrics, result.Metric)
	}
	return metrics, nil
}

func waitForFiringAlertForUpstream(monitor compat_otp.Monitorer, oc *exutil.CLI, dnsPodName, zone, alertName, upstream string, timeout time.Duration) {
	err := wait.PollImmediate(10*time.Second, timeout, func() (bool, error) {
		issueForwardHealthCheckQueries(oc, dnsPodName, zone)
		metrics, err := firingAlertMetrics(monitor, alertName)
		if err != nil {
			e2e.Logf("failed to query firing alert %s: %v", alertName, err)
			return false, nil
		}
		for _, metric := range metrics {
			if strings.Contains(metric["to"], upstream) {
				e2e.Logf("alert %s is firing for upstream %s", alertName, metric["to"])
				return true, nil
			}
		}
		e2e.Logf("alert %s is not yet firing for upstream %s; observed: %#v", alertName, upstream, metrics)
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("timed out waiting for alert %s for upstream %s", alertName, upstream))
}

func waitForFiringAlert(monitor compat_otp.Monitorer, oc *exutil.CLI, dnsPodName, zone, alertName string, timeout time.Duration) {
	err := wait.PollImmediate(10*time.Second, timeout, func() (bool, error) {
		issueForwardHealthCheckQueries(oc, dnsPodName, zone)
		metrics, err := firingAlertMetrics(monitor, alertName)
		if err != nil {
			e2e.Logf("failed to query firing alert %s: %v", alertName, err)
			return false, nil
		}
		if len(metrics) > 0 {
			e2e.Logf("alert %s is firing: %#v", alertName, metrics)
			return true, nil
		}
		e2e.Logf("alert %s is not yet firing", alertName)
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("timed out waiting for alert %s", alertName))
}

func assertNoFiringAlert(monitor compat_otp.Monitorer, alertName string) {
	// Poll for 30s to absorb Prometheus scrape and evaluation jitter before
	// concluding the alert is genuinely not firing.
	o.Consistently(func(g o.Gomega) {
		metrics, err := firingAlertMetrics(monitor, alertName)
		g.Expect(err).NotTo(o.HaveOccurred())
		g.Expect(metrics).To(o.BeEmpty(), "alert %s unexpectedly fired: %#v", alertName, metrics)
	}, 30*time.Second, 10*time.Second).Should(o.Succeed())
}

func assertNoFiringAlertForUpstreams(monitor compat_otp.Monitorer, alertName string, upstreams ...string) {
	// Poll for 30s to absorb Prometheus scrape and evaluation jitter before
	// concluding the alert is genuinely not firing for any of the given upstreams.
	o.Consistently(func(g o.Gomega) {
		metrics, err := firingAlertMetrics(monitor, alertName)
		g.Expect(err).NotTo(o.HaveOccurred())
		for _, metric := range metrics {
			for _, upstream := range upstreams {
				g.Expect(metric["to"]).NotTo(o.ContainSubstring(upstream), "alert %s must be suppressed for unreachable upstream %s", alertName, upstream)
			}
		}
	}, 30*time.Second, 10*time.Second).Should(o.Succeed())
}

func cleanupForwardHealthCheckAlertResources(oc *exutil.CLI) {
	resources := []string{
		"service/" + forwardHealthCheckHealthyUpstreamName,
		"service/" + forwardHealthCheckUnreachableUpstreamOne,
		"service/" + forwardHealthCheckUnreachableUpstreamTwo,
		"pod/" + forwardHealthCheckHealthyUpstreamName,
		"configmap/" + forwardHealthCheckHealthyUpstreamName,
		"networkpolicy/" + forwardHealthCheckNetworkPolicyName,
	}
	var cleanupErrors []string
	for _, resource := range resources {
		output, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("-n", dnsNamespace, resource, "--ignore-not-found").Output()
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Sprintf("%s: %v (%s)", resource, err, output))
		}
	}
	o.Expect(cleanupErrors).To(o.BeEmpty(), "failed to delete forward health check alert resources")
}

func getRandomString() string {
	chars := "abcdefghijklmnopqrstuvwxyz0123456789"
	buffer := make([]byte, 8)
	for index := range buffer {
		buffer[index] = chars[rand.Intn(len(chars))]
	}
	return string(buffer)
}

func waitForPodWithLabelReady(oc *exutil.CLI, ns, label string) error {
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

func ensurePodWithLabelReady(oc *exutil.CLI, ns, label string) {
	err := waitForPodWithLabelReady(oc, ns, label)
	if err != nil {
		output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", "-n", ns, "-l", label).Output()
		e2e.Logf("All pods with label %v are:\n%v", label, output)
		logs, _ := oc.AsAdmin().WithoutNamespace().Run("logs").Args("-n", ns, "-l", label, "--tail=10").Output()
		e2e.Logf("The logs of all labeled pods are:\n%v", logs)
	}
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but the pods with label %v are not ready", label))
}

func waitForResourceToDisappear(oc *exutil.CLI, ns, rsname string) error {
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

func patchGlobalResourceAsAdmin(oc *exutil.CLI, resource, patch string) {
	patchOut, err := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resource, "--patch="+patch, "--type=json").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The output from the patch is:- %q ", patchOut)
}

func getByJsonPath(oc *exutil.CLI, ns, resource, jsonPath string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, resource, "-o=jsonpath="+jsonPath).Output()
	if err != nil {
		e2e.Logf("the error is: %v", err.Error())
	}
	e2e.Logf("the output filtered by jsonpath is: %v", output)
	return output
}

func getByLabelAndJsonPath(oc *exutil.CLI, ns, resource, label, jsonPath string) string {
	output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, resource, "-l", label, "-ojsonpath="+jsonPath).Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("the output filtered by label and jsonpath is: %v", output)
	return output
}

func getNodeNameByPod(oc *exutil.CLI, namespace string, podName string) string {
	nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pod", podName, "-n", namespace, "-o=jsonpath={.spec.nodeName}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The nodename for pod %s in namespace %s is %s", podName, namespace, nodeName)
	return nodeName
}

func getPodListByLabel(oc *exutil.CLI, namespace string, label string) []string {
	var podList []string
	podNameAll, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", namespace, "pod", "-l", label, "-ojsonpath={.items..metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	podList = strings.Split(podNameAll, " ")
	e2e.Logf("The pod list is %v", podList)
	return podList
}

func getDNSPodName(oc *exutil.CLI) string {
	ns := "openshift-dns"
	podName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("-n", ns, "pods", "-l", "dns.operator.openshift.io/daemonset-dns=default", "-o=jsonpath={.items[0].metadata.name}").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	e2e.Logf("The DNS pod name is: %v", podName)
	return podName
}

func pollReadDnsCorefile(oc *exutil.CLI, dnsPodName, searchString1, grepOption, searchString2 string) string {
	e2e.Logf("Polling and search dns Corefile")
	ns := "openshift-dns"
	cmd1 := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile %s | grep \"%s\"", searchString1, grepOption, searchString2)
	cmd2 := fmt.Sprintf("grep \"%s\" /etc/coredns/Corefile %s", searchString1, grepOption)

	waitErr := wait.PollImmediate(5*time.Second, 120*time.Second, func() (bool, error) {
		hackAnnotatePod(oc, ns, dnsPodName)
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

func hackAnnotatePod(oc *exutil.CLI, ns, podName string) {
	hackAnnotation := "ne-testing-hack=" + getRandomString()
	oc.AsAdmin().WithoutNamespace().Run("annotate").Args("pod", podName, "-n", ns, hackAnnotation, "--overwrite").Execute()
}

func ensureClusterOperatorNormal(oc *exutil.CLI, coName string, healthyThreshold int, totalWaitTime time.Duration) {
	count := 0
	printCount := 0
	jsonPath := `{.status.conditions[?(@.type=="Available")].status}{.status.conditions[?(@.type=="Progressing")].status}{.status.conditions[?(@.type=="Degraded")].status}`

	e2e.Logf("waiting for CO %v back to normal status......", coName)
	waitErr := wait.PollImmediate(5*time.Second, totalWaitTime*time.Second, func() (bool, error) {
		status := getByJsonPath(oc, "default", "co/"+coName, jsonPath)
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
		output := getByJsonPath(oc, "default", "co/"+coName, "{.status.conditions}")
		e2e.Logf("The co %v is abnormal and here is status: %v", coName, output)
		if coName == "ingress" {
			output, _ = oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", "openshift-ingress", "service", "router-default").Output()
			e2e.Logf("The output of describe router-default service: %v", output)
		}
	}
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("reached max time allowed but CO %v is still abnoraml.", coName))
}

func forceOnlyOneDnsPodExist(oc *exutil.CLI) string {
	ns := "openshift-dns"
	dnsPodLabel := "dns.operator.openshift.io/daemonset-dns=default"
	dnsNodeSelector := `[{"op":"replace", "path":"/spec/nodePlacement/nodeSelector", "value":{"nid-dns-testing":"true"}}]`
	oc.AsAdmin().WithoutNamespace().Run("label").Args("node", "-l", "nid-dns-testing=true", "nid-dns-testing-").Execute()
	podList := getAllDNSPodsNames(oc)
	if len(podList) == 1 {
		e2e.Logf("Found only one dns-default pod and it looks like SNO cluster. Continue the test...")
	} else {
		dnsPodName := getRandomElementFromList(podList)
		nodeName, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", dnsPodName, "-o=jsonpath={.spec.nodeName}", "-n", ns).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Find random dns pod '%s' and its node '%s' which will be used for the following testing", dnsPodName, nodeName)
		_, err = oc.AsAdmin().WithoutNamespace().Run("label").Args("node", nodeName, "nid-dns-testing=true").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		patchGlobalResourceAsAdmin(oc, "dnses.operator.openshift.io/default", dnsNodeSelector)
		err1 := waitForResourceToDisappear(oc, ns, "pod/"+dnsPodName)
		if err1 != nil {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", dnsPodLabel).Output()
			e2e.Logf("All current dns pods are:\n%v", output)
		}
		compat_otp.AssertWaitPollNoErr(err1, fmt.Sprintf("max time reached but pod %s is not terminated", dnsPodName))
		err2 := waitForPodWithLabelReady(oc, ns, dnsPodLabel)
		if err2 != nil {
			output, _ := oc.AsAdmin().WithoutNamespace().Run("get").Args("pods", "-n", ns, "-l", dnsPodLabel).Output()
			e2e.Logf("All current dns pods are:\n%v", output)
		}
		compat_otp.AssertWaitPollNoErr(err2, fmt.Sprintf("max time reached but no dns pod ready"))
	}
	return getDNSPodName(oc)
}

func deleteDnsOperatorToRestore(oc *exutil.CLI) {
	_, err := oc.AsAdmin().WithoutNamespace().Run("delete").Args("dnses.operator.openshift.io/default").Output()
	o.Expect(err).NotTo(o.HaveOccurred())
	ensureClusterOperatorNormal(oc, "dns", 2, 120)
	oc.AsAdmin().WithoutNamespace().Run("label").Args("node", "-l", "nid-dns-testing=true", "nid-dns-testing-").Execute()
}

func getAllDNSPodsNames(oc *exutil.CLI) []string {
	ns := "openshift-dns"
	label := "dns.operator.openshift.io/daemonset-dns=default"
	dnsPods := getByLabelAndJsonPath(oc, ns, "pod", label, "{.items[*].metadata.name}")
	return strings.Split(dnsPods, " ")
}

func getRandomElementFromList(list []string) string {
	return list[rand.Intn(len(list))]
}

func waitForRangeOfPodsToDisappear(oc *exutil.CLI, resource string, podList []string) {
	for _, podName := range podList {
		err := waitForResourceToDisappear(oc, resource, "pod/"+podName)
		compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("%s pod %s is NOT deleted", resource, podName))
	}
}

func waitForOutputEquals(oc *exutil.CLI, ns, resourceName, jsonPath, expected string, args ...interface{}) {
	waitDuration := 180 * time.Second
	for _, arg := range args {
		duration, ok := arg.(time.Duration)
		if ok {
			waitDuration = duration
		}
	}

	waitErr := wait.PollImmediate(5*time.Second, waitDuration, func() (bool, error) {
		output := getByJsonPath(oc, ns, resourceName, jsonPath)
		if output == expected {
			return true, nil
		}
		e2e.Logf("The output of jsonpath does NOT equal the expected string: %v, retrying...", expected)
		return false, nil
	})
	compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but cannot find the expected string"))
}
