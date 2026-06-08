# Cluster DNS Operator Architecture

The OpenShift Cluster DNS Operator manages the DNS lifecycle within an OpenShift cluster. 

## Core Components

1. **DNS Operator**: Runs in the `openshift-dns-operator` namespace. It watches the `dns.operator.openshift.io/default` custom resource (CR).
2. **CoreDNS (Operand)**: The Operator deploys CoreDNS as a **DaemonSet** (`dns-default`) in the `openshift-dns` namespace. This ensures a local CoreDNS pod runs on every cluster node for maximum scalability and low-latency local resolution.
3. **ClusterIP Service**: A service (typically `dns-default`) provides a stable ClusterIP (e.g., `172.30.0.10`). The Kubelet configures pods to use this IP for resolution.
4. **ConfigMap (Corefile)**: The DNS Operator manages the `Corefile` configuration via a ConfigMap, which includes plugins for kubernetes service discovery, external forwarding, metrics, and caching.

## How Pods Resolve DNS

When a new Pod is created, the Kubelet automatically injects DNS configuration into the Pod's `/etc/resolv.conf`:
1.  **Pod configuration:** The primary nameserver points to the `dns-default` service ClusterIP (typically `172.30.0.10`). The default `dnsPolicy` is `ClusterFirst`.
2.  **Search domains:** Includes domains like `<namespace>.svc.cluster.local`, `svc.cluster.local`, and `cluster.local`, enabling short-name resolution (e.g., `my-service` resolves to `my-service.my-namespace.svc.cluster.local`).
3.  **Routing optimization:** Because CoreDNS runs as a DaemonSet, OVN-Kubernetes preferentially routes DNS queries to the local CoreDNS pod on the same node, minimizing latency.
4.  **Resolution Flow:**
    - **Internal:** CoreDNS uses the `kubernetes` plugin to resolve cluster-internal names (`*.cluster.local`).
    - **External:** Queries outside the cluster domain are handled by the `forward` plugin, relying on the upstream nameservers defined in the node's `/etc/resolv.conf`.

## Advanced Integrations

*   **Node Resolver DaemonSet:** Managed by the operator, this DaemonSet runs on every node to look up the ClusterIP of the internal image registry (`image-registry.openshift-image-registry.svc`) and writes it into the node's `/etc/hosts` file. This resolves the bootstrapping chicken-and-egg problem where the node's container runtime needs to pull images before cluster DNS is fully operational.
*   **On-Premise Platforms (MCO Layer):** On bare metal, vSphere, and OpenStack, there's an additional CoreDNS instance managed by the Machine Config Operator (MCO) running as static pods on nodes. These handle node-level DNS (e.g., resolving `api-int` records) and delegate cluster-related lookups to the main CoreDNS service in `openshift-dns`.

## Key Resources Summary

| Namespace | Resource | Purpose |
| :--- | :--- | :--- |
| `openshift-dns-operator` | Deployment/dns-operator | The Cluster DNS Operator |
| `openshift-dns` | DaemonSet/dns-default | CoreDNS pods (one per node) |
| `openshift-dns` | Service/dns-default | Stable ClusterIP for DNS |
| `openshift-dns` | ConfigMap/dns-default | The generated Corefile |
| `openshift-dns` | DaemonSet/node-resolver | `/etc/hosts` updater for nodes |
| `(Cluster Scoped)` | CR `dns.operator.openshift.io/default` | The DNS configuration CR |
