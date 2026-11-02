# The DNS Operator Hacking

## Building

To build the operator, run:

```
$ make build
```

## Developing

### Prerequisites

* An [OpenShift cluster](https://github.com/openshift/installer)
* An admin-scoped `KUBECONFIG` for the cluster.

#### Building Locally

Uninstall the existing operator and all its managed components:

```
$ hack/uninstall.sh
```

Build a new image and custom manifests:

```
$ REPO=docker.io/you/origin-cluster-dns-operator make release-local
```

Follow the instructions to install the operator, e.g.:

```
$ oc apply -f /tmp/manifests/path
```

#### Building Remotely on the Cluster

To build the operator on the remote cluster, first create a buildconfig on the cluster:

```
$ make buildconfig
```

The above command will create a buildconfig using the current branch and the URL for the default push remote.  You can also specify an explicit branch or repository URL:

```
$ make buildconfig GIT_BRANCH=<branch> \
     GIT_URL=https://github.com/<username>/cluster-dns-operator.git
```

Example -

```
 GIT_BRANCH=NE-218  GIT_URL=https://github.com/miheer/cluster-dns-operator make buildconfig
```

Note: If a buildconfig already exists from an earlier `make buildconfig` command, `make buildconfig` will update the existing buildconfig.

Next, start a build from this buildconfig:

```
$ make cluster-build
```

Alternatively, if you want to see the logs during the build, specify the `V` flag:

```
$ make cluster-build V=1
```

Use the `DEPLOY` flag to start a build and then patch the operator to use the newly built image:

```
$ make cluster-build DEPLOY=1
```

Example -
```
 make cluster-build DEPLOY=1 V=1
```

## Tests

### Formatting Verification

Run `hack/verify-gofmt.sh` from the repository root to check regular Go source
files without modifying them. The same check runs as part of `make verify`.
Directories named `vendor` or `_output` are excluded at any depth. First-party
code under `tests-extension`, including generated Go code, remains checked.

The check uses `gofmt` from `PATH` by default. To use the formatter from the
Go toolchain selected for this module, run:

```bash
GOFMT="$(go env GOROOT)/bin/gofmt" hack/verify-gofmt.sh
```

`GOFMT` must name a single executable, not a command with arguments. Both
formatting differences and formatter execution errors fail verification.
Inspect reported files with `gofmt -s -d` and quote their paths; do not pipe
the output back into a command that splits filenames on whitespace.

Generated-file findings must be addressed through their generation workflow,
not by excluding or hand-editing the generated file. Bindata generation is
defined in `tests-extension/test/qe/bindata.mk`; verification does not regenerate
it automatically.

### Unit And End-To-End Tests

Run unit tests:

```
$ make test
```

Assuming `KUBECONFIG` is set, run end-to-end tests:

```
$ make test-e2e
```
