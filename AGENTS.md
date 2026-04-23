# Cluster DNS Operator - AI Agent Directory

You are an AI assistant working on the `cluster-dns-operator` repository. This repository is responsible for deploying and managing CoreDNS across OpenShift clusters.

**IMPORTANT:** We use a progressive disclosure architecture for instructions to preserve your context window. Do not attempt to guess architectural patterns or OpenShift conventions.

Depending on the task you are assigned, **read the corresponding file** from the `.agents/` directory before proposing changes:

- **[.agents/context.md](.agents/context.md)**: **ALWAYS READ THIS FIRST** to understand the high-level relationship between the Operator, the CoreDNS Operand, and the OpenShift CNI.
- **[.agents/architecture.md](.agents/architecture.md)**: Read this if you are making changes to how CoreDNS is deployed, or if you need to understand DNS resolution routing, Node Resolvers, and Key Resources.
- **[.agents/controllers.md](.agents/controllers.md)**: Read this if you are modifying Go code in `pkg/operator/controller/`, adjusting reconciliation logic, or handling the `dns.operator.openshift.io` API.
- **[.agents/rules.md](.agents/rules.md)**: Read this to understand strict OpenShift Go coding guidelines, Prow CI/CD caveats, and Operator SDK conventions.
- **[.agents/testing.md](.agents/testing.md)**: Read this before writing or running unit tests or end-to-end tests.
- **[.agents/operand.md](.agents/operand.md)**: Read this if you need to understand the CoreDNS operand source code location or its specific plugin chain.
