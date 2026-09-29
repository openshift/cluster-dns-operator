package router

import (
	"fmt"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
)

var _ = g.Describe("[OTP][sig-network-edge] Network_Edge Component_DNS", func() {
	defer g.GinkgoRecover()
	oc := exutil.NewCLIWithoutNamespace("dns-operator")

	// Incorporate OCP-26151 and OCP-23278 into one
	// Test case creater: hongli@redhat.com - OCP-26151-Integrate DNS operator metrics with Prometheus
	// Test case creater: hongli@redhat.com - OCP-23278-Integrate coredns metrics with monitoring component
	g.It("Author:mjoseph-Critical-26151-Integrate DNS operator metrics with Prometheus [Skipped:MicroShift]", func() {
		var (
			ons        = "openshift-dns-operator"
			dns        = "openshift-dns"
			label      = `"openshift.io/cluster-monitoring":"true"`
			prometheus = "prometheus-k8s"
		)
		// OCP-26151
		compat_otp.By("1. Check the `cluster-monitoring` label exist in the dns operator namespace")
		oplabels := getByJsonPath(oc, ons, "ns/openshift-dns-operator", "{.metadata.labels}")
		o.Expect(oplabels).To(o.ContainSubstring(label))

		compat_otp.By("2. Check whether servicemonitor exist in the dns operator namespace")
		smname := getByJsonPath(oc, ons, "servicemonitor/dns-operator", "{.metadata.name}")
		o.Expect(smname).To(o.ContainSubstring("dns-operator"))

		compat_otp.By("3. Check whether rolebinding exist in the dns operator namespace")
		rbName := getByJsonPath(oc, ons, "rolebinding/prometheus-k8s", "{.metadata.name}")
		o.Expect(rbName).To(o.ContainSubstring(prometheus))

		// OCP-23278
		// Bug: 1688969
		compat_otp.By("4. Check the `cluster-monitoring` label exist in the dns namespace")
		nsLabels := getByJsonPath(oc, dns, "ns/openshift-dns", "{.metadata.labels}")
		o.Expect(nsLabels).To(o.ContainSubstring(label))

		compat_otp.By("5. Check whether servicemonitor exist in the dns namespace")
		smname1 := getByJsonPath(oc, dns, "servicemonitor/dns-default", "{.metadata.name}")
		o.Expect(smname1).To(o.ContainSubstring("dns-default"))

		compat_otp.By("6. Check whether rolebinding exist in the dns namespace")
		rbName1 := getByJsonPath(oc, dns, "rolebinding/prometheus-k8s", "{.metadata.name}")
		o.Expect(rbName1).To(o.ContainSubstring(prometheus))
	})

	// Test case creater: hongli@redhat.com
	// No dns operator namespace on HyperShift guest cluster so this case is not available
	g.It("Author:mjoseph-NonHyperShiftHOST-High-37912-DNS operator should show clear error message when DNS service IP already allocated [Disruptive] [Serial] [Skipped:MicroShift]", func() {
		// Bug: 1813062, OCPBUGS-14346
		// Store the clusterip from the cluster
		clusterIp := getByJsonPath(oc, "openshift-dns", "service/dns-default", "{.spec.clusterIP}")

		compat_otp.By("1. Scale the CVO and DNS operator pod to zero and delete the default DNS service")
		dnsOperatorPodName := getPodListByLabel(oc, "openshift-dns-operator", "name=dns-operator")[0]
		defer func() {
			compat_otp.By("Recover the CVO and DNS")
			_, errCVO := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment/cluster-version-operator", "--replicas=1", "-n", "openshift-cluster-version").Output()
			o.Expect(errCVO).NotTo(o.HaveOccurred())
			_, errDNS := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment/dns-operator", "--replicas=1", "-n", "openshift-dns-operator").Output()
			o.Expect(errDNS).NotTo(o.HaveOccurred())
			deleteDnsOperatorToRestore(oc)
		}()
		_, err := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment/cluster-version-operator", "--replicas=0", "-n", "openshift-cluster-version").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		_, err = oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment/dns-operator", "--replicas=0", "-n", "openshift-dns-operator").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		errPodDis := waitForResourceToDisappear(oc, "openshift-dns-operator", "pod/"+dnsOperatorPodName)
		compat_otp.AssertWaitPollNoErr(errPodDis, fmt.Sprintf("resource %v does not disapper", "pod/"+dnsOperatorPodName))
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "dns-default", "-n", "openshift-dns").Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())
		err = waitForResourceToDisappear(oc, "openshift-dns", "service/dns-default")
		compat_otp.AssertWaitPollNoErr(err, "the service/dns-default does not disapper within allowed time")

		compat_otp.By("2. Create a test server with the Cluster IP and scale up the dns operator")
		defer func() {
			errSvcDel := oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "svc-37912", "-n", "openshift-dns").Execute()
			o.Expect(errSvcDel).NotTo(o.HaveOccurred())
		}()
		err2 := oc.AsAdmin().WithoutNamespace().Run("create").Args(
			"svc", "clusterip", "svc-37912", "--tcp=53:53", "--clusterip="+clusterIp, "-n", "openshift-dns").Execute()
		o.Expect(err2).NotTo(o.HaveOccurred())
		_, errScale := oc.AsAdmin().WithoutNamespace().Run("scale").Args("deployment/dns-operator", "--replicas=1", "-n", "openshift-dns-operator").Output()
		o.Expect(errScale).NotTo(o.HaveOccurred())
		// wait for the dns operator pod to come up
		ensurePodWithLabelReady(oc, "openshift-dns-operator", "name=dns-operator")

		compat_otp.By("3. Confirm the new dns service came with the given address")
		newClusterIp := getByJsonPath(oc, "openshift-dns", "service/svc-37912", "{.spec.clusterIP}")
		o.Expect(newClusterIp).To(o.BeEquivalentTo(clusterIp))

		compat_otp.By("4. Verify DNS operator does not report Degraded=True while Progressing=True (OCPBUGS-14346)")
		// As per the fix of OCPBUGS-14346, the operator will not show degraded when it is simply progressing
		// Expected: Available=False, Progressing=True, Degraded=False
		jsonPath := `{.status.conditions[?(@.type=="Available")].status}{.status.conditions[?(@.type=="Progressing")].status}{.status.conditions[?(@.type=="Degraded")].status}`
		waitForOutputEquals(oc, "default", "co/dns", jsonPath, "FalseTrueFalse")

		compat_otp.By("5. Confirm the error message from the DNS operator status")
		outputOpcfg, errOpcfg := oc.AsAdmin().WithoutNamespace().Run("get").Args(
			"dns.operator", "default", `-o=jsonpath={.status.conditions[?(@.type=="Available")].message}`).Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("No IP address is assigned to the DNS service"))
	})

	g.It("Author:mjoseph-Critical-41049-DNS controls pod placement by node selector [Disruptive] [Serial]", func() {
		podList := getAllDNSPodsNames(oc)
		if len(podList) == 1 {
			g.Skip("Skipping on SNO cluster (just has one dns pod)")
		}

		var (
			ns       = "openshift-dns"
			label    = "nid-dns-testing=true"
			labelKey = "nid-dns-testing"
		)

		compat_otp.By("1. Check the default dns nodeSelector is present")
		nodePlacement := getByJsonPath(oc, ns, "ds/dns-default", "{.spec.template.spec.nodeSelector}")
		o.Expect(nodePlacement).To(o.BeEquivalentTo(`{"kubernetes.io/os":"linux"}`))

		// Since func forceOnlyOneDnsPodExist() has implemented DNS nodeSelector
		// just ensure the pod is running on the node with label "nid-dns-testing=true"
		compat_otp.By("2. Check the nodeSelector can be updated")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)
		nodePlacement = getByJsonPath(oc, ns, "ds/dns-default", "{.spec.template.spec.nodeSelector}")
		o.Expect(nodePlacement).To(o.ContainSubstring(labelKey))

		compat_otp.By("3. Check the dns pod is running on the expected node")
		nodeByPod := getByJsonPath(oc, ns, "pod/"+oneDnsPod, "{.spec.nodeName}")
		nodeByLabel := getByLabelAndJsonPath(oc, "default", "node", label, "{.items[*].metadata.name}")
		o.Expect(nodeByPod).To(o.BeEquivalentTo(nodeByLabel))
	})

	g.It("Author:mjoseph-Critical-41050-DNS controller pod placement by tolerations [Disruptive] [Serial]", func() {
		// the case needs at least one worker node since dns pods will be removed from master
		// so skip on SNO and Compact cluster that no dedicated worker node
		output, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("nodes", "-l node-role.kubernetes.io/worker=,node-role.kubernetes.io/master!=", `-ojsonpath={.items[*].status.conditions[?(@.type=="Ready")].status}`).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		if strings.Count(output, "True") < 1 {
			g.Skip("Skipping as there is no dedicated worker nodes")
		}

		var (
			ns                  = "openshift-dns"
			dnsCustomToleration = `[{"op":"replace", "path":"/spec/nodePlacement", "value":{"tolerations":[{"effect":"NoExecute","key":"my-dns-test","operator":"Equal","value":"abc"}]}}]`
		)

		compat_otp.By("1. Check dns pod placement to confirm it is running on default tolerations")
		tolerationCfg := getByJsonPath(oc, ns, "ds/dns-default", "{.spec.template.spec.tolerations}")
		o.Expect(tolerationCfg).To(o.ContainSubstring(`{"key":"node-role.kubernetes.io/master","operator":"Exists"}`))

		compat_otp.By("2. Patch dns operator config with custom tolerations of dns pod, not to tolerate master node taints")
		dnsPodsList := getAllDNSPodsNames(oc)
		jsonPath := `{.status.conditions[?(@.type=="Available")].status}{.status.conditions[?(@.type=="Progressing")].status}{.status.conditions[?(@.type=="Degraded")].status}`
		defer deleteDnsOperatorToRestore(oc)
		patchGlobalResourceAsAdmin(oc, "dns.operator.openshift.io/default", dnsCustomToleration)
		waitForRangeOfPodsToDisappear(oc, ns, dnsPodsList)
		waitForOutputEquals(oc, "default", "co/dns", jsonPath, "TrueFalseFalse")
		// Get new created DNS pods and ensure they are not running on master
		dnsPodsList = getAllDNSPodsNames(oc)
		for _, podName := range dnsPodsList {
			nodeName := getNodeNameByPod(oc, ns, podName)
			nodeLabels := getByJsonPath(oc, "default", "node/"+nodeName, "{.metadata.labels}")
			o.Expect(nodeLabels).NotTo(o.ContainSubstring("node-role.kubernetes.io/master"))
		}

		compat_otp.By("3. Check dns pod placement to check the custom tolerations")
		tolerationCfg = getByJsonPath(oc, ns, "ds/dns-default", "{.spec.template.spec.tolerations}")
		o.Expect(tolerationCfg).To(o.ContainSubstring(`{"effect":"NoExecute","key":"my-dns-test","operator":"Equal","value":"abc"}`))

		compat_otp.By("4. Check dns.operator status to see any error messages")
		status := getByJsonPath(oc, "default", "dns.operator/default", "{.status}")
		o.Expect(status).NotTo(o.ContainSubstring("error"))
	})

	g.It("Author:hongli-High-46183-DNS operator supports Random, RoundRobin and Sequential policy for servers.forwardPlugin [Disruptive] [Serial]", func() {
		resourceName := "dns.operator.openshift.io/default"
		jsonPatch := "[{\"op\":\"add\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Random\",\"upstreams\":[\"8.8.8.8\"]},\"name\":\"test\",\"zones\":[\"mytest.ocp\"]}]}]"

		compat_otp.By("1. Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		compat_otp.By("2. Patch the dns.operator/default and add custom zones config, check Corefile and ensure the policy is Random")
		patchGlobalResourceAsAdmin(oc, resourceName, jsonPatch)
		policy := pollReadDnsCorefile(oc, oneDnsPod, "8.8.8.8", "-A2", "policy random")
		o.Expect(policy).To(o.ContainSubstring(`policy random`))

		compat_otp.By("3. Update the custom zones policy to RoundRobin, check Corefile and ensure it is updated ")
		patchGlobalResourceAsAdmin(oc, resourceName, "[{\"op\":\"replace\", \"path\":\"/spec/servers/0/forwardPlugin/policy\", \"value\":\"RoundRobin\"}]")
		policy = pollReadDnsCorefile(oc, oneDnsPod, "8.8.8.8", "-A2", "policy round_robin")
		o.Expect(policy).To(o.ContainSubstring(`policy round_robin`))

		compat_otp.By("4. Update the custom zones policy to Sequential, check Corefile and ensure it is updated")
		patchGlobalResourceAsAdmin(oc, resourceName, "[{\"op\":\"replace\", \"path\":\"/spec/servers/0/forwardPlugin/policy\", \"value\":\"Sequential\"}]")
		policy = pollReadDnsCorefile(oc, oneDnsPod, "8.8.8.8", "-A2", "policy sequential")
		o.Expect(policy).To(o.ContainSubstring(`policy sequential`))
	})

	g.It("Author:davidsalerno-High-122810-CoreDNS forward health check alerts fire correctly [Disruptive] [Serial] [Skipped:MicroShift]", func() {
		const (
			resourceName = "dns.operator.openshift.io/default"
			testZone     = "healthcheck-alerts.test"
		)

		compat_otp.By("1. Prepare a single DNS pod and deterministic in-cluster upstream services")
		// Register cleanup first so it runs even if the restore helper reports a
		// failure while unwinding this test. Deferred functions run in LIFO order.
		defer cleanupForwardHealthCheckAlertResources(oc)
		defer deleteDnsOperatorToRestore(oc)
		oneDNSPod := forceOnlyOneDnsPodExist(oc)
		coreDNSImage := getDNSContainerImage(oc)
		healthyUpstream, unreachableUpstreamOne, unreachableUpstreamTwo := createForwardHealthCheckAlertResources(oc, coreDNSImage)

		monitor, err := compat_otp.NewPrometheusMonitor(oc)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("2. Configure one healthy and one unreachable upstream and verify the per-upstream alert")
		patchForwardHealthCheckAlertServer(oc, resourceName, testZone, []string{healthyUpstream, unreachableUpstreamOne}, "add")
		waitForForwardHealthCheckCorefile(oc, oneDNSPod, unreachableUpstreamOne)
		ensureClusterOperatorNormal(oc, "dns", 2, 120)
		waitForFiringAlertForUpstream(monitor, oc, oneDNSPod, testZone, "CoreDNSForwardHealthCheckFailure", unreachableUpstreamOne, 8*time.Minute)
		assertNoFiringAlert(monitor, "CoreDNSForwardHealthCheckBroken")

		compat_otp.By("3. Configure two unreachable upstreams and verify the complete-outage alert suppresses per-upstream alerts")
		patchForwardHealthCheckAlertServer(oc, resourceName, testZone, []string{unreachableUpstreamOne, unreachableUpstreamTwo}, "replace")
		waitForForwardHealthCheckCorefile(oc, oneDNSPod, unreachableUpstreamTwo)
		ensureClusterOperatorNormal(oc, "dns", 2, 120)
		waitForFiringAlert(monitor, oc, oneDNSPod, testZone, "CoreDNSForwardHealthCheckBroken", 8*time.Minute)
		assertNoFiringAlertForUpstreams(monitor, "CoreDNSForwardHealthCheckFailure", unreachableUpstreamOne, unreachableUpstreamTwo)
	})

	// No dns operator namespace on HyperShift guest cluster so this case is not available
	g.It("Author:shudili-NonHyperShiftHOST-Medium-46873-Configure operatorLogLevel under the default dns operator and check the logs flag [Disruptive] [Serial]", func() {
		var (
			resourceName        = "dns.operator.openshift.io/default"
			cfgOploglevelDebug  = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"Debug\"}]"
			cfgOploglevelTrace  = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"Trace\"}]"
			cfgOploglevelNormal = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"Normal\"}]"
		)
		defer deleteDnsOperatorToRestore(oc)

		compat_otp.By("1. Check default log level of dns operator")
		outputOpcfg, errOpcfg := oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.operator", "default", "-o=jsonpath={.spec.operatorLogLevel}").Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("Normal"))

		//Remove the dns operator pod and wait for the new pod is created, which is useful to check the dns operator log
		compat_otp.By("2. Remove dns operator pod")
		dnsOperatorPodName := getPodListByLabel(oc, "openshift-dns-operator", "name=dns-operator")[0]
		_, errDelpod := oc.AsAdmin().WithoutNamespace().Run("delete").Args("pod", dnsOperatorPodName, "-n", "openshift-dns-operator").Output()
		o.Expect(errDelpod).NotTo(o.HaveOccurred())
		errPodDis := waitForResourceToDisappear(oc, "openshift-dns-operator", "pod/"+dnsOperatorPodName)
		compat_otp.AssertWaitPollNoErr(errPodDis, fmt.Sprintf("the dns-operator pod isn't terminated"))
		ensurePodWithLabelReady(oc, "openshift-dns-operator", "name=dns-operator")

		compat_otp.By("3. Patch dns operator with operator logLevel Debug")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgOploglevelDebug)
		compat_otp.By("Check logLevel debug in dns operator")
		outputOpcfg, errOpcfg = oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.operator", "default", "-o=jsonpath={.spec.operatorLogLevel}").Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("Debug"))

		compat_otp.By("4. Patch dns operator with operator logLevel trace")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgOploglevelTrace)
		compat_otp.By("Check logLevel trace in dns operator")
		outputOpcfg, errOpcfg = oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.operator", "default", "-o=jsonpath={.spec.operatorLogLevel}").Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("Trace"))

		compat_otp.By("5. Patch dns operator with operator logLevel normal")
		patchGlobalResourceAsAdmin(oc, resourceName, cfgOploglevelNormal)
		compat_otp.By("Check logLevel normal in dns operator")
		outputOpcfg, errOpcfg = oc.AsAdmin().WithoutNamespace().Run("get").Args("dns.operator", "default", "-o=jsonpath={.spec.operatorLogLevel}").Output()
		o.Expect(errOpcfg).NotTo(o.HaveOccurred())
		o.Expect(outputOpcfg).To(o.ContainSubstring("Normal"))

		compat_otp.By("6. Check logs of dns operator")
		outputLogs, errLog := oc.AsAdmin().Run("logs").Args("deployment/dns-operator", "-n", "openshift-dns-operator", "-c", "dns-operator").Output()
		o.Expect(errLog).NotTo(o.HaveOccurred())
		o.Expect(outputLogs).To(o.ContainSubstring("level=info"))
	})

	// Bug: OCPBUGS-6829
	g.It("Author:mjoseph-High-63512-Enbaling force_tcp for protocolStrategy field to allow DNS queries to send on TCP to upstream server [Disruptive] [Serial]", func() {
		var (
			resourceName                = "dns.operator.openshift.io/default"
			upstreamResolverPatch       = "[{\"op\":\"add\", \"path\":\"/spec/upstreamResolvers/protocolStrategy\", \"value\":\"TCP\"}]"
			upstreamResolverPatchRemove = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/protocolStrategy\", \"value\":\"\"}]"
			dnsForwardPluginPatch       = "[{\"op\":\"replace\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Sequential\",\"protocolStrategy\": \"TCP\",\"upstreams\":[\"8.8.8.8\"]},\"name\":\"test\",\"zones\":[\"mytest.ocp\"]}]}]"
		)

		compat_otp.By("1. Check the default dns operator config for “protocol strategy” is none")
		output, err := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(strings.Contains(output, "force_tcp")).NotTo(o.BeTrue())

		compat_otp.By("2. Prepare the dns testing node and pod")
		defer deleteDnsOperatorToRestore(oc)
		oneDnsPod := forceOnlyOneDnsPodExist(oc)

		compat_otp.By("3. Patch dns operator with 'TCP' as protocol strategy for upstreamresolver")
		patchGlobalResourceAsAdmin(oc, resourceName, upstreamResolverPatch)

		compat_otp.By("4. Check the upstreamresolver for “protocol strategy” is TCP in Corefile of coredns")
		tcp := pollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "force_tcp")
		o.Expect(tcp).To(o.ContainSubstring("force_tcp"))
		//remove the patch from upstreamresolver
		patchGlobalResourceAsAdmin(oc, resourceName, upstreamResolverPatchRemove)
		output, err = oc.AsAdmin().Run("get").Args("dns.operator.openshift.io/default", "-o=jsonpath={.spec.upstreamResolvers.protocolStrategy}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.BeEmpty())

		compat_otp.By("5. Patch dns operator with 'TCP' as protocol strategy for forwardPlugin")
		patchGlobalResourceAsAdmin(oc, resourceName, dnsForwardPluginPatch)

		compat_otp.By("6. Check the protocol strategy value as 'TCP' in Corefile of coredns under forwardPlugin")
		tcp1 := pollReadDnsCorefile(oc, oneDnsPod, "test", "-A5", "force_tcp")
		o.Expect(tcp1).To(o.ContainSubstring("force_tcp"))
	})
})
