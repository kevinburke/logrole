#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(
  CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd
)"
readonly SCRIPT_DIR

# shellcheck source=.buildkite/ci/setup-env.sh
source "${SCRIPT_DIR}/setup-env.sh"

mkdir -p tmp/dist reports/build

run_logged reports/build/go-build.txt \
  go build -buildvcs=true -o tmp/dist/ ./commands/logrole_server ./commands/logrole_write_config_from_env
