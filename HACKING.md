# Cluster DNS Operator Hacking


## Local development

It's possible (and useful) to develop the operator locally targeting a remote cluster.

### Prerequisites

* An OpenShift cluster with at least a master, and compute node. 
* An admin-scoped `KUBECONFIG` for the cluster.
* The [operator-sdk](https://github.com/operator-framework/operator-sdk).

#### GCP test clusters

One reliable and hands-free way to create a suitable test cluster and `KUBECONFIG` in GCP is to use the [openshift/release](https://github.com/openshift/release/tree/master/cluster/test-deploy) tooling.

### Building

To build the operator during development, use the standard Go toolchain:

```
$ go build ./...
```

### Running

To run the operator, first deploy the custom resource definitions:

```
$ oc create -f deploy/crd.yaml
```

Then, use the operator-sdk to launch the operator:

```
$ operator-sdk up local namespace default --kubeconfig=$KUBECONFIG
```

If you're using the `openshift/release` tooling, `KUBECONFIG` will be something like `$RELEASE_REPO/cluster/test-deploy/gcp-dev/admin.kubeconfig`.

To test the default `ClusterDNS` manifest:

```
$ oc create -f deploy/cr.yaml
```

## Tests

### Integration tests

Integration tests are still very immature. To run them, start with an OpenShift 4.0 cluster and then run the following,
substituting for your own details where appropriate. This assumes `KUBECONFIG` is set.

```
# 1. Uninstall any existing cluster-version-operator and cluster-dns-operator.
$ hack/uninstall.sh

# 2. Build and push a new cluster-dns-operator image.
$ REPO=docker.io/username/origin-cluster-dns-operator make release-local

# 3. Run `oc apply` as instructed to install the locally-built operator.

# 4. Run integration tests against the cluster.
$ CLUSTER_NAME=your-cluster-name make test-integration
```

**Important**: Don't run these tests in a cluster where data loss is a concern.

### End-to-end tests

The OpenShift/Kubernetes DNS e2e tests must pass against a cluster using cluster-dns-operator.

To run all of them, try:

```
$ FOCUS='DNS' SKIP='\[Disabled:.+\]|\[Disruptive\]|\[Skipped\]|\[Slow\]|\[Flaky\]|\[local\]|\[Local\]' TEST_ONLY=1 test/extended/conformance.sh
```

To run the bare minimum smoke test, try:

```
$ FOCUS='should provide DNS for the cluster' SKIP='\[Disabled:.+\]|\[Disruptive\]|\[Skipped\]|\[Slow\]|\[Flaky\]|\[local\]|\[Local\]' TEST_ONLY=1 test/extended/conformance.sh
```
