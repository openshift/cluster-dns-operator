# Cluster DNS Operator

Cluster DNS Operator enables [DNS-based Kubernetes Service discovery](https://kubernetes.io/docs/concepts/services-networking/service/#dns) in [OpenShift](https://openshift.io) by managing [CoreDNS](https://coredns.io).

* The default cluster domain is `cluster.local`.
* Configuration of the CoreDNS [Corefile](https://coredns.io/manual/toc/#configuration) or [kubernetes plugin](https://coredns.io/plugins/kubernetes/) are not yet supported.

The operator tries to be useful by default.

## How it works

Cluster DNS Operator manages CoreDNS as a Kubernetes DaemonSet exposed as a Service with a static IP — CoreDNS runs on all nodes in the cluster.

## How to help

See [HACKING.md](HACKING.md) for development topics.
