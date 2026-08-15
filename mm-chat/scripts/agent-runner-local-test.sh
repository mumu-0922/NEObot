#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
command_name="${1:-}"
if [[ "${EUID}" -ne 0 && "${NEO_AGENT_LOCAL_TEST_DELEGATED:-0}" != 1 && \
  "${command_name}" != bootstrap-system && "${command_name}" != rollback-system && \
  "$(cat /proc/1/comm 2>/dev/null || true)" == systemd && \
  -x /usr/bin/systemd-run ]]; then
  exec /usr/bin/systemd-run --user --scope --quiet --collect \
    --property=Delegate=yes --setenv=NEO_AGENT_LOCAL_TEST_DELEGATED=1 \
    /usr/bin/python3 "${script_dir}/manage-agent-runner-local-test.py" "$@"
fi
exec /usr/bin/python3 "${script_dir}/manage-agent-runner-local-test.py" "$@"
