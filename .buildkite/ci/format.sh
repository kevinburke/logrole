#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(
  CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd
)"
readonly SCRIPT_DIR

# shellcheck source=.buildkite/ci/setup-env.sh
source "${SCRIPT_DIR}/setup-env.sh"

install_tool differ github.com/kevinburke/differ "${DIFFER_VERSION:?DIFFER_VERSION is required}"
install_tool goimports golang.org/x/tools/cmd/goimports latest

mkdir -p reports/format

run_logged reports/format/go-mod-tidy.txt differ go mod tidy
run_logged reports/format/go-fix.txt differ go fix ./...
run_logged reports/format/go-fmt.txt differ go fmt ./...
run_logged reports/format/goimports.txt differ .buildkite/ci/goimports.sh
