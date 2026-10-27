BINARY := bin/cluster-dns-operator-tests-ext

# -trimpath is required, not cosmetic. OTE records each spec's Ginkgo CodeLocations in
# .openshift-tests-extension/<component>.json, and those come from runtime.Caller, which
# without -trimpath bakes the absolute path of whichever checkout built the binary into
# the committed metadata. `make verify` reruns `update` and diffs the result, so any other
# checkout - including CI - would see a non-empty diff and fail even with no test change.
# -trimpath rewrites them to module-relative paths, making the metadata reproducible.
# Payload test binaries use static linking and the ART test-binary compliance exemption.
.PHONY: build
build:
	@echo "Building extension binary..."
	@mkdir -p bin
	CGO_ENABLED=0 GO_COMPLIANCE_POLICY=exempt_all GOTOOLCHAIN=auto GOSUMDB=sum.golang.org go build -trimpath -o $(BINARY) ./cmd
	@echo "✅ Binary built: $(BINARY)"

# A KUBECONFIG must be set, but is never read. The vendored
# origin/test/extended/util/compat_otp package has an init() that prints
# "Please set KUBECONFIG first!" and calls os.Exit(0) when KUBECONFIG is empty - exit code
# zero, so an unguarded `make verify` would report success while regenerating nothing.
# update only walks the Ginkgo spec tree and contacts no cluster, so point it at /dev/null
# to keep these targets hermetic and runnable in a CI job that has no cluster.
OTE_KUBECONFIG := /dev/null

# update regenerates .openshift-tests-extension/<component>.json. OTE uses it to verify
# that renamed or deleted tests are explicitly handled; run it whenever a test name changes
# and commit the result. Renames must also be reflected in openshift-eng/ci-test-mapping.
.PHONY: update
update: build
	KUBECONFIG=$(OTE_KUBECONFIG) $(BINARY) update

.PHONY: verify
verify: update
	@echo "Verifying extension metadata is up to date..."
	git diff --exit-code .openshift-tests-extension || \
		(echo "Extension metadata is out of date, run 'make update' and commit the result" && exit 1)
	@echo "Extension metadata is up to date"

.PHONY: clean
clean:
	@rm -f $(BINARY)
	@cd test/qe && $(MAKE) -f bindata.mk clean-bindata

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build  - Build extension binary"
	@echo "  update - Regenerate .openshift-tests-extension metadata"
	@echo "  verify - Fail if the extension metadata is out of date"
	@echo "  clean  - Remove binaries and bindata"
