#!/bin/bash

set -euo pipefail

formatter="${GOFMT:-gofmt}"
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/verify-gofmt.XXXXXX")
trap 'rm -rf "$temporary_directory"' EXIT
trap 'exit 1' HUP INT TERM

if ! find . -type d \( -name vendor -o -name _output \) -prune -o -type f -name '*.go' -print0 > "$temporary_directory/files"; then
    echo "Unable to select Go files for formatting verification" >&2
    exit 1
fi

if [[ ! -s "$temporary_directory/files" ]]; then
    exit 0
fi

if ! xargs -0 -n 100 "$formatter" -s -l < "$temporary_directory/files" > "$temporary_directory/gofmt.out"; then
    echo "Unable to complete Go formatting verification" >&2
    exit 1
fi

if [[ -s "$temporary_directory/gofmt.out" ]]; then
    echo "!!! gofmt needs to be run on the listed files" >&2
    cat "$temporary_directory/gofmt.out"
    echo "Inspect each file with 'gofmt -s -d' and use quoted paths when formatting it" >&2
    exit 1
fi
