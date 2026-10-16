FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.26-openshift-5.0 AS builder
WORKDIR /dns-operator
COPY . .
RUN make build

# Test extension builder stage (added by ote-migration)
FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.26-openshift-5.0 AS test-extension-builder
WORKDIR /build/cluster-dns-operator
COPY tests-extension/ tests-extension/
RUN cd tests-extension && \
    GOFLAGS=-mod=vendor make build && \
    cd bin && \
    gzip -n cluster-dns-operator-tests-ext

FROM registry.ci.openshift.org/ocp/5.0:base-rhel9
COPY --from=builder /dns-operator/dns-operator /usr/bin/
# Copy test extension binary (raw gzip per OTE convention)
COPY --from=test-extension-builder /build/cluster-dns-operator/tests-extension/bin/cluster-dns-operator-tests-ext.gz /usr/bin/
COPY manifests /manifests
RUN useradd dns-operator
USER dns-operator
ENTRYPOINT ["/usr/bin/dns-operator"]
LABEL io.openshift.release.operator true
LABEL io.k8s.display-name="OpenShift dns-operator" \
      io.k8s.description="This is a component of OpenShift and manages the lifecycle of cluster DNS services." \
      maintainer="Dan Mace <dmace@redhat.com>"
