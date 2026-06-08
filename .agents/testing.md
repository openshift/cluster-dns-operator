# Testing Instructions

Rigorous testing is required for all changes in the `cluster-dns-operator`.

## Unit Testing
- Unit tests are co-located with their respective Go files (e.g., `controller_dns_daemonset_test.go`).
- Run unit tests locally using standard Go tools or the Makefile:
  ```bash
  make test
  # or
  go test ./pkg/operator/controller/...
  ```
- Focus on testing the generation of desired resources (e.g., ensuring `ensureDNSDaemonSet` produces the exact expected YAML given various cluster state inputs).

## End-to-End (E2E) Testing
- E2E tests are located in the `test/` directory (e.g., `e2e.test`).
- These tests expect a live OpenShift cluster.
- E2E tests are automatically executed by Prow on every Pull Request.
- **Consistency**: Fit any new tests into existing test libraries and formats rather than introducing new testing frameworks or paradigms.
- **E2E Necessity**: Read the existing E2E tests to understand the complexity and scope of things currently tested with the suite. Use this context to determine what type of new feature might rise to the level of needing a new E2E test versus a unit test.
