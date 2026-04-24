# Controller Patterns in cluster-dns-operator

The business logic resides primarily in `pkg/operator/controller/`. The operator is built on `controller-runtime` and follows the standard operator pattern.

## The Custom Resource: The Single Source of Truth

Everything starts with a single Custom Resource:
```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
```
This CR is the single source of truth for how DNS should behave in the cluster. Administrators configure upstream resolvers, per-zone forwarding rules, cache TTLs, node placement, and more by editing this CR. The operator watches it and reconciles the cluster state to match.

## Operator Reconciliation: From CR to Corefile

### Key controllers:
*   `controller.go`: Main reconciler; sets up watches on DNS CR, DaemonSets, Services, and ConfigMaps.
*   `controller_dns_configmap.go`: Generates the Corefile from a Go template and stores it in a ConfigMap.
*   `controller_dns_daemonset.go`: Manages the CoreDNS DaemonSet.
*   `controller_dns_service.go`: Manages the `dns-default` Kubernetes Service.
*   `controller_dns_node_resolver_daemonset.go`: Manages the `node-resolver` DaemonSet.
*   `dns_status.go`: Updates the ClusterOperator status conditions.

### Data flow:
1. The `dns-controller` detects a change to the `dns.operator.openshift.io/default` CR.
2. The Reconciler calls `ensureDNSConfigMap` → `desiredDNSConfigMap` → `desiredCorefile`.
3. `desiredCorefile` populates a Go `text/template` with values from the CR's spec.
4. The rendered Corefile string is placed into the `Data` field of a ConfigMap named `dns-default` in the `openshift-dns` namespace.
5. The operator creates or updates this ConfigMap via the API.
6. CoreDNS pods mount this ConfigMap, and the `reload` plugin detects the change, applying it without requiring a pod restart.

## Key Concepts
- **ManagementState**: OpenShift operators support a `ManagementState` field.
  - `Managed`: The operator actively reconciles and overwrites manual changes.
  - `Unmanaged`: The operator pauses reconciliation, allowing manual intervention or debugging.
  - `Removed`: The operator tears down the managed components.
- **Idempotency**: All `ensure*` functions (e.g., `ensureDNSDaemonSet`) MUST be idempotent. They must compare the existing state with the desired state and apply changes only if there is a diff.
