#!/bin/bash
# Verifies that the CoreDNS module vendored into the operator, whose
# template plugin validates rendered response rule stanzas, matches the
# CoreDNS version shipped in the CoreDNS image for the same release.
#
# COREDNS_BRANCH selects the openshift/coredns branch that builds the
# CoreDNS image for this release (default: main).
set -euo pipefail

COREDNS_BRANCH="${COREDNS_BRANCH:-main}"
VERSION_URL="https://raw.githubusercontent.com/openshift/coredns/${COREDNS_BRANCH}/coremain/version.go"

vendored="$(go list -m -f '{{.Version}}' github.com/coredns/coredns)"
vendored="${vendored#v}"

shipped="$(curl -fsSL "${VERSION_URL}" | sed -n 's/^[[:space:]]*CoreVersion[[:space:]]*=[[:space:]]*"\(.*\)"/\1/p')"
if [ -z "${shipped}" ]; then
  echo "could not read CoreVersion from ${VERSION_URL}"
  exit 1
fi

if [ "${vendored}" != "${shipped}" ]; then
  echo "vendored github.com/coredns/coredns is ${vendored}, but openshift/coredns ${COREDNS_BRANCH} ships ${shipped}."
  echo "Update the vendored module so response rule validation matches the template plugin CoreDNS runs."
  exit 1
fi
echo "verified vendored CoreDNS ${vendored} matches openshift/coredns ${COREDNS_BRANCH}"
