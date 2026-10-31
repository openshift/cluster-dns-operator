# cluster-dns-operator OTE Test Extension

All 28 DNS test cases have been migrated from openshift-tests-private to the OTE (openshift-tests-extension) framework, split across two test files.

## Test Files

| File | Tests | Description |
|------|-------|-------------|
| `dns-operator.go` | 7 | DNS operator lifecycle tests (metrics, placement, logging, protocols) |
| `dns.go` | 21 | CoreDNS configuration tests (forwarding, TLS, caching, search paths, egress) |

## Naming and tagging convention

Every test name has the form:

```text
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS <description> [<execution tags>]
```

Case IDs are Ginkgo labels such as `g.Label("OCP-60350")`, not part of the title.
Environment selectors use these labels, so changing a description does not change which
clusters can run it. These descriptive names form the initial OTE registration metadata.

`[Level0]` must reflect the source case classification, not severity or a desire to
select a conformance suite. The current OTP source for these 28 cases contains no
explicit Level0 marker. Confirm the canonical case IDs against the QE-approved
Level0 plan or Polarion export; the existing tags are retained while that confirmation
is pending. Record the classification source and have the component owner review it.


| Tag | Meaning |
|-----|---------|
| `[sig-network-edge]` | Broad area grouping. |
| `[OTP]` | Ported from openshift-tests-private, for porting-volume tracking. |
| `[Level0]` | Level 0 test; controls conformance parallel/serial suite membership. |
| `[Serial]` / `[Disruptive]` | Execution constraints, honoured by the suite qualifiers below. |

**Renaming a test requires three follow-ups**, in order:

1. Set `spec.OriginalName` to the test's original name in `cmd/main.go`, preserving an
   existing `OriginalName` on subsequent renames. `make update`
   cannot infer which old name belongs to a renamed test — without this, OTE's
   `FindRemovedTestsWithoutRename` treats the old name as deleted and fails with
   *"missing tests, if you've renamed tests you must add their names to OriginalName, or
   mark them obsolete"*.
2. Run `make update` to regenerate
   `.openshift-tests-extension/openshift_payload_cluster-dns-operator.json`, and commit it.
   That file carries the `originalName` which preserves the test's CI history.
3. Open a PR against
   [ci-test-mapping](https://github.com/openshift-eng/ci-test-mapping).

## Where these tests run

All 28 tests are registered with the **informing** lifecycle, so routine failures stay
non-fatal while the suite builds up a reliability record. Drop a test to blocking only once
it demonstrates a ≥99% pass rate.

Discovery-time filtering uses CEL environment selectors in `cmd/main.go`. Runtime
capability guards can also skip a scheduled test before its setup or assertion:

| Exclusion | Applies to | Reason |
|-----------|-----------|--------|
| `topology=="External"` | 37912, 46873, 51946, 52077, 52497, 55821, 56325, 56539, 73379, 75426 | Require operator/control-plane/registry resources or operations unavailable in the supported HyperShift test setup; these are case-specific restrictions. |
| `externalConnectivity=="Disconnected"` | 73379, 75426 | Resolve and reach public DNS names to prove egress firewall and DNSNameResolver behaviour. |
| `topology=="SingleReplica"` | 56325 | Taints separate control-plane and worker nodes. Runtime guards also skip single-node clusters and clusters without dedicated workers, including compact clusters. |

Exclude wins over Include, and a test with neither selector runs everywhere.

### MicroShift

All 28 tests drive `dns.operator.openshift.io`, which MicroShift does not serve. This is
handled by the runtime `util.SkipOnMicroShift()` guard in each `BeforeEach`, **not** by an
environment selector, because neither available selector is correct:

- `topology=="microshift"` **does not exist.** `openshift-tests` derives `--topology` by
  casting `configv1.TopologyMode` directly, and that type declares only `HighlyAvailable`,
  `HighlyAvailableArbiter`, `SingleReplica`, `DualReplica` and `External`. The `"microshift"`
  value in the OTE flag help text is illustrative; a rule written against it silently never
  matches.
- `platform=="none"` is what `openshift/kubernetes` uses — its
  `environment_selectors.go` carries the comment *"MicroShift identifies itself as none"* —
  but platform-agnostic and UPI OpenShift installs report `none` as well, so this would skip
  real clusters. Baremetal **IPI** is not affected (it reports `baremetal`), but the agnostic
  lanes are a genuine false positive.

The guard reads the `microshift-version` configmap in `kube-public` with a bounded API
call. Only a successful read identifies MicroShift; API errors continue to normal test
setup so an unavailable cluster cannot silently pass as a MicroShift skip.

### Disconnected and address-family coverage

Test 55821 checks EDNS buffer size using an existing cluster Service and a nonexistent
cluster-local name, so its positive and NXDOMAIN queries work without public DNS.
Tests 73379 and 75426 validate resolved addresses for the cluster's network stack:
IPv4 on IPv4 clusters, IPv6 on IPv6 clusters, and both on dual-stack clusters.
Test 46867 deliberately validates rendering of both IPv4 and IPv6 upstream addresses;
it does not require connectivity to those synthetic upstreams.

## Test Suites

| Suite | Parent | Tests | Cluster stability |
|-------|--------|-------|-------------------|
| `cluster-dns-operator/conformance/parallel` | `openshift/conformance/parallel` | 7 | Stable |
| `cluster-dns-operator/conformance/serial` | `openshift/conformance/serial` | 2 | Stable |
| `cluster-dns-operator/disruptive` | — | 19 | Disruptive |
| `cluster-dns-operator/non-disruptive` | — | 9 | Stable |
| `cluster-dns-operator/all` | — | 28 | Disruptive |

The serial, disruptive, and all suites advertise `Parallelism: 1`. Direct `run-suite`
invocations cap concurrency at one even when `--max-concurrency` is omitted. A parent
suite controls concurrency when `openshift-tests` runs the child suite through a parent.

Both conformance suites exclude `[Disruptive]` tests. Serial conformance contains only
OCP-73379 and OCP-75426. The 19 disruptive cases remain in the component disruptive
and all suites. They retain `[Serial]` in their titles because they must still run
sequentially. Test names, case labels, and environment exclusions are unchanged.

Serial execution does not make intentional outages compatible with stable-cluster
monitoring. OCP-37912 deliberately waits for DNS `Available=False`. The disruptive and
all suites declare `ClusterStability: Disruptive` so `openshift-tests` uses the appropriate
monitoring when running either suite directly. Cleanup must still restore cluster health.
A parent suite overrides a child's stability and concurrency, so these tests must not be
added back to a stable parent.

Shared CI coverage is planned in separate Origin and `openshift/release` PRs. The
`e2e-aws-netedge-disruptive` profile will provide a manual presubmit for DNS, router, and
ingress operator PRs, plus a daily 5.1 nightly payload periodic. It will select Origin's
`openshift/network-edge/disruptive` suite with `TEST_TYPE: suite-conformance`.
The shared suite will select `[Disruptive]` tests from exactly the `cluster-dns-operator`,
`router`, and `cluster-ingress-operator` payload extensions, preserving their environment
filters. It will run selected cases sequentially with disruptive monitoring, followed by
stable parallel conformance to verify recovery.

The DNS disruptive suite has no parent: `openshift/disruptive` defaults to concurrent
execution and overrides a child's `Parallelism: 1`. The planned shared NetEdge suite will
use its own source-scoped qualifier, `Parallelism: 1`, and disruptive stability. Neither
`openshift/disruptive` nor `openshift/disruptive-longrunning` selects these DNS cases.
Regular CI coverage for the 19 disruptive cases requires those follow-up PRs; until then,
run the component disruptive suite explicitly.


### cluster-dns-operator/non-disruptive

9 non-disruptive tests (safe to run in parallel):

- 26151 - Integrate DNS operator metrics with Prometheus
- 39842 - CoreDNS supports dual stack ClusterIP Services for OCP4.8 or higher
- 55821 - Check CoreDNS default bufsize, readinessProbe path and policy
- 56884 - Confirm the coreDNS version and Kubernetes version of the oc client
- 60350 - Check the max number of domains in the search path list of any pod
- 60492 - Check the max number of characters in the search path of any pod
- 63553 - Annotation 'TopologyAwareHints' presents should not cause any pathological events
- 73379 - DNSNameResolver CR get updated with IP addresses and TTL of the DNS name [Serial]
- 75426 - DNSNameResolver CR should resolve multiple DNS names [Serial]

> **Note**: Tests 73379 and 75426 retain the `[Serial]` tag but are parallel-safe by design — all `dnsnameresolver` queries use jsonpath filters scoped to specific CRs, so they do not interfere with other tests.

### cluster-dns-operator/disruptive

19 disruptive tests (may affect cluster state, all serial):

- 37912 - DNS operator should show clear error message when DNS service IP already allocated
- 40718 - CoreDNS cache should use 900s for positive responses and 30s for negative responses
- 40867 - Deleting the internal registry should not corrupt /etc/hosts
- 41049 - DNS controls pod placement by node selector
- 41050 - DNS controller pod placement by tolerations
- 46183 - DNS operator supports Random, RoundRobin and Sequential policy for servers.forwardPlugin
- 46867 - Configure upstream resolvers for CoreDNS flag
- 46869 - Negative test of configuring upstream resolvers and policy flag
- 46872 - Configure logLevel for CoreDNS under DNS operator flag
- 46873 - Configure operatorLogLevel under the default dns operator and check the logs flag
- 46874 - Negative test for configuring logLevel and operatorLogLevel flag
- 46875 - Different LogLevel logging function of CoreDNS flag
- 51946 - Support CoreDNS forwarding DNS requests over TLS using UpstreamResolvers
- 52077 - CoreDNS forwarding DNS requests over TLS with CLEAR TEXT
- 52497 - Support CoreDNS forwarding DNS requests over TLS - using system CA
- 54042 - Configuring CoreDNS caching and TTL parameters
- 56325 - DNS pod should not work on nodes with taint configured
- 56539 - Disabling the internal registry should not corrupt /etc/hosts
- 63512 - Enabling force_tcp for protocolStrategy field to allow DNS queries to send on TCP to upstream server

## How to Run

```bash
# Run these commands from the repository root
cd tests-extension

export KUBECONFIG=/path/to/kubeconfig

# Build the binary (CGO_ENABLED=0, GO_COMPLIANCE_POLICY=exempt_all)
make build

# Regenerate the extension metadata after renaming or adding a test
make update

# Fail if the committed metadata is stale (suitable for CI)
make verify

# List all available suites
./bin/cluster-dns-operator-tests-ext list suites

# List all tests
./bin/cluster-dns-operator-tests-ext list tests
```

`list`, `update` and `info` all require `KUBECONFIG` to be set, because the origin test
framework initialises a cluster client at startup. Any syntactically valid kubeconfig will
do for `list` and `update`; the cluster is not contacted.

### Run All Tests

```bash
# Run all 28 tests (serially recommended)
./bin/cluster-dns-operator-tests-ext run-suite cluster-dns-operator/all --max-concurrency=1
```

### Run by Suite

```bash
# Run non-disruptive tests (parallel-safe, default concurrency is fine)
./bin/cluster-dns-operator-tests-ext run-suite cluster-dns-operator/non-disruptive

# Run only disruptive tests (serially)
./bin/cluster-dns-operator-tests-ext run-suite cluster-dns-operator/disruptive --max-concurrency=1

# Run serial conformance (only the two non-disruptive DNSNameResolver cases)
./bin/cluster-dns-operator-tests-ext run-suite cluster-dns-operator/conformance/serial
```

To use the cluster monitors, run the suite through `openshift-tests` with the DNS
extension available in the payload:

```bash
openshift-tests run cluster-dns-operator/disruptive
```

The extension binary's `run-suite` command runs test cases directly; it does not start
Origin's cluster monitors.

### Run a Single Test

```bash
./bin/cluster-dns-operator-tests-ext run-test \
  -n "[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Check the max number of domains in the search path list of any pod"
```

**Important**: 19 of 28 tests are `[Disruptive] [Serial]` and must run sequentially.
Tests that modify the shared `dns.operator.openshift.io/default` resource capture its
original spec and the test node-label values with `PreserveDNSConfig` before mutation.
Their deferred cleanup restores those values without deleting the CR, preserves unrelated
node labels, retries update conflicts, and waits for DNS to become healthy. Running these
mutations concurrently still causes interference. The OTE `run-suite` command does not parse `[Serial]` from test names; the
suite's `Parallelism: 1` enforces sequential execution.

Tests 73379 and 75426 retain `[Serial]` but are parallel-safe — all `dnsnameresolver` queries use jsonpath filters scoped to specific CRs (e.g., `{.items[?(@.spec.name=="*.google.com.")].spec.name}`), so they do not read or interfere with resources from other tests (a difference with openshift-tests-private repo).

The 9 non-disruptive tests can run in parallel. New suites that include disruptive tests
must also set `Parallelism: 1`.

## Pre-merge CI validation

The OTE integration guide recommends running the payload jobs below on any PR that adds or
renames tests; substitute the current release for `5.1`. The metal IPv6 job in particular is
the lane that surfaced the OCPBUGS-132070 failures.

```text
/payload-job periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn-conformance
/payload-job periodic-ci-openshift-release-master-nightly-5.1-e2e-metal-ipi-ovn-ipv6
/payload-job periodic-ci-openshift-release-master-ci-5.1-e2e-aws-upgrade-ovn-single-node
/payload-job periodic-ci-openshift-hypershift-release-5.1-periodics-e2e-aws-ovn-conformance-serial
/payload-job periodic-ci-openshift-release-master-nightly-5.1-e2e-aws-ovn-single-node-serial

# Planned shared coverage; available after the separate Origin and release PRs merge
/payload-job periodic-ci-openshift-release-main-nightly-5.1-e2e-aws-netedge-disruptive
```

If the binary registration PR in `openshift/origin` has not merged yet, test both together:

```text
/payload-job-with-prs <job_name> openshift/origin#<PR_NUMBER>
```

## Test Execution Time

- Single test: 2-10 minutes
- Non-disruptive suite: 5-10 minutes
- Disruptive suite: 30-60 minutes
- All tests: 40-70 minutes

## Files

```text
tests-extension/
├── .openshift-tests-extension/
│   └── openshift_payload_cluster-dns-operator.json  # Test metadata, generated by `make update`
├── cmd/main.go                         # OTE entry point, suites, renames, environment selectors
├── test/
│   ├── util.go                         # Shared helper functions (package util)
│   └── qe/
│       ├── dns-operator.go             # 7 DNS operator test implementations
│       ├── dns.go                      # 21 CoreDNS configuration test implementations
│       ├── bindata.go                  # Embedded test fixtures (auto-generated)
│       ├── fixtures.go                 # FixturePath helper
│       ├── bindata.mk                  # Bindata generation makefile
│       ├── README.md                   # This file
│       └── testdata/
│           ├── coreDNS-pod.yaml        # CoreDNS server pod (upstream resolver tests)
│           ├── test-client-pod.yaml    # Client pod for dig/nslookup/curl assertions
│           ├── testpod-60350.yaml      # Pod with 32 search domains (test 60350)
│           ├── testpod-60492.yaml      # Pod with 253-char search domain (test 60492)
│           ├── web-server-v4v6rc.yaml  # Dual-stack workload and Services (test 39842)
│           ├── egressfirewall-wildcard.yaml     # Wildcard egress firewall (test 73379)
│           └── egressfirewall-multiDomain.yaml  # Multi-domain egress firewall (test 75426)
├── Makefile                            # Build / update / verify targets
├── go.mod                              # Dependencies
├── go.sum                              # Dependency checksums
└── vendor/                             # Vendored dependencies
```

## All Test Names

### dns-operator.go (7 tests)

```text
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Integrate DNS operator metrics with Prometheus
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS DNS operator should show clear error message when DNS service IP already allocated [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS DNS controls pod placement by node selector [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS DNS controller pod placement by tolerations [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS DNS operator supports Random, RoundRobin and Sequential policy for servers.forwardPlugin [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Configure operatorLogLevel under the default dns operator and check the logs flag [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Enabling force_tcp for protocolStrategy field to allow DNS queries to send on TCP to upstream server [Disruptive] [Serial]
```

### dns.go (21 tests)

```text
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS CoreDNS supports dual stack ClusterIP Services for OCP4.8 or higher
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS CoreDNS cache should use 900s for positive responses and 30s for negative responses [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Deleting the internal registry should not corrupt /etc/hosts [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Configure upstream resolvers for CoreDNS flag [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Negative test of configuring upstream resolvers and policy flag [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Configure logLevel for CoreDNS under DNS operator flag [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Negative test for configuring logLevel and operatorLogLevel flag [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Different LogLevel logging function of CoreDNS flag [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Support CoreDNS forwarding DNS requests over TLS using UpstreamResolvers [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS CoreDNS forwarding DNS requests over TLS with CLEAR TEXT [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Support CoreDNS forwarding DNS requests over TLS - using system CA [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Configuring CoreDNS caching and TTL parameters [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Check CoreDNS default bufsize, readinessProbe path and policy
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS DNS pod should not work on nodes with taint configured [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Disabling the internal registry should not corrupt /etc/hosts [Disruptive] [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Confirm the coreDNS version and Kubernetes version of the oc client
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Check the max number of domains in the search path list of any pod
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Check the max number of characters in the search path of any pod
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS Annotation 'TopologyAwareHints' presents should not cause any pathological events
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS DNSNameResolver CR get updated with IP addresses and TTL of the DNS name [Serial]
[sig-network-edge][OTP][Level0] Network_Edge Component_DNS DNSNameResolver CR should resolve multiple DNS names [Serial]
```
