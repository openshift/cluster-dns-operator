# OpenShift Networking Context: DNS Operator & CoreDNS

This document provides a high-level overview of how the `cluster-dns-operator` and its operand, `CoreDNS`, fit into the broader OpenShift Networking architecture. This context is critical for understanding the impact of changes made within this repository.

## The Big Picture: Operator vs. Operand vs. CNI

In OpenShift, network connectivity and service discovery are handled by a layered architecture. CoreDNS and the Cluster DNS Operator are distinct components with separate Git repositories, but they work together as a single integrated DNS system:

*   **OVN-Kubernetes (The CNI / The Wires):** The default Container Network Interface (CNI). It uses Open Virtual Network (OVN) and Open vSwitch (OVS) to create the software-defined overlay network, providing routing, switching, and encapsulation.
*   **Cluster DNS Operator (The Manager):** This repository (`openshift/cluster-dns-operator`). It lives in the `openshift-dns-operator` namespace as a Deployment. It is the control plane that automates the deployment, configuration, and lifecycle of the DNS infrastructure.
*   **CoreDNS (The Directory / The Operand):** The actual DNS server implementation (`openshift/coredns`). The operator deploys it as a DaemonSet (`dns-default`) in the `openshift-dns` namespace to ensure every node has a local resolver. OVN-Kubernetes routes DNS queries from pods to these local CoreDNS instances.

*Note: The owning team for both DNS components is Network Ingress and DNS.*
