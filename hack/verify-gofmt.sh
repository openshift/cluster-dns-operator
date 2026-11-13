#!/bin/bash

set -euo pipefail

bad_files=$(
    find . -type d \( -name vendor -o -name _output \) -prune -o \
        -type f -name '*.go' -print0 |
        xargs -0 -r -n 100 "${GOFMT:-gofmt}" -s -l
)
if [[ -n "${bad_files}" ]]; then
    (>&2 echo "!!! gofmt needs to be run on the listed files")
	echo "${bad_files}"
    (>&2 echo "Try running 'gofmt -s -d [path]' or autocorrect with 'hack/verify-gofmt.sh | xargs -n 1 gofmt -s -w'")
    exit 1
fi
