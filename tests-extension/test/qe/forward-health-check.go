package router

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	o "github.com/onsi/gomega"
	util "github.com/openshift/cluster-dns-operator-tests-extension/test"
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
	image := util.GetByJsonPath(oc, dnsNamespace, "daemonset/dns-default", `{.spec.template.spec.containers[?(@.name=="dns")].image}`)
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
		ready := util.GetByJsonPath(oc, namespace, "pod/"+podName, `{.status.conditions[?(@.type=="Ready")].status}`)
		return ready == "True", nil
	})
	compat_otp.AssertWaitPollNoErr(err, fmt.Sprintf("max time reached but pod %s/%s is not ready", namespace, podName))
}

func getServiceClusterIP(oc *exutil.CLI, serviceName string) string {
	ip := util.GetByJsonPath(oc, dnsNamespace, "service/"+serviceName, "{.spec.clusterIP}")
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
	util.PatchGlobalResourceAsAdmin(oc, resourceName, string(patch))
}

func waitForForwardHealthCheckCorefile(oc *exutil.CLI, dnsPodName, upstream string) {
	util.PollReadDnsCorefile(oc, dnsPodName, upstream, "-A2", upstream)
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
