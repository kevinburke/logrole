#!/usr/bin/env bash
# Verify the generated browser bundle + go-bindata are in sync with their
# inputs. The bundle (static/js/twilio-voice-sdk.js) is produced by
# esbuild from js/twilio-voice-sdk.js + node_modules/@twilio/voice-sdk;
# the bindata is produced by go-bindata from everything under
# templates/ and static/.
#
# This step runs `make assets`, which rebuilds both, and uses `differ` to
# fail if any tracked file changed. That catches:
#   - JS source changed but `npm run build` not re-run
#   - @twilio/voice-sdk version bumped in package.json but bundle not
#     rebuilt
#   - Bundle (or any template) changed but go-bindata not re-run
#
# Requires `node` and `npm` to be installed on the agent. The
# `caracal` agent already has them; if you move this step to a host
# without Node, install Node 22+ first.
set -euo pipefail

SCRIPT_DIR="$(
  CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd
)"
readonly SCRIPT_DIR

# shellcheck source=.buildkite/ci/setup-env.sh
source "${SCRIPT_DIR}/setup-env.sh"

install_tool differ github.com/kevinburke/differ "${DIFFER_VERSION:?DIFFER_VERSION is required}"
install_tool go-bindata github.com/kevinburke/go-bindata/v4/go-bindata latest

# Fail loudly with an actionable message rather than letting `make`
# complain about a missing program inside a recipe.
if ! command -v npm >/dev/null 2>&1; then
  echo "npm is required but was not found on PATH." >&2
  echo "Install Node 22+ on the build agent (the bundle is produced via esbuild)." >&2
  exit 1
fi

mkdir -p reports/assets

run_logged reports/assets/make-assets.txt differ make assets
