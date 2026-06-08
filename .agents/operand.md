# Operand Reference

The `cluster-dns-operator` is the *manager*. The actual DNS service it manages is the **operand**.

## CoreDNS
- The operand for this operator is **CoreDNS**.
- The operator does not contain the CoreDNS source code. 
- The source code for the CoreDNS implementation customized for OpenShift is located in the sibling repository:
  **`https://github.com/openshift/coredns`**
- If you need to understand how DNS resolution actually behaves internally (e.g., plugins, caching logic), refer to the codebase in the `https://github.com/openshift/coredns` path. The operator's job is solely to deploy and configure it via the `Corefile` ConfigMap.

## CoreDNS Plugin Chain

The generated Corefile enables a specific set of plugins. OpenShift's default plugin chain includes:
*   `kubernetes`: Resolves `*.svc.cluster.local` and pod DNS using the Kubernetes API.
*   `forward`: Forwards external queries to upstream resolvers (defaults to the node's `/etc/resolv.conf`).
*   `cache`: Caches responses (configurable max TTLs).
*   `reload`: Hot-reloads the Corefile on ConfigMap changes.
*   `prometheus`: Exposes metrics at `:9153`.
*   `health` / `ready`: Health check endpoints for liveness/readiness probes.
*   `errors`: Error logging.
*   `bufsize`: Controls EDNS0 buffer size.
*   `ocp_dnsnameresolver`: Integrates with OVN-Kubernetes for Egress Firewall DNS-based rules, allowing the network policy engine to resolve DNS names for fine-grained egress traffic control.
