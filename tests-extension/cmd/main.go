package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/component-base/logs"

	"github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	e "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
	g "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"
	"github.com/openshift/origin/test/extended/util"
	compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
	framework "k8s.io/kubernetes/test/e2e/framework"

	// Import test packages from this module
	_ "github.com/openshift/cluster-dns-operator-tests-extension/test/qe"
)

// hyperShiftGuestUnsupported lists case labels for tests that read or mutate resources
// found only on a standalone control plane - the openshift-dns-operator and
// openshift-ingress-operator namespaces, master nodes, and configs.imageregistry/cluster.
// On a HyperShift guest cluster those are absent, so the tests are excluded there rather
// than carrying a NonHyperShiftHOST marker in the name, which OTE cannot act on.
// OCP-26151 reads the same openshift-dns-operator namespace but is deliberately *not*
// listed: it passed on a topology=="External" cluster in the
// periodics-e2e-aws-ovn-conformance payload job, so the namespace, servicemonitor and
// rolebinding it asserts do exist on a HyperShift guest. Adding it would exclude a test
// that demonstrably works there.
var hyperShiftGuestUnsupported = []string{
	"OCP-37912", "OCP-46873", "OCP-51946", "OCP-52077", "OCP-52497",
	"OCP-55821", "OCP-56325", "OCP-56539", "OCP-73379", "OCP-75426",
}

// externalConnectivityRequired lists case labels for tests that resolve and reach public
// DNS names (google.com, redhat.com, facebook.com, registry-1.docker.io) to prove egress
// firewall and DNSNameResolver behaviour. They cannot work on a disconnected cluster.
var externalConnectivityRequired = []string{"OCP-73379", "OCP-75426"}

// main registers the DNS tests and exposes the OTE commands.
func main() {
	util.InitStandardFlags()
	// Set the framework's existing fallback before initialization so its startup
	// message does not precede the JSON returned by list and info.
	if framework.TestContext.Provider == "" {
		framework.TestContext.Provider = "skeleton"
	}
	framework.AfterReadingAllFlags(&framework.TestContext)

	logs.InitLogs()
	defer logs.FlushLogs()

	registry := e.NewRegistry()
	ext := e.NewExtension("openshift", "payload", "cluster-dns-operator")

	registerSuites(ext)

	allSpecs, err := g.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite()
	if err != nil {
		panic(fmt.Sprintf("couldn't build extension test specs from ginkgo: %+v", err.Error()))
	}

	componentSpecs := allSpecs.Select(func(spec *et.ExtensionTestSpec) bool {
		for _, loc := range spec.CodeLocations {
			if strings.Contains(loc, "/test/qe/") && !strings.Contains(loc, "/go/pkg/mod/") && !strings.Contains(loc, "/vendor/") {
				return true
			}
		}
		return false
	})

	componentSpecs.AddBeforeAll(func() {
		if err := compat_otp.InitTest(false); err != nil {
			panic(err)
		}
		util.WithCleanup(func() {})
	})

	componentSpecs.Walk(func(spec *et.ExtensionTestSpec) {
		for label := range spec.Labels {
			if strings.HasPrefix(label, "Platform:") {
				platformName := strings.TrimPrefix(label, "Platform:")
				spec.Include(et.PlatformEquals(platformName))
			}
		}

		re := regexp.MustCompile(`\[platform:([a-z]+)\]`)
		if match := re.FindStringSubmatch(spec.Name); match != nil {
			platform := match[1]
			spec.Include(et.PlatformEquals(platform))
		}

		// Ported tests land informing: routine failures stay non-fatal while the suite
		// builds up a reliability record, per the OTE integration guide.
		spec.Lifecycle = et.LifecycleInforming
	})

	applyEnvironmentSelectors(componentSpecs)

	ext.AddSpecs(componentSpecs)

	registry.Register(ext)

	root := &cobra.Command{
		Long: "Cluster DNS Operator Tests",
	}

	root.AddCommand(cmd.DefaultExtensionCommands(registry)...)

	if err := func() error {
		return root.Execute()
	}(); err != nil {
		os.Exit(1)
	}
}

// applyEnvironmentSelectors restricts where each test runs. Environment selectors are the
// only mechanism openshift-tests honours; markers embedded in a test name are not
// executable, which is how the ingress-operator tests in OCPBUGS-132070 ended up failing
// 10/10 on MicroShift. Exclude wins over Include, and a test with neither runs everywhere.
func applyEnvironmentSelectors(specs et.ExtensionTestSpecs) {
	// MicroShift. origin's determineEnvironmentFlags reports a MicroShift cluster as
	// topology "SingleReplica" and platform "none":
	//
	//   if config.SingleReplicaTopology {        // "In cases like Microshift, we will not
	//       envFlagBuilder.AddTopology(...)      //  be able to determine the clusterState"
	//   }
	//   if platform == "skeleton" { platform = "none" }
	//
	// Neither flag, alone or combined, identifies MicroShift. A platform-agnostic or UPI
	// single-node OpenShift cluster reports exactly the same pair, so excluding on
	// topology=="SingleReplica" && platform=="none" would silently drop all 28 tests on a
	// supported OpenShift topology - and would do so at list time, before
	// util.SkipOnMicroShift ever runs and could tell the two apart. Excluding bare
	// topology=="SingleReplica", as openshift/images#256 does, drops even more. These
	// tests do support SNO (util.ForceOnlyOneDnsPodExist has a single-pod path, and
	// OCP-41049 self-skips there), and SNO is one of the payload lanes the OTE guide asks
	// us to run, so no selector tries to guess here.
	//
	// util.SkipOnMicroShift in test/qe is the authoritative gate instead: it looks for the
	// microshift-version configmap, which is a positive identification rather than an
	// inference, and it runs before any framework setup. On MicroShift all 28 specs are
	// listed and then skipped; applicable tests remain eligible on SNO.
	//
	// topology=="microshift" is kept as a defensive clause: configv1.TopologyMode declares
	// no such value today, so it never matches, but the OTE --topology flag documents it
	// and a non-origin orchestrator could emit it.
	specs.Exclude(et.TopologyEquals("microshift"))

	specs.SelectAny(hasAnyCaseLabel(hyperShiftGuestUnsupported)).
		Exclude(et.TopologyEquals("External"))

	specs.SelectAny(hasAnyCaseLabel(externalConnectivityRequired)).
		Exclude(et.ExternalConnectivityEquals("Disconnected"))

	// The taint test needs separate control-plane and worker nodes. Its runtime
	// guard also protects direct run-test/run-suite calls, which bypass selectors.
	specs.Select(et.HasLabel("OCP-56325")).Exclude(et.TopologyEquals("SingleReplica"))
}

// hasAnyCaseLabel builds selectors for stable case labels rather than test titles.
func hasAnyCaseLabel(labels []string) []et.SelectFunction {
	selectFns := make([]et.SelectFunction, 0, len(labels))
	for _, label := range labels {
		selectFns = append(selectFns, et.HasLabel(label))
	}
	return selectFns
}

// registerSuites advertises DNS suites and serializes suites that can disrupt DNS.
func registerSuites(ext *e.Extension) {
	suites := []e.Suite{
		{
			Name: "cluster-dns-operator/conformance/parallel",
			Parents: []string{
				"openshift/conformance/parallel",
			},
			Description: "Parallel conformance tests (non-serial, non-disruptive)",
			Qualifiers: []string{
				`name.contains("[Level0]") && !(name.contains("[Serial]") || name.contains("[Disruptive]"))`,
			},
		},
		// Serial conformance requires a stable cluster. Intentional DNS outages
		// belong in the disruptive suite even when each test restores its changes.
		{
			Name: "cluster-dns-operator/conformance/serial",
			Parents: []string{
				"openshift/conformance/serial",
			},
			Description: "Serial conformance tests (non-disruptive)",
			Parallelism: 1,
			Qualifiers: []string{
				`name.contains("[Level0]") && name.contains("[Serial]") && !name.contains("[Disruptive]")`,
			},
		},
		// Run this suite directly: parent suites override Parallelism, and DNS
		// mutations must never overlap even when the parent tolerates disruption.
		{
			Name:             "cluster-dns-operator/disruptive",
			Description:      "Disruptive tests (may affect cluster state)",
			Parallelism:      1,
			ClusterStability: e.ClusterStabilityDisruptive,
			Qualifiers: []string{
				`name.contains("[Disruptive]")`,
			},
		},
		{
			Name:        "cluster-dns-operator/non-disruptive",
			Description: "All non-disruptive tests (safe for development clusters)",
			Qualifiers: []string{
				`!name.contains("[Disruptive]")`,
			},
		},
		{
			Name:             "cluster-dns-operator/all",
			Description:      "All cluster-dns-operator tests",
			Parallelism:      1,
			ClusterStability: e.ClusterStabilityDisruptive,
		},
	}

	for _, suite := range suites {
		ext.AddSuite(suite)
	}
}
