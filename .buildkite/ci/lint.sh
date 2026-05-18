#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(
  CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd
)"
readonly SCRIPT_DIR

# shellcheck source=.buildkite/ci/setup-env.sh
source "${SCRIPT_DIR}/setup-env.sh"

install_tool staticcheck honnef.co/go/tools/cmd/staticcheck "${STATICCHECK_VERSION:?STATICCHECK_VERSION is required}"

mkdir -p reports/lint

run_logged reports/lint/go-vet.txt go vet ./...
run_logged reports/lint/staticcheck.txt staticcheck ./...
