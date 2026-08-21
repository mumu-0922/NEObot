#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd "${script_dir}/.." && pwd -P)"

if (( EUID == 0 )); then
  echo "test-agent-host: skipped because the Runner intentionally refuses root" >&2
  exit 0
fi

temporary="$(mktemp -d)"
state_dir="${temporary}/state"
secret_dir="${temporary}/secrets"
cleanup() {
  env \
    AGENT_HOST_STATE_DIR="${state_dir}" \
    AGENT_HOST_TOKEN_FILE="${secret_dir}/token" \
    AGENT_HOST_RUNNER_ID_FILE="${secret_dir}/runner-id" \
    "${script_dir}/agent-host.sh" stop >/dev/null 2>&1 || true
  rm -rf -- "${temporary}"
}
trap cleanup EXIT

run_host() {
  env \
    AGENT_HOST_STATE_DIR="${state_dir}" \
    AGENT_HOST_TOKEN_FILE="${secret_dir}/token" \
    AGENT_HOST_RUNNER_ID_FILE="${secret_dir}/runner-id" \
    "${script_dir}/agent-host.sh" "$@"
}

run_host prepare
run_host status
run_host start
pid_before="$(<"${state_dir}/agent-host.pid")"
run_host prepare
[[ "$(<"${state_dir}/agent-host.pid")" == "${pid_before}" ]]

runner_id_before="$(<"${secret_dir}/runner-id")"
token_before="$(<"${secret_dir}/token")"
run_host restart
[[ "$(<"${secret_dir}/runner-id")" == "${runner_id_before}" ]]
[[ "$(<"${secret_dir}/token")" == "${token_before}" ]]
run_host status
run_host stop
if run_host status >/dev/null 2>&1; then
  echo "test-agent-host: status remained healthy after stop" >&2
  exit 1
fi

echo "test-agent-host: lifecycle passed"
