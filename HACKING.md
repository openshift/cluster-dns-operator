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

Integration tests are still very immature. To run them, use the GCP test cluster instructions and then run the tests with:

```
KUBECONFIG=</path/to/admin.kubeconfig> make test-integration
```

**Important**: Note that the resources and namespaces used for the test are currently fixed and the tests will clean up after themselves, including deleting the `openshift-cluster-dns` namespace. Don't run these tests in a cluster where data loss is a concern.
