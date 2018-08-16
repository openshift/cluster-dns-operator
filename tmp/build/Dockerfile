FROM alpine:3.6

RUN adduser -D cluster-dns-operator
USER cluster-dns-operator

ADD tmp/_output/bin/cluster-dns-operator /usr/local/bin/cluster-dns-operator
