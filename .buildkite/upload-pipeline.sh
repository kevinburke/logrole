#!/usr/bin/env bash
set -euo pipefail

export BUILDKITE_AGENT_PIPELINE_UPLOAD_REJECT_SECRETS=1

buildkite-agent pipeline upload .buildkite/pipeline.yml
