package router

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	util "github.com/openshift/cluster-dns-operator-tests-extension/test"
	exutil "github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	netutils "k8s.io/utils/net"
)

var _ = g.Describe("[sig-network-edge][OTP][Level0] Network_Edge Component_DNS", func() {
	defer g.GinkgoRecover()

	// Registered before exutil.NewCLI on purpose. NewCLI adds its own BeforeEach that
	// creates a project, and Ginkgo runs BeforeEach blocks in declaration order; on
	// MicroShift that project creation fails before any later guard could run, which is
	// exactly the setup failure OCPBUGS-132070 describes. Keep this first.
	g.BeforeEach(func() {
		util.SkipOnMicroShift()
	})

	var oc = exutil.NewCLI("coredns")

	//Author:shudili
	g.It("CoreDNS supports dual stack ClusterIP Services for OCP4.8 or higher", g.Label("OCP-39842"), func() {
		var (
			buildPruningBaseDir = FixturePath()
			testPodSvc          = filepath.Join(buildPruningBaseDir, "web-server-v4v6rc.yaml")
			unsecsvcName        = "service-unsecurev4v6"
			secsvcName          = "service-securev4v6"
		)

		compat_otp.By("check the IP stack tpye, skip for non-dualstack platform")
		ipStackType := util.CheckIPStackType(oc)
		e2e.Logf("the cluster IP stack type is: %v", ipStackType)
		if ipStackType != "dualstack" {
			g.Skip("Skip for non-dualstack platform")
		}

		compat_otp.By("Create a backend pod and its services resources")
		ns := oc.Namespace()
		util.ReplacePayloadImage(oc, testPodSvc)
		util.CreateResourceFromFile(oc, ns, testPodSvc)
		util.EnsurePodWithLabelReady(oc, ns, "name=web-server-v4v6rc")
		srvPod := util.GetPodListByLabel(oc, ns, "name=web-server-v4v6rc")[0]

		compat_otp.By("check the services v4v6 addresses")
		IPAddresses := util.GetByJsonPath(oc, ns, "service/"+unsecsvcName, "{.spec.clusterIPs}")
		o.Expect(IPAddresses).To(o.MatchRegexp(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`))
		o.Expect(strings.Count(IPAddresses, ":") >= 2).To(o.BeTrue())

		IPAddresses = util.GetByJsonPath(oc, ns, "service/"+secsvcName, "{.spec.clusterIPs}")
		o.Expect(IPAddresses).To(o.MatchRegexp(`[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}`))
		o.Expect(strings.Count(IPAddresses, ":") >= 2).To(o.BeTrue())

		compat_otp.By("check the services names can be resolved to their v4v6 addresses")
		IPAddress1 := util.GetByJsonPath(oc, ns, "service/"+unsecsvcName, "{.spec.clusterIPs[0]}")
		IPAddress2 := util.GetByJsonPath(oc, ns, "service/"+unsecsvcName, "{.spec.clusterIPs[1]}")
		cmdOnPod := []string{"-n", ns, srvPod, "--", "getent", "ahosts", unsecsvcName}
		util.RepeatCmdOnClient(oc, cmdOnPod, IPAddress1, 30, 1)
		util.RepeatCmdOnClient(oc, cmdOnPod, IPAddress2, 30, 1)

		IPAddress1 = util.GetByJsonPath(oc, ns, "service/"+secsvcName, "{.spec.clusterIPs[0]}")
		IPAddress2 = util.GetByJsonPath(oc, ns, "service/"+secsvcName, "{.spec.clusterIPs[1]}")
		cmdOnPod = []string{"-n", ns, srvPod, "--", "getent", "ahosts", secsvcName}
		util.RepeatCmdOnClient(oc, cmdOnPod, IPAddress1, 30, 1)
		util.RepeatCmdOnClient(oc, cmdOnPod, IPAddress2, 30, 1)
	})

	// incorporate OCP-56047 and OCP-40718 into one
	//Author:shudili
	g.It("CoreDNS cache should use 900s for positive responses and 30s for negative responses [Disruptive] [Serial]", g.Label("OCP-40718"), func() {
		compat_otp.By("Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		oneDnsPod := util.ForceOnlyOneDnsPodExist(oc)

		// OCP-40718
		compat_otp.By("1. Check the cache entries of the default corefiles in CoreDNS")
		zoneInCoreFile1 := util.PollReadDnsCorefile(oc, oneDnsPod, ".:5353", "-A20", "cache 900")
		o.Expect(zoneInCoreFile1).Should(o.And(
			o.ContainSubstring("cache 900"),
			o.ContainSubstring("denial 9984 30")))

		// OCP-56047
		compat_otp.By("2. Patch the dns.operator/default and add a custom forward zone config")
		resourceName := "dns.operator.openshift.io/default"
		jsonPatch := "[{\"op\":\"add\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Random\",\"upstreams\":[\"8.8.8.8\"]},\"name\":\"test\",\"zones\":[\"mytest.ocp\"]}]}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, jsonPatch)

		compat_otp.By("3. Check the cache entries of the custom forward zone in CoreDNS")
		zoneInCoreFile := util.PollReadDnsCorefile(oc, oneDnsPod, "mytest.ocp", "-A15", "cache 900")
		o.Expect(zoneInCoreFile).Should(o.And(
			o.ContainSubstring("cache 900"),
			o.ContainSubstring("denial 9984 30")))
	})

	// Bug: 1916907
	//Author:mjoseph
	g.It("Deleting the internal registry should not corrupt /etc/hosts [Disruptive] [Serial]", g.Label("OCP-40867"), func() {
		compat_otp.By("Step1: Get the Cluster IP of image-registry")
		clusterIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
			"service", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.clusterIP}").Output()
		if err != nil || strings.Contains(clusterIP, `namespaces "openshift-image-registry" not found`) {
			g.Skip("Skip for non-supported platform")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Step2: SSH to the node and confirm the /etc/hosts have the same clusterIP")
		allNodeList, err := compat_otp.GetAllNodes(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(allNodeList).NotTo(o.BeEmpty())
		node := util.GetRandomElementFromList(allNodeList)
		hostOutput, err := compat_otp.DebugNodeWithChroot(oc, node, "cat", "/etc/hosts")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(hostOutput).To(o.And(
			o.ContainSubstring("127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4"),
			o.ContainSubstring("::1         localhost localhost.localdomain localhost6 localhost6.localdomain6"),
			o.ContainSubstring(clusterIP+" image-registry.openshift-image-registry.svc image-registry.openshift-image-registry.svc.cluster.local")))
		o.Expect(hostOutput).NotTo(o.Or(o.ContainSubstring("error"), o.ContainSubstring("failed"), o.ContainSubstring("timed out")))

		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		compat_otp.By("Step3: Delete the image-registry svc and check whether it receives a new Cluster IP")
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("svc", "image-registry", "-n", "openshift-image-registry").Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())
		err = util.WaitCoBecomes(oc, "image-registry", 240, expectedStatus)
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Step4: Get the new Cluster IP of image-registry")
		newClusterIP, err2 := oc.AsAdmin().WithoutNamespace().Run("get").Args(
			"service", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.clusterIP}").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(newClusterIP).NotTo(o.ContainSubstring(clusterIP))
		e2e.Logf("The new cluster IP is %v", newClusterIP)

		compat_otp.By("Step5: SSH to the node and confirm the /etc/hosts details, after deletion")
		cmdList := []string{"cat", "/etc/hosts"}
		expectedString := fmt.Sprintf(`%s image-registry.openshift-image-registry.svc image-registry.openshift-image-registry.svc.cluster.local # openshift-generated-node-resolver`, newClusterIP)
		util.WaitForDebugNodeOutputContains(oc, "default", node, cmdList, expectedString, 90*time.Second)
	})

	// incorporate OCP-40717 into existing OCP-46867
	//Author:shudili
	g.It("Configure upstream resolvers for CoreDNS flag [Disruptive] [Serial]", g.Label("OCP-46867"), func() {
		var (
			resourceName        = "dns.operator.openshift.io/default"
			cfgMulIPv4Upstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"10.100.1.11\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"10.100.1.12\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"10.100.1.13\",\"port\":5353,\"type\":\"Network\"}]}]"
			expMulIPv4Upstreams = "forward . 10.100.1.11:53 10.100.1.12:53 10.100.1.13:5353"
			cfgOneIPv4Upstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"20.100.1.11\",\"port\":53,\"type\":\"Network\"}]}]"
			expOneIPv4Upstreams = "forward . 20.100.1.11:53"
			cfgMax15Upstreams   = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"30.100.1.11\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.12\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.13\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.14\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.15\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.16\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.17\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.18\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.19\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.20\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.21\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.22\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.23\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.24\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.25\",\"port\":53,\"type\":\"Network\"}]}]"
			expMax15Upstreams = "forward . 30.100.1.11:53 30.100.1.12:53 30.100.1.13:53 30.100.1.14:53 30.100.1.15:53 " +
				"30.100.1.16:53 30.100.1.17:53 30.100.1.18:53 30.100.1.19:53 30.100.1.20:53 " +
				"30.100.1.21:53 30.100.1.22:53 30.100.1.23:53 30.100.1.24:53 30.100.1.25:53"
			cfgMulIPv6Upstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"1001::aaaa\",\"port\":5353,\"type\":\"Network\"}, " +
				"{\"address\":\"1001::BBBB\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"1001::cccc\",\"port\":53,\"type\":\"Network\"}]}]"
			expMulIPv6Upstreams = "forward . [1001::AAAA]:5353 [1001::BBBB]:53 [1001::CCCC]:53"
		)
		compat_otp.By("Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		oneDnsPod := util.ForceOnlyOneDnsPodExist(oc)

		// OCP-40717
		compat_otp.By("Check the readiness probe period and timeout parameters are both set to 3 seconds")
		output, err := oc.AsAdmin().Run("get").Args("pod/"+oneDnsPod, "-n", "openshift-dns", "-o=jsonpath={.spec.containers[0].readinessProbe.periodSeconds}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.Equal(`3`))
		output1, err1 := oc.AsAdmin().Run("get").Args("pod/"+oneDnsPod, "-n", "openshift-dns", "-o=jsonpath={.spec.containers[0].readinessProbe.timeoutSeconds}").Output()
		o.Expect(err1).NotTo(o.HaveOccurred())
		o.Expect(output1).To(o.Equal(`3`))

		// OCP-46867
		compat_otp.By("Check default values of forward upstream resolvers for CoreDNS")
		upstreams := util.PollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "resolv.conf")
		o.Expect(upstreams).To(o.ContainSubstring("forward . /etc/resolv.conf"))

		compat_otp.By("Patch dns operator with multiple ipv4 upstreams")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cfgMulIPv4Upstreams)

		compat_otp.By("Check multiple ipv4 forward upstream resolvers in CoreDNS")
		upstreams = util.PollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", expMulIPv4Upstreams)
		o.Expect(upstreams).To(o.ContainSubstring(expMulIPv4Upstreams))

		compat_otp.By("Patch dns operator with a single ipv4 upstream, and then check the single ipv4 forward upstream resolver for CoreDNS")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cfgOneIPv4Upstreams)
		upstreams = util.PollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", expOneIPv4Upstreams)
		o.Expect(upstreams).To(o.ContainSubstring(expOneIPv4Upstreams))

		compat_otp.By("Patch dns operator with max 15 ipv4 upstreams, and then the max 15 ipv4 forward upstream resolvers for CoreDNS")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cfgMax15Upstreams)
		upstreams = util.PollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", expMax15Upstreams)
		o.Expect(upstreams).To(o.ContainSubstring(expMax15Upstreams))

		compat_otp.By("Patch dns operator with multiple ipv6 upstreams, and then check the multiple ipv6 forward upstream resolvers for CoreDNS")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cfgMulIPv6Upstreams)
		upstreams = util.PollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", "1001")
		o.Expect(upstreams).To(o.ContainSubstring(expMulIPv6Upstreams))
	})

	//Author:shudili
	g.It("Negative test of configuring upstream resolvers and policy flag [Disruptive] [Serial]", g.Label("OCP-46869"), func() {
		var (
			resourceName       = "dns.operator.openshift.io/default"
			cfgAddOneUpstreams = "[{\"op\":\"add\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"30.100.1.11\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.12\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.13\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.14\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.15\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.16\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.17\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.18\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.19\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.20\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.21\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.22\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.23\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.24\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.25\",\"port\":53,\"type\":\"Network\"}, " +
				"{\"address\":\"30.100.1.26\",\"port\":53,\"type\":\"Network\"}]}]"
			invalidCfgStringUpstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"str_test\",\"port\":53,\"type\":\"Network\"}]}]"
			invalidCfgNumberUpstreams = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[" +
				"{\"address\":\"100\",\"port\":53,\"type\":\"Network\"}]}]"
			invalidCfgSringPolicy  = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"string_test\"}]"
			invalidCfgNumberPolicy = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"2\"}]"
			invalidCfgRandomPolicy = "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/policy\", \"value\":\"random\"}]"
		)
		compat_otp.By("Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		util.ForceOnlyOneDnsPodExist(oc)

		compat_otp.By("Try to add one more upstream resolver, totally 16 upstream resolvers by patching dns operator")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+cfgAddOneUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("have at most 15 items"))

		compat_otp.By("Try to add a upstream resolver with a string as an address")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"str_test\""))

		compat_otp.By("Try to add a upstream resolver with a number as an address")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberUpstreams, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Invalid value: \"100\""))

		compat_otp.By("Try to configure the policy with a string")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgSringPolicy, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		compat_otp.By("Try to configure the policy with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberPolicy, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		compat_otp.By("Try to configure the policy with a similar string like random")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgRandomPolicy, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"random\""))
	})

	//Author:shudili
	g.It("Configure logLevel for CoreDNS under DNS operator flag [Disruptive] [Serial]", g.Label("OCP-46872"), func() {
		var (
			resourceName     = "dns.operator.openshift.io/default"
			cfgLogLevelDebug = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Debug\"}]"
			cfgLogLevelTrace = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Trace\"}]"
		)
		compat_otp.By("Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		oneDnsPod := util.ForceOnlyOneDnsPodExist(oc)

		compat_otp.By("Check default log level of CoreDNS")
		logOutput := util.PollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class error")
		o.Expect(logOutput).To(o.ContainSubstring("class error"))

		compat_otp.By("Patch dns operator with logLevel Debug for CoreDNS, and then check log class for logLevel Debug in both CM and the Corefile of coredns")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cfgLogLevelDebug)
		logOutput = util.PollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class denial error")
		o.Expect(logOutput).To(o.ContainSubstring("class denial error"))

		compat_otp.By("Patch dns operator with logLevel Trace for CoreDNS, and then check log class for logLevel Trace in Corefile of coredns")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cfgLogLevelTrace)
		logOutput = util.PollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class all")
		o.Expect(logOutput).To(o.ContainSubstring("class all"))
	})

	//Author:shudili
	g.It("Negative test for configuring logLevel and operatorLogLevel flag [Disruptive] [Serial]", g.Label("OCP-46874"), func() {
		var (
			resourceName               = "dns.operator.openshift.io/default"
			invalidCfgStringLogLevel   = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"string_test\"}]"
			invalidCfgNumberLogLevel   = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"2\"}]"
			invalidCfgTraceLogLevel    = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"trace\"}]"
			invalidCfgStringOPLogLevel = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"string_test\"}]"
			invalidCfgNumberOPLogLevel = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"2\"}]"
			invalidCfgTraceOPLogLevel  = "[{\"op\":\"replace\", \"path\":\"/spec/operatorLogLevel\", \"value\":\"trace\"}]"
		)
		compat_otp.By("Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		util.ForceOnlyOneDnsPodExist(oc)

		compat_otp.By("Try to configure log level with a string")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		compat_otp.By("Try to configure log level with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		compat_otp.By("Try to configure log level with a similar string like trace")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgTraceLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"trace\""))

		compat_otp.By("Try to configure dns operator log level with a string")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgStringOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"string_test\""))

		compat_otp.By("Try to configure dns operator log level with a number")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgNumberOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"2\""))

		compat_otp.By("Try to configure dns operator log level with a similar string like trace")
		output, _ = oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+invalidCfgTraceOPLogLevel, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("Unsupported value: \"trace\""))
	})

	//Author:shudili
	g.It("Different LogLevel logging function of CoreDNS flag [Disruptive] [Serial]", g.Label("OCP-46875"), func() {
		var (
			buildPruningBaseDir = FixturePath()
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			clientPodName       = "hello-pod"
			clientPodLabel      = "app=hello-pod"
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			failedDNSReq        = "failed.not-myocp-test.com"
			nxDNSReq            = "notexist.myocp-test.com"
			normalDNSReq        = "www.myocp-test.com"
			resourceName        = "dns.operator.openshift.io/default"
			cfgDebug            = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Debug\"}]"
			cfgTrace            = "[{\"op\":\"replace\", \"path\":\"/spec/logLevel\", \"value\":\"Trace\"}]"
		)
		compat_otp.By("Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		oneDnsPod := util.ForceOnlyOneDnsPodExist(oc)
		podList := []string{oneDnsPod}

		compat_otp.By("Create a dns server pod")
		ns := oc.Namespace()
		defer func() {
			err := compat_otp.RecoverNamespaceRestricted(oc, ns)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := compat_otp.SetNamespacePrivileged(oc, ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		util.ReplaceCoreDnsImage(oc, coreDNSSrvPod)
		err = oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		util.EnsurePodWithLabelReady(oc, ns, srvPodLabel)

		compat_otp.By("get the user's dns server pod's IP")
		srvPodIP := util.GetPodv4Address(oc, srvPodName, ns)

		compat_otp.By("patch upstream dns resolver with the user's dns server, and then wait the corefile is updated")
		// Port 5353, not 53: the test CoreDNS server cannot bind a privileged port under the
		// restricted-v2 SCC. See the comment in testdata/coreDNS-pod.yaml.
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers/upstreams\", \"value\":[{\"address\":\"" + srvPodIP + "\",\"port\":5353,\"type\":\"Network\"}]}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)
		if strings.Count(srvPodIP, ":") >= 2 {
			srvPodIP = strings.ToUpper(srvPodIP)
			srvPodIP = "[" + srvPodIP + "]"
		}
		util.PollReadDnsCorefile(oc, oneDnsPod, "forward", "-A2", srvPodIP)

		compat_otp.By("create a client pod")
		util.ReplacePayloadImage(oc, clientPod)
		util.CreateResourceFromFile(oc, ns, clientPod)
		util.EnsurePodWithLabelReady(oc, ns, clientPodLabel)

		compat_otp.By("Let client send out SERVFAIL nslookup to the dns server, and check the desired SERVFAIL logs from a coredns pod")
		output := util.NslookupsAndWaitForDNSlog(oc, clientPodName, failedDNSReq, podList, failedDNSReq+".")
		o.Expect(output).To(o.ContainSubstring(failedDNSReq))

		compat_otp.By("Patch dns operator with logLevel Debug for CoreDNS, and wait the Corefile is updated")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cfgDebug)
		util.PollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class denial error")

		compat_otp.By("Let client send out NXDOMAIN nslookup to the dns server, and check the desired NXDOMAIN logs from a coredns pod")
		output = util.NslookupsAndWaitForDNSlog(oc, clientPodName, nxDNSReq, podList, "-type=mx", nxDNSReq+".")
		o.Expect(output).To(o.ContainSubstring(nxDNSReq))

		compat_otp.By("Patch dns operator with logLevel Trace for CoreDNS, and wait the Corefile is updated")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cfgTrace)
		util.PollReadDnsCorefile(oc, oneDnsPod, "log", "-A2", "class all")

		compat_otp.By("Let client send out normal nslookup which will get correct response, and check the desired TRACE logs from a coredns pod")
		output = util.NslookupsAndWaitForDNSlog(oc, clientPodName, normalDNSReq, podList, normalDNSReq+".")
		o.Expect(output).To(o.ContainSubstring(normalDNSReq))
	})

	// Needs the dns-operator namespace, absent on a HyperShift guest cluster, so cmd/main.go
	// excludes this case on the External topology.
	//Author:mjoseph
	g.It("Support CoreDNS forwarding DNS requests over TLS using UpstreamResolvers [Disruptive] [Serial]", g.Label("OCP-51946"), func() {
		var (
			buildPruningBaseDir = FixturePath()
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			resourceName        = "dns.operator.openshift.io/default"
			dirname             = "/tmp/OCP-51946-ca/"
			caKey               = dirname + "ca.key"
			caCert              = dirname + "ca-bundle.crt"
			caSubj              = "/CN=NE-Test-Root-CA"
			dnsPodLabel         = "dns.operator.openshift.io/daemonset-dns=default"
		)

		compat_otp.By("1.Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		oneDnsPod := util.ForceOnlyOneDnsPodExist(oc)

		compat_otp.By("2.Create a dns server pod")
		ns := oc.Namespace()
		defer func() {
			err := compat_otp.RecoverNamespaceRestricted(oc, ns)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := compat_otp.SetNamespacePrivileged(oc, ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		util.ReplaceCoreDnsImage(oc, coreDNSSrvPod)
		err = oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		util.EnsurePodWithLabelReady(oc, ns, srvPodLabel)
		srvPodIP := util.GetPodv4Address(oc, srvPodName, ns)

		compat_otp.By("3.Generate a new self-signed CA")
		defer os.RemoveAll(dirname)
		err = os.MkdirAll(dirname, 0755)
		o.Expect(err).NotTo(o.HaveOccurred())
		e2e.Logf("Generate the CA private key")
		opensslCmd := fmt.Sprintf(`openssl genrsa -out %s 4096`, caKey)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		e2e.Logf("Create the CA certificate")
		opensslCmd = fmt.Sprintf(`openssl req -x509 -new -nodes -key %s -sha256 -days 1 -out %s -subj %s`, caKey, caCert, caSubj)
		_, err = exec.Command("bash", "-c", opensslCmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("4.Create configmap ca-xxxxx-bundle in namespace openshift-config")
		defer util.DeleteConfigMap(oc, "openshift-config", "ca-51946-bundle")
		util.CreateConfigMapFromFile(oc, "openshift-config", "ca-51946-bundle", caCert)

		compat_otp.By("5.Patch the dns.operator/default with transport option as TLS for upstreamresolver")
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers\", \"value\":{\"transportConfig\": {\"tls\":{\"caBundle\": {\"name\": \"ca-51946-bundle\"}, \"serverName\": \"dns.ocp51946.ocp\"}, \"transport\": \"TLS\"}, \"upstreams\":[{\"address\":\"" + srvPodIP + "\",  \"port\": 853, \"type\":\"Network\"}]}}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)

		compat_otp.By("6.Check and confirm the upstream resolver's IP(srvPodIP) and custom CAbundle name appearing in the dns pod")
		if strings.Count(srvPodIP, ":") >= 2 {
			srvPodIP = strings.ToUpper(srvPodIP)
			srvPodIP = "[" + srvPodIP + "]"
		}
		waitErr := util.WaitForResourceToDisappear(oc, "openshift-dns", "pod/"+oneDnsPod)
		compat_otp.AssertWaitPollNoErr(waitErr, fmt.Sprintf("max time reached but pod %s is not terminated", oneDnsPod))
		util.EnsurePodWithLabelReady(oc, "openshift-dns", dnsPodLabel)
		newDnsPod := util.GetDNSPodName(oc)
		upstreams := util.ReadDNSCorefile(oc, newDnsPod, srvPodIP, "-A4")
		o.Expect(upstreams).To(o.And(
			o.ContainSubstring("forward . tls://"+srvPodIP+":853"),
			o.ContainSubstring("tls_servername dns.ocp51946.ocp"),
			o.ContainSubstring("tls /etc/pki/dns.ocp51946.ocp-ca-ca-51946-bundle")))

		compat_otp.By("7.Check no error logs from dns operator pod")
		dnsOperatorPodName := util.GetPodListByLabel(oc, "openshift-dns-operator", "name=dns-operator")
		podLogs, errLogs := compat_otp.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], srvPodIP+` -A3`)
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs).To(o.ContainSubstring(`msg="reconciling request: /default"`))
	})

	// Needs the dns-operator namespace, absent on a HyperShift guest cluster, so cmd/main.go
	// excludes this case on the External topology.
	//Author:mjoseph
	g.It("CoreDNS forwarding DNS requests over TLS with CLEAR TEXT [Disruptive] [Serial]", g.Label("OCP-52077"), func() {
		var (
			buildPruningBaseDir = FixturePath()
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			resourceName        = "dns.operator.openshift.io/default"
		)

		compat_otp.By("1.Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		oneDnsPod := util.ForceOnlyOneDnsPodExist(oc)

		compat_otp.By("2.Create a dns server pod")
		ns := oc.Namespace()
		defer func() {
			err := compat_otp.RecoverNamespaceRestricted(oc, ns)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := compat_otp.SetNamespacePrivileged(oc, ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		util.ReplaceCoreDnsImage(oc, coreDNSSrvPod)
		err = oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		util.EnsurePodWithLabelReady(oc, ns, srvPodLabel)

		compat_otp.By("3.Get the user's dns server pod's IP")
		srvPodIP := util.GetPodv4Address(oc, srvPodName, ns)
		upstreamAddress := net.JoinHostPort(strings.ToUpper(srvPodIP), "5353")

		compat_otp.By("4.Patch the dns.operator/default with transport option as Cleartext for forwardplugin")
		dnsForwardPlugin := "[{\"op\":\"add\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Sequential\",\"transportConfig\": {\"transport\": \"Cleartext\"}, \"upstreams\":[\"" + upstreamAddress + "\"]}, \"name\": \"test\", \"zones\":[\"ocp52077.ocp\"]}]}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, dnsForwardPlugin)

		compat_otp.By("5.Check and confirm the upstream resolver's IP(srvPodIP) appearing in the dns pod")
		forward := util.PollReadDnsCorefile(oc, oneDnsPod, "ocp52077.ocp", "-A6", "forward . "+upstreamAddress)
		o.Expect(forward).To(o.And(
			o.ContainSubstring("ocp52077.ocp:5353"),
			o.ContainSubstring("forward . "+upstreamAddress)), "Cleartext forwarding must use the fixture's port 5353")

		compat_otp.By("6.Check no error logs from dns operator pod")
		dnsOperatorPodName := util.GetPodListByLabel(oc, "openshift-dns-operator", "name=dns-operator")
		podLogs1, errLogs1 := compat_otp.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], `-F -A3 -- "ocp52077.ocp:5353"`)
		o.Expect(errLogs1).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs1).To(o.ContainSubstring(`msg="reconciling request: /default"`))
		dnsDefault := "[{\"op\":\"remove\", \"path\":\"/spec/servers\"}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, dnsDefault)

		compat_otp.By("7.Patch dns.operator/default with transport option as Cleartext for upstreamresolver")
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers\", \"value\":{\"transportConfig\":{\"transport\":\"Cleartext\"}, \"upstreams\":[{\"address\":\"" + srvPodIP + "\", \"port\":5353, \"type\":\"Network\"}]}}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)

		compat_otp.By("8.Check and confirm the upstream resolver's IP(srvPodIP) appearing in the dns pod")
		upstreams := util.PollReadDnsCorefile(oc, oneDnsPod, "forward .", "-A2", "forward . "+upstreamAddress)
		o.Expect(upstreams).To(o.ContainSubstring("forward . "+upstreamAddress), "Cleartext upstream resolvers must use the fixture's port 5353")

		compat_otp.By("9.Check no error logs from dns operator pod")
		podLogs, errLogs := compat_otp.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], fmt.Sprintf("-F -A3 -- %q", upstreamAddress))
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs).To(o.ContainSubstring(`msg="reconciling request: /default"`))
	})

	// Needs the dns-operator namespace, absent on a HyperShift guest cluster, so cmd/main.go
	// excludes this case on the External topology.
	//Author:mjoseph
	g.It("Support CoreDNS forwarding DNS requests over TLS - using system CA [Disruptive] [Serial]", g.Label("OCP-52497"), func() {
		var (
			buildPruningBaseDir = FixturePath()
			coreDNSSrvPod       = filepath.Join(buildPruningBaseDir, "coreDNS-pod.yaml")
			srvPodName          = "test-coredns"
			srvPodLabel         = "name=test-coredns"
			resourceName        = "dns.operator.openshift.io/default"
		)

		compat_otp.By("1.Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		oneDnsPod := util.ForceOnlyOneDnsPodExist(oc)

		compat_otp.By("2.Create a dns server pod")
		ns := oc.Namespace()
		defer func() {
			err := compat_otp.RecoverNamespaceRestricted(oc, ns)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		err := compat_otp.SetNamespacePrivileged(oc, ns)
		o.Expect(err).NotTo(o.HaveOccurred())
		util.ReplaceCoreDnsImage(oc, coreDNSSrvPod)
		err = oc.AsAdmin().Run("create").Args("-f", coreDNSSrvPod, "-n", ns).Execute()
		o.Expect(err).NotTo(o.HaveOccurred())
		util.EnsurePodWithLabelReady(oc, ns, srvPodLabel)

		compat_otp.By("3.Get the user's dns server pod's IP")
		srvPodIP := util.GetPodv4Address(oc, srvPodName, ns)

		compat_otp.By("4.Patch the dns.operator/default with transport option as tls for forwardplugin")
		dnsForwardPlugin := "[{\"op\":\"add\", \"path\":\"/spec/servers\", \"value\":[{\"forwardPlugin\":{\"policy\":\"Sequential\",\"transportConfig\": {\"tls\":{\"serverName\": \"dns.ocp52497.ocp\"}, \"transport\": \"TLS\"}, \"upstreams\":[\"" + srvPodIP + "\"]}, \"name\": \"test\", \"zones\":[\"ocp52497.ocp\"]}]}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, dnsForwardPlugin)

		compat_otp.By("5.Check and confirm the upstream resolver's IP(srvPodIP) appearing in the dns pod")
		forward := util.PollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-b6", "ocp52497")
		o.Expect(forward).To(o.And(
			o.ContainSubstring("ocp52497.ocp:5353"),
			o.ContainSubstring("forward . tls://"+srvPodIP),
			o.ContainSubstring("tls_servername dns.ocp52497.ocp"),
			o.ContainSubstring("tls")))

		compat_otp.By("6.Check no error logs from dns operator pod")
		dnsOperatorPodName := util.GetPodListByLabel(oc, "openshift-dns-operator", "name=dns-operator")
		podLogs1, errLogs1 := compat_otp.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], `ocp52497.ocp:5353 -A3`)
		o.Expect(errLogs1).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs1).To(o.ContainSubstring(`msg="reconciling request: /default"`))
		dnsDefault := "[{\"op\":\"remove\", \"path\":\"/spec/servers\"}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, dnsDefault)

		compat_otp.By("7.Patch dns.operator/default with transport option as tls for upstreamresolver")
		dnsUpstreamResolver := "[{\"op\":\"replace\", \"path\":\"/spec/upstreamResolvers\", \"value\":{\"transportConfig\": {\"tls\":{\"serverName\": \"dns.ocp52497.ocp\"}, \"transport\": \"TLS\"}, \"upstreams\":[{\"address\":\"" + srvPodIP + "\",  \"port\": 853, \"type\":\"Network\"}]}}]"
		util.PatchGlobalResourceAsAdmin(oc, resourceName, dnsUpstreamResolver)

		compat_otp.By("8.Check and confirm the upstream resolver's IP(srvPodIP) appearing in the dns pod")
		if strings.Count(srvPodIP, ":") >= 2 {
			srvPodIP = strings.ToUpper(srvPodIP)
			srvPodIP = "[" + srvPodIP + "]"
		}
		upstreams := util.PollReadDnsCorefile(oc, oneDnsPod, srvPodIP, "-A3", "forward . tls://"+srvPodIP+":853")
		o.Expect(upstreams).To(o.And(
			o.ContainSubstring("forward . tls://"+srvPodIP+":853"),
			o.ContainSubstring("tls_servername dns.ocp52497.ocp"),
			o.ContainSubstring("tls")))

		compat_otp.By("9.Check no error logs from dns operator pod")
		podLogs, errLogs := compat_otp.GetSpecificPodLogs(oc, "openshift-dns-operator", "dns-operator", dnsOperatorPodName[0], srvPodIP+` -A3`)
		o.Expect(errLogs).NotTo(o.HaveOccurred(), "Error in getting logs from the pod")
		o.Expect(podLogs).To(o.ContainSubstring(`msg="reconciling request: /default"`))
	})

	//Author:mjoseph
	g.It("Configuring CoreDNS caching and TTL parameters [Disruptive] [Serial]", g.Label("OCP-54042"), func() {
		var (
			resourceName      = "dns.operator.openshift.io/default"
			cacheValue        = "[{\"op\":\"replace\", \"path\":\"/spec/cache\", \"value\":{\"negativeTTL\":\"1800s\", \"positiveTTL\":\"604801s\"}}]"
			cacheSmallValue   = "[{\"op\":\"replace\", \"path\":\"/spec/cache\", \"value\":{\"negativeTTL\":\"1s\", \"positiveTTL\":\"1s\"}}]"
			cacheDecimalValue = "[{\"op\":\"replace\", \"path\":\"/spec/cache\", \"value\":{\"negativeTTL\":\"1.9s\", \"positiveTTL\":\"1.6m\"}}]"
			cacheWrongValue   = "[{\"op\":\"replace\", \"path\":\"/spec/cache\", \"value\":{\"negativeTTL\":\"-9s\", \"positiveTTL\":\"1.6\"}}]"
		)

		compat_otp.By("1. Prepare the dns testing node and pod")
		restoreDNS := util.PreserveDNSConfig(oc)
		defer restoreDNS()
		oneDnsPod := util.ForceOnlyOneDnsPodExist(oc)

		compat_otp.By("2. Patch the dns.operator/default with postive and negative cache values")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cacheValue)

		compat_otp.By("3. Check the cache value in Corefile of coredns")
		cache := util.PollReadDnsCorefile(oc, oneDnsPod, "cache 604801", "-A2", "denial")
		o.Expect(cache).To(o.ContainSubstring("denial 9984 1800"))

		compat_otp.By("4. Patch the dns.operator/default with smallest cache values and verify the same")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cacheSmallValue)
		cache1 := util.PollReadDnsCorefile(oc, oneDnsPod, "cache 1", "-A2", "denial")
		o.Expect(cache1).To(o.ContainSubstring("denial 9984 1"))

		compat_otp.By("5. Patch the dns.operator/default with decimal cache values and verify the same")
		util.PatchGlobalResourceAsAdmin(oc, resourceName, cacheDecimalValue)
		cache2 := util.PollReadDnsCorefile(oc, oneDnsPod, "cache 96", "-A2", "denial")
		o.Expect(cache2).To(o.ContainSubstring("denial 9984 2"))

		compat_otp.By("6. Patch the dns.operator/default with unrelasitc cache values and check the error messages")
		output, _ := oc.AsAdmin().WithoutNamespace().Run("patch").Args(resourceName, "--patch="+cacheWrongValue, "--type=json").Output()
		o.Expect(output).To(o.ContainSubstring("spec.cache.positiveTTL: Invalid value: \"1.6\""))
		o.Expect(output).To(o.ContainSubstring("spec.cache.negativeTTL: Invalid value: \"-9s\""))
	})

	// Bug: 1949361, 1884053, 1756344
	// Reads the ingress-operator pod, absent on a HyperShift guest cluster, so cmd/main.go
	// excludes this case on the External topology.
	//Author:mjoseph
	g.It("Check CoreDNS default bufsize, readinessProbe path and policy", g.Label("OCP-55821"), func() {
		var (
			buildPruningBaseDir = FixturePath()
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			clientPodLabel      = "app=hello-pod"
			clientPodName       = "hello-pod"
			ptrValue            = "10.0.30.172.in-addr.arpa"
		)
		ns := oc.Namespace()

		compat_otp.By("Check updated value in dns operator file")
		output, err := oc.AsAdmin().Run("get").Args("cm/dns-default", "-n", "openshift-dns", "-o=jsonpath={.data.Corefile}").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring("bufsize 1232"))

		compat_otp.By("Check the cache value in Corefile of coredns under all dns-default-xxx pods")
		podList := util.GetAllDNSPodsNames(oc)
		util.KeepSearchInAllDNSPods(oc, podList, "bufsize 1232")

		compat_otp.By("Create a client pod")
		util.ReplacePayloadImage(oc, clientPod)
		util.CreateResourceFromFile(oc, ns, clientPod)
		util.EnsurePodWithLabelReady(oc, ns, clientPodLabel)

		compat_otp.By("Query a cluster-local name to check the EDNS buffer size without external DNS")
		digOutput, err2 := oc.Run("exec").Args(clientPodName, "--", "dig", "kubernetes.default.svc.cluster.local").Output()
		o.Expect(err2).NotTo(o.HaveOccurred())
		o.Expect(digOutput).To(o.ContainSubstring("status: NOERROR"), "the Kubernetes service must resolve through cluster DNS")
		o.Expect(digOutput).To(o.ContainSubstring("udp: 1232"))

		compat_otp.By("Client send out a dig for NXDOMAIN to check response")
		digOutput1, err3 := oc.Run("exec").Args(clientPodName, "--", "dig", "nonexistent.invalid-namespace.svc.cluster.local").Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		o.Expect(digOutput1).To(o.ContainSubstring("status: NXDOMAIN"), "a nonexistent cluster service must return NXDOMAIN")
		o.Expect(digOutput1).To(o.ContainSubstring("udp: 1232"))

		compat_otp.By("Check the different DNS records")
		ingressContPod := util.GetPodListByLabel(oc, "openshift-ingress-operator", "name=ingress-operator")
		clusterIP := util.GetSvcClusterIPByName(oc, "openshift-dns", "dns-default")
		if netutils.IsIPv6String(clusterIP) {
			ptrValue = util.ConvertV6AddressToPTR(clusterIP)
		} else {
			octets := strings.Split(clusterIP, ".")
			slices.Reverse(octets)
			ptrValue = strings.Join(octets, ".") + ".in-addr.arpa"
		}

		digOutput3, err3 := oc.AsAdmin().Run("exec").Args("-n", "openshift-ingress-operator", ingressContPod[0],
			"--", "dig", "+short", ptrValue, "PTR").Output()
		o.Expect(err3).NotTo(o.HaveOccurred())
		o.Expect(digOutput3).To(o.ContainSubstring("dns-default.openshift-dns.svc.cluster.local."))

		digOutput4, err4 := oc.AsAdmin().Run("exec").Args("-n", "openshift-ingress-operator", ingressContPod[0], "--", "dig",
			"+short", "_8443-tcp._tcp.ingress-canary.openshift-ingress-canary.svc.cluster.local", "SRV").Output()
		o.Expect(err4).NotTo(o.HaveOccurred())
		o.Expect(digOutput4).To(o.ContainSubstring("ingress-canary.openshift-ingress-canary.svc.cluster.local."))

		// bug:- 1884053
		compat_otp.By("Check Readiness probe configured to use the '/ready' path")
		dnsPodName2 := util.GetRandomElementFromList(podList)
		output2, err4 := oc.AsAdmin().Run("get").Args("pod/"+dnsPodName2, "-n", "openshift-dns", "-o=jsonpath={.spec.containers[0].readinessProbe.httpGet}").Output()
		o.Expect(err4).NotTo(o.HaveOccurred())
		o.Expect(output2).To(o.ContainSubstring(`"path":"/ready"`))

		// bug:- 1756344
		compat_otp.By("Check the policy is sequential in Corefile of coredns under all dns-default-xxx pods")
		util.KeepSearchInAllDNSPods(oc, podList, "policy sequential")
	})

	// Bug: 2061244
	// Taints master nodes, which a HyperShift guest cluster does not have, so cmd/main.go
	// excludes this case on the External topology.
	//Author:mjoseph
	g.It("DNS pod should not work on nodes with taint configured [Disruptive] [Serial]", g.Label("OCP-56325"), func() {
		nodes, err := compat_otp.GetAllNodes(oc)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to list nodes before the taint test")
		if len(nodes) == 1 {
			g.Skip("Tainting separate control-plane and worker nodes requires a multi-node cluster")
		}

		masterNodes := strings.Fields(util.GetByLabelAndJsonPath(oc, "default", "node", "node-role.kubernetes.io/master", "{.items[*].metadata.name}"))
		workerNodes := strings.Fields(util.GetByLabelAndJsonPath(oc, "default", "node", "node-role.kubernetes.io/worker,!node-role.kubernetes.io/master", "{.items[*].metadata.name}"))
		if len(workerNodes) == 0 {
			g.Skip("Tainting separate control-plane and worker nodes requires a dedicated worker")
		}
		o.Expect(masterNodes).NotTo(o.BeEmpty(), "the taint test requires a control-plane node")
		masterNodeName := util.GetRandomElementFromList(masterNodes)
		workerNodeName := util.GetRandomElementFromList(workerNodes)

		compat_otp.By("Check whether the dns pods eviction annotation is set or not")
		podList := util.GetAllDNSPodsNames(oc)
		dnsPodName := util.GetRandomElementFromList(podList)
		findAnnotation := util.GetAnnotation(oc, "openshift-dns", "po", dnsPodName)
		o.Expect(findAnnotation).To(o.ContainSubstring(`cluster-autoscaler.kubernetes.io/enable-ds-eviction":"true`))

		compat_otp.By("Apply NoSchedule taint to worker node and confirm the dns pod is not scheduled")
		defer util.DeleteTaint(oc, "node", workerNodeName, "dedicated-")
		util.AddTaint(oc, "node", workerNodeName, "dedicated=Kafka:NoSchedule")
		podOut, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", "openshift-dns", "ds", "dns-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podOut).NotTo(o.ContainSubstring("Number of Nodes Misscheduled: 1"),
			"DNS pod should not be misscheduled after tainting worker node")

		compat_otp.By("Apply NoSchedule taint to master node and confirm the dns pod is not scheduled on it")
		defer util.DeleteTaint(oc, "node", masterNodeName, "dns-taint-")
		util.AddTaint(oc, "node", masterNodeName, "dns-taint=test:NoSchedule")
		podOut2, err := oc.AsAdmin().WithoutNamespace().Run("describe").Args("-n", "openshift-dns", "ds", "dns-default").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(podOut2).NotTo(o.ContainSubstring("Number of Nodes Misscheduled: 2"),
			"DNS pod should not be misscheduled after tainting master node")
	})

	// Bug: 1916907
	// Bug: OCPBUGS-35063
	// Patches configs.imageregistry/cluster, absent on a HyperShift guest cluster, so
	// cmd/main.go excludes this case on the External topology.
	//Author:mjoseph
	g.It("Disabling the internal registry should not corrupt /etc/hosts [Disruptive] [Serial]", g.Label("OCP-56539"), func() {
		compat_otp.By("Step1: Get the Cluster IP of image-registry")
		clusterIP, err := oc.AsAdmin().WithoutNamespace().Run("get").Args(
			"service", "image-registry", "-n", "openshift-image-registry", "-o=jsonpath={.spec.clusterIP}").Output()
		if err != nil || strings.Contains(clusterIP, `namespaces "openshift-image-registry" not found`) {
			g.Skip("Skip for non-supported platform")
		}
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Step2: SSH to the node and confirm the /etc/hosts have the same clusterIP")
		allNodeList, err := compat_otp.GetAllNodes(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(allNodeList).NotTo(o.BeEmpty())
		node := util.GetRandomElementFromList(allNodeList)
		hostOutput, err := compat_otp.DebugNodeWithChroot(oc, node, "cat", "/etc/hosts")
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(hostOutput).To(o.And(
			o.ContainSubstring("127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4"),
			o.ContainSubstring("::1         localhost localhost.localdomain localhost6 localhost6.localdomain6"),
			o.ContainSubstring(clusterIP)))
		o.Expect(hostOutput).NotTo(o.Or(o.ContainSubstring("error"), o.ContainSubstring("failed"), o.ContainSubstring("timed out")))

		expectedStatus := map[string]string{"Available": "True", "Progressing": "False", "Degraded": "False"}

		compat_otp.By("Step3: Disable the internal registry and check /host details")
		defer func() {
			compat_otp.By("Recover image registry change")
			err4 := oc.AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", "{\"spec\":{\"managementState\":\"Managed\"}}", "--type=merge").Execute()
			o.Expect(err4).NotTo(o.HaveOccurred())
			err = util.WaitCoBecomes(oc, "image-registry", 240, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = util.WaitCoBecomes(oc, "openshift-apiserver", 480, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
			err = util.WaitCoBecomes(oc, "kube-apiserver", 800, expectedStatus)
			o.Expect(err).NotTo(o.HaveOccurred())
		}()
		_, err = oc.WithoutNamespace().AsAdmin().Run("patch").Args("configs.imageregistry/cluster", "-p", `{"spec":{"managementState":"Removed"}}`, "--type=merge").Output()
		o.Expect(err).NotTo(o.HaveOccurred())

		compat_otp.By("Step4: SSH to the node and confirm the /etc/hosts details, after disabling")
		hostOutput2, err5 := compat_otp.DebugNodeWithChroot(oc, node, "cat", "/etc/hosts")
		o.Expect(err5).NotTo(o.HaveOccurred())
		o.Expect(hostOutput2).To(o.And(
			o.ContainSubstring("127.0.0.1   localhost localhost.localdomain localhost4 localhost4.localdomain4"),
			o.ContainSubstring("::1         localhost localhost.localdomain localhost6 localhost6.localdomain6")))
		o.Expect(hostOutput2).NotTo(o.Or(o.ContainSubstring("error"), o.ContainSubstring("failed"), o.ContainSubstring("timed out")))
	})

	//Author:mjoseph
	g.It("Confirm the coreDNS version and Kubernetes version of the oc client", g.Label("OCP-56884"), func() {
		// Bump these two constants whenever the payload rebases; a stale value fails the
		// test on every run, which is what it was doing against k8s v1.37.
		var kubernetesVersion = "v1.37"
		var coreDNS = "CoreDNS-1.14.7"

		compat_otp.By("1.Check the Kubernetes version")
		ocClientOutput, err := oc.AsAdmin().WithoutNamespace().Run("version").Args("--client=false").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(ocClientOutput).To(o.ContainSubstring(kubernetesVersion))

		compat_otp.By("2.Check all default dns pods for coredns version")
		cmd := fmt.Sprintf("coredns --version")
		podList := util.GetAllDNSPodsNames(oc)
		dnsPod := util.GetRandomElementFromList(podList)
		output, err := oc.AsAdmin().Run("exec").Args("-n", "openshift-dns", dnsPod, "-c", "dns", "--", "bash", "-c", cmd).Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(output).To(o.ContainSubstring(coreDNS))
	})

	//Author:mjoseph
	g.It("Check the max number of domains in the search path list of any pod", g.Label("OCP-60350"), func() {
		var (
			buildPruningBaseDir = FixturePath()
			clientPod           = filepath.Join(buildPruningBaseDir, "testpod-60350.yaml")
			clientPodLabel      = "app=testpod-60350"
			clientPodName       = "testpod-60350"
		)
		ns := oc.Namespace()

		compat_otp.By("Create a pod with 32 DNS search list")
		util.ReplacePayloadImage(oc, clientPod)
		util.CreateResourceFromFile(oc, ns, clientPod)
		util.EnsurePodWithLabelReady(oc, ns, clientPodLabel)

		compat_otp.By("Check the pod event logs and confirm there is no Search Line limits")
		checkPodEvent := util.DescribePodResource(oc, clientPodName, ns)
		o.Expect(checkPodEvent).NotTo(o.ContainSubstring("Warning  DNSConfigForming"))

		compat_otp.By("Check the resulting pod have all those search entries in its /etc/resolf.conf")
		execOutput, err := oc.Run("exec").Args(clientPodName, "--", "sh", "-c", "cat /etc/resolv.conf").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execOutput).To(o.ContainSubstring("8th.com 9th.com 10th.com 11th.com 12th.com 13th.com 14th.com 15th.com 16th.com 17th.com 18th.com 19th.com 20th.com 21th.com 22th.com 23th.com 24th.com 25th.com 26th.com 27th.com 28th.com 29th.com 30th.com 31th.com 32th.com"))
	})

	//Author:mjoseph
	g.It("Check the max number of characters in the search path of any pod", g.Label("OCP-60492"), func() {
		var (
			buildPruningBaseDir = FixturePath()
			clientPod           = filepath.Join(buildPruningBaseDir, "testpod-60492.yaml")
			clientPodLabel      = "app=testpod-60492"
			clientPodName       = "testpod-60492"
		)
		ns := oc.Namespace()

		compat_otp.By("Create a pod with a single search path with 253 characters")
		util.ReplacePayloadImage(oc, clientPod)
		util.CreateResourceFromFile(oc, ns, clientPod)
		util.EnsurePodWithLabelReady(oc, ns, clientPodLabel)

		compat_otp.By("Check the pod event logs and confirm there is no Search Line limits")
		checkPodEvent := util.DescribePodResource(oc, clientPodName, ns)
		o.Expect(checkPodEvent).NotTo(o.ContainSubstring("Warning  DNSConfigForming"))

		compat_otp.By("Check the resulting pod have all those search entries in its /etc/resolf.conf")
		execOutput, err := oc.Run("exec").Args(clientPodName, "--", "sh", "-c", "cat /etc/resolv.conf").Output()
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(execOutput).To(o.ContainSubstring("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.ccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc.ddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"))
	})

	// Bug: 2095941, OCPBUGS-5943
	//Author:mjoseph
	g.It("Annotation 'TopologyAwareHints' presents should not cause any pathological events", g.Label("OCP-63553"), func() {
		// OCPBUGS-5943
		compat_otp.By("Check dns daemon set for minReadySeconds to 9, maxSurge to 10% and maxUnavailable to 0")
		jsonPath := `{.spec.minReadySeconds}-{.spec.updateStrategy.rollingUpdate.maxSurge}-{.spec.updateStrategy.rollingUpdate.maxUnavailable}`
		spec := util.GetByJsonPath(oc, "openshift-dns", "daemonset/dns-default", jsonPath)
		o.Expect(spec).To(o.ContainSubstring("9-10%-0"))

		// OCPBUGS-87970
		// The operator no longer sets the legacy service.kubernetes.io/topology-aware-hints
		// annotation; it relies on the Service's trafficDistribution field instead. The
		// legacy annotation took precedence over trafficDistribution and made the
		// EndpointSlice controller report TopologyAwareHintsDisabled on clusters where the
		// DNS endpoints could not be spread across the eligible zones. This expectation is
		// unconditional - it no longer depends on how many zones the cluster has - and it
		// mirrors TestDesiredDNSServiceUsesTrafficDistribution in the operator's unit tests.
		compat_otp.By("Check the legacy topology-aware-hints annotation is not set on the dns service")
		findAnnotation := util.GetAnnotation(oc, "openshift-dns", "svc", "dns-default")
		o.Expect(findAnnotation).NotTo(o.ContainSubstring("service.kubernetes.io/topology-aware-hints"),
			"the operator must not set the legacy topology-aware-hints annotation (OCPBUGS-87970)")

		compat_otp.By("Check the dns service uses trafficDistribution PreferSameNode instead")
		trafficDistribution := util.GetByJsonPath(oc, "openshift-dns", "svc/dns-default", "{.spec.trafficDistribution}")
		o.Expect(trafficDistribution).To(o.Equal("PreferSameNode"), "dns-default service must use trafficDistribution PreferSameNode")
	})

	// Resolves public DNS names, so cmd/main.go excludes this case on Disconnected clusters
	// and on the External topology.
	//Author:mjoseph
	g.It("DNSNameResolver CR get updated with IP addresses and TTL of the DNS name [Serial]", g.Label("OCP-73379"), func() {
		if !compat_otp.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test, skipping")
		}
		var (
			buildPruningBaseDir = FixturePath()
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			clientPodLabel      = "app=hello-pod"
			clientPodName       = "hello-pod"
			egressFirewall      = filepath.Join(buildPruningBaseDir, "egressfirewall-wildcard.yaml")
		)

		compat_otp.By("1. Create egressfirewall file")
		ns := oc.Namespace()
		util.OperateResourceFromFile(oc, "create", ns, egressFirewall)
		util.WaitEgressFirewallApplied(oc, "default", ns)

		compat_otp.By("2. Create a client pod")
		util.ReplacePayloadImage(oc, clientPod)
		util.CreateResourceFromFile(oc, ns, clientPod)
		util.EnsurePodWithLabelReady(oc, ns, clientPodLabel)

		compat_otp.By("3. Verify the record created with the dns name in the DNSNameResolver CR")
		wildcardDnsName := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="*.google.com.")].spec.name}`)
		o.Expect(wildcardDnsName).To(o.ContainSubstring("*.google.com."))

		compat_otp.By("4. Verify the allowed rules which matches the wildcard take effect.")
		util.CheckDomainReachability(oc, clientPodName, ns, "https://www.google.com", true)
		util.CheckDomainReachability(oc, clientPodName, ns, "https://www.redhat.com", false)
		util.CheckDomainReachability(oc, clientPodName, ns, "https://calendar.google.com", true)

		compat_otp.By("5. Confirm the wildcard entry is resolved to dnsName with IP address and TTL value")
		dnsName := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="*.google.com.")].status.resolvedNames[*].dnsName}`)
		o.Expect(dnsName).To(o.And(o.ContainSubstring("www.google.com."), o.ContainSubstring("calendar.google.com.")))
		ttlValues := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="*.google.com.")].status.resolvedNames[*].resolvedAddresses[*].ttlSeconds}`)
		o.Expect(ttlValues).To(o.MatchRegexp(`[0-9]{1,3}`))
		ipAddress := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="*.google.com.")].status.resolvedNames[*].resolvedAddresses[*].ip}`)
		util.ExpectResolvedAddressesForCluster(oc, ipAddress)
	})

	// Bug: OCPBUGS-33750
	// Resolves public DNS names, so cmd/main.go excludes this case on Disconnected clusters
	// and on the External topology.
	//Author:mjoseph
	g.It("DNSNameResolver CR should resolve multiple DNS names [Serial]", g.Label("OCP-75426"), func() {
		if !compat_otp.IsTechPreviewNoUpgrade(oc) {
			g.Skip("featureSet: TechPreviewNoUpgrade is required for this test, skipping")
		}
		var (
			buildPruningBaseDir = FixturePath()
			clientPod           = filepath.Join(buildPruningBaseDir, "test-client-pod.yaml")
			clientPodLabel      = "app=hello-pod"
			clientPodName       = "hello-pod"
			egressFirewall      = filepath.Join(buildPruningBaseDir, "egressfirewall-wildcard.yaml")
			egressFirewall2     = filepath.Join(buildPruningBaseDir, "egressfirewall-multiDomain.yaml")
		)

		compat_otp.By("1. Create four egressfirewall rules and client pods in different namepaces, then wait until there are available")
		var project []string
		for i := range 4 {
			project = append(project, oc.Namespace())
			err := compat_otp.SetNamespacePrivileged(oc, project[i])
			o.Expect(err).NotTo(o.HaveOccurred())
			util.ReplacePayloadImage(oc, clientPod)
			util.CreateResourceFromFile(oc, project[i], clientPod)
			util.OperateResourceFromFile(oc, "create", project[i], egressFirewall)
			util.EnsurePodWithLabelReady(oc, project[i], clientPodLabel)
			util.WaitEgressFirewallApplied(oc, "default", project[i])
			oc.SetupProject()
		}

		compat_otp.By("2. Check whether the default dnsnameresolver CR got created and its resolved dns name")
		wildcardDnsName := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="*.google.com.")].spec.name}`)
		o.Expect(wildcardDnsName).To(o.ContainSubstring("*.google.com."))
		randomNS := util.GetRandomElementFromList(project)
		util.CheckDomainReachability(oc, clientPodName, randomNS, "https://www.google.com", true)

		compat_otp.By("3. Edit some egressfirewalls")
		updateValueTest1 := "[{\"op\":\"replace\",\"path\":\"/spec/egress/0/to/dnsName\", \"value\":\"www.yahoo.com\"}]"
		updateValueTest2 := "[{\"op\":\"add\",\"path\":\"/spec/egress/1\", \"value\":{\"type\":\"Deny\",\"to\":{\"dnsName\":\"www.redhat.com\"}}}]"
		updateValueTest3 := "[{\"op\":\"add\",\"path\":\"/spec/egress/0\", \"value\":{\"type\":\"Deny\",\"to\":{\"dnsName\":\"calendar.google.com\"}}}]"
		updateValueTest4 := "[{\"op\":\"add\",\"path\":\"/spec/egress/1\", \"value\":{\"type\":\"Deny\",\"to\":{\"dnsName\":\"calendar.google.com\"}}}]"
		util.PatchResourceAsAdminAnyType(oc, project[0], "egressfirewall.k8s.ovn.org/default", updateValueTest1, "json")
		util.PatchResourceAsAdminAnyType(oc, project[1], "egressfirewall.k8s.ovn.org/default", updateValueTest2, "json")
		util.PatchResourceAsAdminAnyType(oc, project[2], "egressfirewall.k8s.ovn.org/default", updateValueTest3, "json")
		util.PatchResourceAsAdminAnyType(oc, project[3], "egressfirewall.k8s.ovn.org/default", updateValueTest4, "json")
		util.WaitEgressFirewallApplied(oc, "default", project[0])
		util.WaitEgressFirewallApplied(oc, "default", project[1])
		util.WaitEgressFirewallApplied(oc, "default", project[2])
		util.WaitEgressFirewallApplied(oc, "default", project[3])

		compat_otp.By("4. Check the changes made to dnsnameresolver CR and its resolved dns name in different namespace")
		o.Expect(util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="calendar.google.com.")].spec.name}`)).To(o.ContainSubstring("calendar.google.com."))
		o.Expect(util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="*.google.com.")].spec.name}`)).To(o.ContainSubstring("*.google.com."))
		o.Expect(util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="www.redhat.com.")].spec.name}`)).To(o.ContainSubstring("www.redhat.com."))
		o.Expect(util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="www.yahoo.com.")].spec.name}`)).To(o.ContainSubstring("www.yahoo.com."))
		util.CheckDomainReachability(oc, clientPodName, project[0], "https://www.yahoo.com", true)
		util.CheckDomainReachability(oc, clientPodName, project[0], "https://www.google.com", false)
		util.CheckDomainReachability(oc, clientPodName, project[1], "https://www.google.com", true)
		util.CheckDomainReachability(oc, clientPodName, project[1], "https://www.redhat.com", false)
		util.CheckDomainReachability(oc, clientPodName, project[2], "https://calendar.google.com", false)
		util.CheckDomainReachability(oc, clientPodName, project[2], "https://www.google.com", true)
		util.CheckDomainReachability(oc, clientPodName, project[3], "https://calendar.google.com", true)

		compat_otp.By("5. Delete an egressfirewall and confirm the same")
		err1 := oc.AsAdmin().WithoutNamespace().Run("delete").Args("egressfirewall", "default", "-n", project[0]).Execute()
		o.Expect(err1).NotTo(o.HaveOccurred())
		util.CheckDomainReachability(oc, clientPodName, project[0], "https://www.google.com", true)
		yahooDnsName := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="www.yahoo.com.")].spec.name}`)
		o.Expect(yahooDnsName).To(o.BeEmpty())

		compat_otp.By("6. Recreate an egressfirewall and confirm the same")
		origData, readErr := os.ReadFile(egressFirewall)
		o.Expect(readErr).NotTo(o.HaveOccurred())
		modifiedData := strings.ReplaceAll(string(origData), `"*.google.com"`, "www.amazon.com")
		tmpFile, tmpErr := os.CreateTemp("", "egressfirewall-*.yaml")
		o.Expect(tmpErr).NotTo(o.HaveOccurred())
		defer os.Remove(tmpFile.Name())
		_, writeErr := tmpFile.WriteString(modifiedData)
		o.Expect(writeErr).NotTo(o.HaveOccurred())
		tmpFile.Close()
		util.OperateResourceFromFile(oc, "create", project[0], tmpFile.Name())
		util.WaitEgressFirewallApplied(oc, "default", project[0])
		util.CheckDomainReachability(oc, clientPodName, project[0], "https://www.amazon.com", true)
		amazonDnsName := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="www.amazon.com.")].spec.name}`)
		o.Expect(amazonDnsName).To(o.ContainSubstring("www.amazon.com."))

		compat_otp.By("7. Create another egressfirewall and its client pod in a different namespace")
		project5 := oc.Namespace()
		err2 := compat_otp.SetNamespacePrivileged(oc, project5)
		o.Expect(err2).NotTo(o.HaveOccurred())
		util.OperateResourceFromFile(oc, "create", project5, egressFirewall2)
		util.WaitEgressFirewallApplied(oc, "default", project5)
		util.ReplacePayloadImage(oc, clientPod)
		util.CreateResourceFromFile(oc, project5, clientPod)
		util.EnsurePodWithLabelReady(oc, project5, clientPodLabel)

		compat_otp.By("8. Verify the  three dnsnameresolver records created in DNSNameResolver CR")
		o.Expect(util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="*.google.com.")].spec.name}`)).To(o.ContainSubstring("*.google.com."))
		o.Expect(util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="www.facebook.com.")].spec.name}`)).To(o.ContainSubstring("www.facebook.com."))
		o.Expect(util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="registry-1.docker.io.")].spec.name}`)).To(o.ContainSubstring("registry-1.docker.io."))

		compat_otp.By("9. Verify the dns records are resolved based on allowed rules only")
		util.CheckDomainReachability(oc, clientPodName, project5, "https://www.facebook.com:443", true)
		util.CheckDomainReachability(oc, clientPodName, project5, "https://registry-1.docker.io", true)
		util.CheckDomainReachability(oc, clientPodName, project5, "https://www.facebook.com:80", false)

		compat_otp.By("10. Confirm the dns records are resolved with IP address and TTL value")
		facebookDnsName := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="www.facebook.com.")].status.resolvedNames[*].dnsName}`)
		o.Expect(facebookDnsName).To(o.ContainSubstring("www.facebook.com."))
		dockerDnsName := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="registry-1.docker.io.")].status.resolvedNames[*].dnsName}`)
		o.Expect(dockerDnsName).To(o.ContainSubstring("registry-1.docker.io."))
		ttlValues := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="www.facebook.com.")].status.resolvedNames[*].resolvedAddresses[*].ttlSeconds}`)
		o.Expect(ttlValues).To(o.MatchRegexp(`[0-9]{1,3}`))
		ipAddress := util.GetByJsonPath(oc, "openshift-ovn-kubernetes", "dnsnameresolver", `{.items[?(@.spec.name=="www.facebook.com.")].status.resolvedNames[*].resolvedAddresses[*].ip}`)
		util.ExpectResolvedAddressesForCluster(oc, ipAddress)
	})
})
