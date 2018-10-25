FROM openshift/origin-release:golang-1.10 as builder
COPY . /go/src/github.com/openshift/cluster-dns-operator/
RUN cd /go/src/github.com/openshift/cluster-dns-operator && make build

FROM centos:7
LABEL io.openshift.release.operator true
LABEL io.k8s.display-name="OpenShift cluster-dns-operator" \
      io.k8s.description="This is a component of OpenShift Container Platform and manages the lifecycle of cluster DNS services." \
      maintainer="Dan Mace <dmace@redhat.com>"

COPY --from=builder /go/src/github.com/openshift/cluster-dns-operator/cluster-dns-operator /usr/bin/
COPY manifests /manifests

RUN useradd cluster-dns-operator
USER cluster-dns-operator

ENTRYPOINT ["/usr/bin/cluster-dns-operator"]
