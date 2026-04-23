# OpenShift Go Guidelines & CI/CD Caveats

When contributing to this repository, strictly adhere to these OpenShift ecosystem rules.

## Go Coding Guidelines
- **Idempotency is King**: Reconciliation loops MUST be idempotent. Do not blindly recreate resources; always fetch, compare, and update only if a diff exists.
- **Never Hardcode**: Do not hardcode namespaces, secrets, or resource names unless defined as strict API constants. Watch namespaces must be configurable.
- **Structural Schemas**: All Custom Resource Definitions (CRDs) must use OpenAPI v3 validation to strictly validate user input.
- **Principle of Least Privilege**: The operator must run with minimal RBAC permissions using dedicated ServiceAccounts. Do not add broad permissions.

## CI/CD and Prow Caveats
OpenShift uses **Prow** for CI/CD automation.

- **Prow Jobs are Auto-Generated**: Do not manually craft Prow job YAMLs. Prow job definitions are generated via `ci-operator-prowgen` from configurations stored centrally in the `openshift/release` repository (`ci-operator/config/openshift/cluster-dns-operator/`).
- **Changing CI Configs**: If a new e2e test or build step is required, you must submit a PR to the `openshift/release` repository, not this one.
- **Image Security**: Pipeline configurations implicitly enforce image scanning and signing. Ensure all base images and operands are appropriately tracked.
- **Local Testing First**: Prow CI runs take time. Always validate changes locally (`make test`) before pushing PRs to trigger Prow jobs.
