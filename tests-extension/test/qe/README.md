# cluster-dns-operator OTE Test Extension

All 28 DNS test cases have been migrated from openshift-tests-private to the OTE (openshift-tests-extension) framework, split across two test files.

## Test Files

| File | Tests | Description |
|------|-------|-------------|
| `dns-operator.go` | 7 | DNS operator lifecycle tests (metrics, placement, logging, protocols) |
| `dns.go` | 21 | CoreDNS configuration tests (forwarding, TLS, caching, search paths, egress) |

## Test Suites

### cluster-dns-operator/all
All 28 tests (use `--max-concurrency=1` since most are disruptive)

### cluster-dns-operator/non-disruptive
9 non-disruptive tests (safe to run in parallel):
- 26151 - Integrate DNS operator metrics with Prometheus
- 39842 - CoreDNS supports dual stack ClusterIP Services
- 55821 - Check CoreDNS default bufsize, readinessProbe path and policy
- 56884 - Confirm the coreDNS version and Kubernetes version
- 60350 - Check the max number of domains in the search path list
- 60492 - Check the max number of characters in the search path
- 63553 - Annotation 'TopologyAwareHints' presents should not cause pathological events
- 73379 - DNSNameResolver CR get updated with IP addresses and TTL [Serial]
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
- 46873 - Configure operatorLogLevel under the default dns operator
- 46874 - Negative test for configuring logLevel and operatorLogLevel flag
- 46875 - Different LogLevel logging function of CoreDNS flag
- 51946 - Support CoreDNS forwarding DNS requests over TLS using UpstreamResolvers
- 52077 - CoreDNS forwarding DNS requests over TLS with CLEAR TEXT
- 52497 - Support CoreDNS forwarding DNS requests over TLS - using system CA
- 54042 - Configuring CoreDNS caching and TTL parameters
- 56325 - DNS pod should not work on nodes with taint configured
- 56539 - Disabling the internal registry should not corrupt /etc/hosts
- 63512 - Enabling force_tcp for protocolStrategy field

### cluster-dns-operator/conformance/parallel
Level0 non-serial, non-disruptive tests

### cluster-dns-operator/conformance/serial
Level0 serial, non-disruptive tests

## How to Run

```bash
# Run these commands from the repository root
cd tests-extension

export KUBECONFIG=/path/to/kubeconfig

# Build the binary
make build

# List all available suites
./bin/cluster-dns-operator-tests-ext list suites

# List all tests
./bin/cluster-dns-operator-tests-ext list tests
```

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
```

### Run Only dns-operator.go Tests

```bash
# Run all 7 dns-operator tests by name filter
./bin/cluster-dns-operator-tests-ext run-test \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-26151-Integrate DNS operator metrics with Prometheus [Skipped:MicroShift]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-37912-DNS operator should show clear error message when DNS service IP already allocated [Disruptive] [Serial] [Skipped:MicroShift]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-41049-DNS controls pod placement by node selector [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-41050-DNS controller pod placement by tolerations [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:hongli-High-46183-DNS operator supports Random, RoundRobin and Sequential policy for servers.forwardPlugin [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-NonHyperShiftHOST-Medium-46873-Configure operatorLogLevel under the default dns operator and check the logs flag [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-High-63512-Enbaling force_tcp for protocolStrategy field to allow DNS queries to send on TCP to upstream server [Disruptive] [Serial]" --max-concurrency=1
```

### Run Only dns.go Tests

```bash
# Run all 21 dns.go tests by name filter
./bin/cluster-dns-operator-tests-ext run-test \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-High-39842-CoreDNS supports dual stack ClusterIP Services for OCP4.8 or higher" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Critical-40718-CoreDNS cache should use 900s for positive responses and 30s for negative responses [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-High-40867-Deleting the internal registry should not corrupt /etc/hosts [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Critical-46867-Configure upstream resolvers for CoreDNS flag [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Medium-46869-Negative test of configuring upstream resolvers and policy flag [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Critical-46872-Configure logLevel for CoreDNS under DNS operator flag [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Medium-46874-negative test for configuring logLevel and operatorLogLevel flag [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Low-46875-Different LogLevel logging function of CoreDNS flag [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-Critical-51946-Support CoreDNS forwarding DNS requests over TLS using UpstreamResolvers [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-52077-CoreDNS forwarding DNS requests over TLS with CLEAR TEXT [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-52497-Support CoreDNS forwarding DNS requests over TLS - using system CA [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-54042-Configuring CoreDNS caching and TTL parameters [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-55821-Check CoreDNS default bufsize, readinessProbe path and policy" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-56325-DNS pod should not work on nodes with taint configured [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-Longduration-NonPreRelease-High-56539-Disabling the internal registry should not corrupt /etc/hosts [Disruptive] [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-ROSA-OSD_CCS-ARO-Critical-56884-Confirm the coreDNS version and Kubernetes version of the oc client" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-60350-Check the max number of domains in the search path list of any pod" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-60492-Check the max number of characters in the search path of any pod" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-ROSA-OSD_CCS-ARO-High-63553-Annotation 'TopologyAwareHints' presents should not cause any pathological events" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-ConnectedOnly-Critical-73379-DNSNameResolver CR get updated with IP addresses and TTL of the DNS name [Serial]" \
  -n "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-ConnectedOnly-High-75426-DNSNameResolver CR should resolve multiple DNS names [Serial]" --max-concurrency=1
```

### Run a Single Test

```bash
# Run a single test by its full name
./bin/cluster-dns-operator-tests-ext run-test "[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-60350-Check the max number of domains in the search path list of any pod"
```

**Important**: 19 of 28 tests are `[Disruptive] [Serial]` and **must** run with `--max-concurrency=1`. Disruptive tests modify the shared `dns.operator.openshift.io/default` resource and call `DeleteDnsOperatorToRestore` in defers, so running them concurrently causes cascading failures. The OTE `run-suite` command does not parse `[Serial]` from test names to enforce serial execution — it defaults to `--max-concurrency=10`.

Tests 73379 and 75426 retain `[Serial]` but are parallel-safe — all `dnsnameresolver` queries use jsonpath filters scoped to specific CRs (e.g., `{.items[?(@.spec.name=="*.google.com.")].spec.name}`), so they do not read or interfere with resources from other tests (a difference with openshift-tests-private repo).

The 9 non-disruptive tests can run in parallel. Always use `--max-concurrency=1` for any suite that includes disruptive tests.

## Test Execution Time

- Single test: 2-10 minutes
- Non-disruptive suite: 5-10 minutes
- Disruptive suite: 30-60 minutes
- All tests: 40-70 minutes

## Files

```text
tests-extension/
├── cmd/main.go                         # OTE entry point, suite definitions
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
│           ├── test-client-pod.yaml    # Client pod for egress firewall tests
│           ├── testpod-60350.yaml      # Pod with 32 search domains (test 60350)
│           ├── testpod-60492.yaml      # Pod with 253-char search domain (test 60492)
│           ├── web-server-v4v6rc.yaml  # Dual-stack web server (test 39842)
│           ├── egressfirewall-wildcard.yaml     # Wildcard egress firewall (test 73379)
│           └── egressfirewall-multiDomain.yaml  # Multi-domain egress firewall (test 75426)
├── Makefile                            # Build targets
├── go.mod                              # Dependencies
├── go.sum                              # Dependency checksums
└── vendor/                             # Vendored dependencies
```

## All Test Names

### dns-operator.go (7 tests)

```text
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-26151-Integrate DNS operator metrics with Prometheus [Skipped:MicroShift]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-37912-DNS operator should show clear error message when DNS service IP already allocated [Disruptive] [Serial] [Skipped:MicroShift]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-41049-DNS controls pod placement by node selector [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-41050-DNS controller pod placement by tolerations [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:hongli-High-46183-DNS operator supports Random, RoundRobin and Sequential policy for servers.forwardPlugin [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-NonHyperShiftHOST-Medium-46873-Configure operatorLogLevel under the default dns operator and check the logs flag [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-High-63512-Enbaling force_tcp for protocolStrategy field to allow DNS queries to send on TCP to upstream server [Disruptive] [Serial]
```

### dns.go (21 tests)

```text
[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-High-39842-CoreDNS supports dual stack ClusterIP Services for OCP4.8 or higher
[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Critical-40718-CoreDNS cache should use 900s for positive responses and 30s for negative responses [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-High-40867-Deleting the internal registry should not corrupt /etc/hosts [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Critical-46867-Configure upstream resolvers for CoreDNS flag [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Medium-46869-Negative test of configuring upstream resolvers and policy flag [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Critical-46872-Configure logLevel for CoreDNS under DNS operator flag [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Medium-46874-negative test for configuring logLevel and operatorLogLevel flag [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:shudili-Low-46875-Different LogLevel logging function of CoreDNS flag [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-Critical-51946-Support CoreDNS forwarding DNS requests over TLS using UpstreamResolvers [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-52077-CoreDNS forwarding DNS requests over TLS with CLEAR TEXT [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-52497-Support CoreDNS forwarding DNS requests over TLS - using system CA [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-54042-Configuring CoreDNS caching and TTL parameters [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-55821-Check CoreDNS default bufsize, readinessProbe path and policy
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-High-56325-DNS pod should not work on nodes with taint configured [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-Longduration-NonPreRelease-High-56539-Disabling the internal registry should not corrupt /etc/hosts [Disruptive] [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-ROSA-OSD_CCS-ARO-Critical-56884-Confirm the coreDNS version and Kubernetes version of the oc client
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-60350-Check the max number of domains in the search path list of any pod
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-Critical-60492-Check the max number of characters in the search path of any pod
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-ROSA-OSD_CCS-ARO-High-63553-Annotation 'TopologyAwareHints' presents should not cause any pathological events
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-ConnectedOnly-Critical-73379-DNSNameResolver CR get updated with IP addresses and TTL of the DNS name [Serial]
[OTP][sig-network-edge] Network_Edge Component_DNS Author:mjoseph-NonHyperShiftHOST-ConnectedOnly-High-75426-DNSNameResolver CR should resolve multiple DNS names [Serial]
```
