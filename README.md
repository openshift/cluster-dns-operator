# The DNS Operator

The DNS Operator deploys and manages [CoreDNS](https://coredns.io) to provide a name resolution service to pods that enables [DNS-based Kubernetes Service discovery](https://kubernetes.io/docs/concepts/services-networking/service/#dns) in [OpenShift](https://openshift.io).

The operator tries to be useful out of the box by creating a working default deployment based on the cluster's configuration.

* The default cluster domain is `cluster.local`.
* Limited configuration of the CoreDNS [Corefile](https://coredns.io/manual/toc/#configuration) or [kubernetes plugin](https://coredns.io/plugins/kubernetes/) is supported.

## How it works

The DNS Operator deploys CoreDNS using a DaemonSet, which means that each node has a local CoreDNS pod replica.  This topology provides scalability as the cluster grows or shrinks and resilience in case a node becomes temporarily isolated from other nodes.

The DaemonSet's pod template specifies a "dns" container with CoreDNS and a "dns-node-resolver" container with a process that adds the cluster image registry service's DNS name to the host node's `/etc/hosts` file (see below).

In order to resolve cluster service DNS names, the operator configures CoreDNS with the [kubernetes plugin](https://coredns.io/plugins/kubernetes/).  This plugin resolves DNS names of the form `<service name>.<namespace>.svc.cluster.local` to the identified Service's corresponding endpoints.

In order to resolve external DNS names, the operator configures CoreDNS to forward to the upstream name servers configured in the node host's `/etc/resolv.conf` (typically these name servers come from DHCP or are injected into a custom VM image).  The operator allows the user to configure additional upstreams to use for specific zones; see <https://github.com/openshift/enhancements/blob/master/enhancements/dns/plugins.md>.

The operator also creates a Service with a fixed IP address.  This address is derived from the service network CIDR, namely by taking the tenth address in the address space.  For example, if the service network CIDR is 172.30.0.0/16, then the DNS service's address is 172.30.0.10.

When a pod is created, the kubelet injects a `nameserver` entry with the DNS service's IP address into the pod's `/etc/resolv.conf` file (unless the pod overrides the default behavior with `spec.dnsPolicy`; see [DNS for Services and Pods: Pod's DNS Policy](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#pod-s-dns-policy)).

Within a pod, the flow of a DNS query varies depending on whether the DNS name to be resolved is for a cluster service DNS name or for an external DNS name.  A query for a cluster service DNS name flows from the pod process via the service proxy to a randomly chosen CoreDNS instance, which itself resolves the name.  A query for an external DNS name flows from the pod process via the service proxy to a CoreDNS instance, which forwards the request to an upstream name server; this name server may be on a network that is external to the cluster, possibly the Internet.

The foregoing describes the behavior for pods that use container networking.  If a pod is configured to use the host network, or if a process runs directly on a node, it uses the name servers configured in the host node's `/etc/resolv.conf` file.  This means queries from host-network pods or processes flow from the process to the name server that is specified in `/etc/resolv.conf` (which typically is on an external network or the Internet).

In general, DNS names for Services will not resolve from the node host as the node itself is not configured to use CoreDNS as its name server.  For example, the container runtime runs directly on the node host, so it cannot resolve cluster service DNS names, with the following exception.  As a special case, a process in the DNS DaemonSet's "dns-node-resolver" container adds the registry service's DNS name, `image-registry.openshift-image-registry.svc`, to the node's `/etc/hosts` file so that the container runtime and kubelet can resolve the registry service's DNS name.

Troubleshooting DNS issues can may require tools such as strace, tcpdump, dropwatch, and other low-level network diagnostics tools.


## How to help

See [HACKING.md](HACKING.md) for development topics.

## Reporting issues

Bugs are tracked in [the Red Hat Issue Tracker](https://issues.redhat.com/secure/CreateIssueDetails!init.jspa?pid=12332330&issuetype=1&components=12367613&priority=10300&customfield_12316142=26752).
