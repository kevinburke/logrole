#!/usr/bin/env bash
set -euo pipefail

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  echo "source .buildkite/ci/setup-env.sh from another script" >&2
  exit 1
fi

SETUP_ENV_SCRIPT_DIR="$(
  CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd
)"
readonly SETUP_ENV_SCRIPT_DIR
SETUP_ENV_REPO_ROOT="$(
  CDPATH='' cd -- "${SETUP_ENV_SCRIPT_DIR}/../.." && pwd
)"
readonly SETUP_ENV_REPO_ROOT

if [[ -z "${BUILDKITE_CI_ENV_READY:-}" ]]; then
  export BUILDKITE_CI_ENV_READY=1

  cd "${SETUP_ENV_REPO_ROOT}"

  export TZ="${TZ:-UTC}"
  export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"
  export DIFFER_VERSION="${DIFFER_VERSION:-v0.0.0-20260403230520-c0574ebcacb2}"
  export STATICCHECK_VERSION="${STATICCHECK_VERSION:-master}"

  readonly DEFAULT_CI_CACHE_ROOT="${XDG_CACHE_HOME:-${HOME}/.cache}/logrole-buildkite"
  export BUILDKITE_CI_CACHE_ROOT="${BUILDKITE_CI_CACHE_ROOT:-${DEFAULT_CI_CACHE_ROOT}}"

  export GOCACHE="${GOCACHE:-${BUILDKITE_CI_CACHE_ROOT}/go-build}"
  export GOMODCACHE="${GOMODCACHE:-${BUILDKITE_CI_CACHE_ROOT}/go-mod}"
  export GOTMPDIR="${GOTMPDIR:-${BUILDKITE_CI_CACHE_ROOT}/go-tmp}"

  goflags="${GOFLAGS:-}"
  if [[ -n "${goflags}" ]]; then
    goflags+=" "
  fi
  goflags+="-trimpath"
  export GOFLAGS="${goflags}"

  export TOOL_CACHE_ROOT="${TOOL_CACHE_ROOT:-${BUILDKITE_CI_CACHE_ROOT}/tools}"
  export GOBIN="${TOOL_CACHE_ROOT}/bin"
  readonly TOOL_STAMP_DIR="${TOOL_CACHE_ROOT}/stamps"

  mkdir -p "${GOCACHE}" "${GOMODCACHE}" "${GOTMPDIR}" "${GOBIN}" "${TOOL_STAMP_DIR}"
  export PATH="${GOBIN}:${PATH}"

  GO_VERSION="$(
    go env GOVERSION
  )"
  echo "Using ${GO_VERSION}"
fi

install_tool() {
  local binary="$1"
  local pkg="$2"
  local version="$3"
  local stamp_file="${TOOL_STAMP_DIR}/${binary}-${version}"

  if [[ -x "${GOBIN}/${binary}" && -f "${stamp_file}" ]]; then
    return
  fi

  find "${TOOL_STAMP_DIR}" -maxdepth 1 -type f -name "${binary}-*" -delete
  GOFLAGS="" GOBIN="${GOBIN}" go install "${pkg}@${version}"
  : > "${stamp_file}"
}

run_logged() {
  local logfile="$1"
  shift

  set +e
  "$@" >"${logfile}" 2>&1
  local status=$?
  set -e

  cat "${logfile}"
  return "${status}"
}

run_goimports_repo() {
  local -a gofiles=()

  while IFS= read -r file; do
    gofiles+=("${file}")
  done < <(git ls-files -- '*.go')

  if [[ "${#gofiles[@]}" -eq 0 ]]; then
    return 0
  fi

  goimports -w "${gofiles[@]}"
}
