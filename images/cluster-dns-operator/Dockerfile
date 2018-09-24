FROM centos:7

RUN yum install -y golang make

ENV GOPATH /go
RUN mkdir $GOPATH

COPY . $GOPATH/src/github.com/openshift/cluster-dns-operator

RUN cd $GOPATH/src/github.com/openshift/cluster-dns-operator \
    && make build \
    && cp $GOPATH/src/github.com/openshift/cluster-dns-operator/cluster-dns-operator /usr/bin/

RUN yum remove -y golang make

ENTRYPOINT ["/usr/bin/cluster-dns-operator"]

USER 1001

LABEL io.k8s.display-name="OpenShift cluster-dns-operator" \
      io.k8s.description="This is a component of OpenShift Container Platform and manages the lifecycle of cluster DNS services." \
      maintainer="Dan Mace <dmace@redhat.com>"

COPY manifests /manifests
LABEL io.openshift.release.operator true
