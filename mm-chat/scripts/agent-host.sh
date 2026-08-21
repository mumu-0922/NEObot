#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: agent-host.sh <build|start|prepare|status|stop|restart>

Build and manage the ordinary-user WSL Agent Host without systemd.

Commands:
  build    Build cmd/agent-host into the private runtime directory.
  start    Start the built Runner, or reuse it when its identity is healthy.
  prepare  Build and then start the Runner (deployment-wrapper entrypoint).
  status   Verify the Unix-socket protocol and pinned Runner identity.
  stop     Stop only the exact PID started from this Runner binary path.
  restart  Stop, build, and start.

Optional test/operator overrides:
  AGENT_HOST_STATE_DIR
  AGENT_HOST_TOKEN_FILE
  AGENT_HOST_RUNNER_ID_FILE
  AGENT_HOST_SOCKET
  AGENT_HOST_PID_FILE
  AGENT_HOST_LOG_FILE
  AGENT_HOST_BINARY
  AGENT_HOST_VERSION_FILE
  AGENT_HOST_SKILLS_ROOT
USAGE
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"

state_dir="${AGENT_HOST_STATE_DIR:-${project_dir}/.runtime/agent-host}"
token_file="${AGENT_HOST_TOKEN_FILE:-${project_dir}/secrets/agent-host-token}"
runner_id_file="${AGENT_HOST_RUNNER_ID_FILE:-${project_dir}/secrets/agent-host-runner-id}"
socket_path="${AGENT_HOST_SOCKET:-${state_dir}/agent-host.sock}"
pid_file="${AGENT_HOST_PID_FILE:-${state_dir}/agent-host.pid}"
log_file="${AGENT_HOST_LOG_FILE:-${state_dir}/agent-host.log}"
binary="${AGENT_HOST_BINARY:-${state_dir}/agent-host}"
version_file="${AGENT_HOST_VERSION_FILE:-${state_dir}/agent-host.version}"
skills_root="${AGENT_HOST_SKILLS_ROOT:-${project_dir}/data/agent-skills}"

command="${1:-}"
if [[ -z "${command}" || $# -ne 1 ]]; then
  usage >&2
  exit 2
fi
if [[ "${command}" == "-h" || "${command}" == "--help" ]]; then
  usage
  exit 0
fi
case "${command}" in
  build|start|prepare|status|stop|restart) ;;
  *) usage >&2; exit 2 ;;
esac

if (( EUID == 0 )); then
  echo "agent-host: refusing root execution" >&2
  exit 1
fi

umask 077

fail() {
  echo "agent-host: $*" >&2
  exit 1
}

require_absolute_path() {
  local label="$1"
  local path="$2"
  [[ "${path}" == /* && "${path}" != *$'\n'* && "${path}" != *$'\r'* ]] ||
    fail "${label} must be an absolute path without control characters"
}

for pair in \
  "state directory:${state_dir}" \
  "token file:${token_file}" \
  "Runner id file:${runner_id_file}" \
  "socket:${socket_path}" \
  "PID file:${pid_file}" \
  "log file:${log_file}" \
  "binary:${binary}" \
  "version file:${version_file}"; do
  require_absolute_path "${pair%%:*}" "${pair#*:}"
done
require_absolute_path "Skills root" "${skills_root}"
[[ -d "${skills_root}" && ! -L "${skills_root}" ]] || fail "Skills root must be an existing directory"
[[ "$(readlink -f -- "${skills_root}")" == "${skills_root}" ]] || fail "Skills root contains a symlink component"

ensure_private_directory() {
  local directory="$1"
  if [[ ! -e "${directory}" ]]; then
    mkdir -m 0700 -p "${directory}"
  fi
  [[ -d "${directory}" && ! -L "${directory}" ]] || fail "unsafe directory: ${directory}"
  [[ "$(readlink -f -- "${directory}")" == "${directory}" ]] ||
    fail "directory contains a symlink component: ${directory}"
  [[ "$(stat -c '%u' -- "${directory}")" == "${EUID}" ]] ||
    fail "directory is not owned by the current user: ${directory}"
  [[ "$(stat -c '%a' -- "${directory}")" == "700" ]] ||
    fail "directory mode must be 0700: ${directory}"
}

ensure_parent_directory() {
  local path="$1"
  local parent
  parent="$(dirname -- "${path}")"
  if [[ ! -e "${parent}" ]]; then
    mkdir -m 0700 -p "${parent}"
  fi
  [[ -d "${parent}" && ! -L "${parent}" ]] || fail "unsafe parent directory: ${parent}"
  [[ "$(readlink -f -- "${parent}")" == "${parent}" ]] ||
    fail "parent directory contains a symlink component: ${parent}"
  [[ "$(stat -c '%u' -- "${parent}")" == "${EUID}" ]] ||
    fail "parent directory is not owned by the current user: ${parent}"
  local mode
  mode="$(stat -c '%a' -- "${parent}")"
  (( (8#${mode} & 8#022) == 0 )) || fail "parent directory is group/other writable: ${parent}"
}

ensure_runtime_layout() {
  ensure_private_directory "${state_dir}"
  ensure_parent_directory "${token_file}"
  ensure_parent_directory "${runner_id_file}"
  [[ "$(dirname -- "${socket_path}")" == "${state_dir}" ]] ||
    fail "socket must remain directly under the private state directory"
  [[ "$(dirname -- "${pid_file}")" == "${state_dir}" ]] ||
    fail "PID file must remain directly under the private state directory"
  [[ "$(dirname -- "${log_file}")" == "${state_dir}" ]] ||
    fail "log file must remain directly under the private state directory"
  [[ "$(dirname -- "${binary}")" == "${state_dir}" ]] ||
    fail "binary must remain directly under the private state directory"
  [[ "$(dirname -- "${version_file}")" == "${state_dir}" ]] ||
    fail "version file must remain directly under the private state directory"
}

install_generated_file_once() {
  local destination="$1"
  local value="$2"
  local temporary
  temporary="$(mktemp "$(dirname -- "${destination}")/.agent-host-new.XXXXXX")"
  chmod 0600 "${temporary}"
  printf '%s\n' "${value}" >"${temporary}"
  if ! ln -- "${temporary}" "${destination}" 2>/dev/null; then
    rm -f -- "${temporary}"
    [[ -e "${destination}" ]] || fail "could not install ${destination}"
    return 0
  fi
  rm -f -- "${temporary}"
}

generate_hex() {
  local bytes="$1"
  od -An -N "${bytes}" -tx1 /dev/urandom | tr -d ' \n'
}

ensure_identity_files() {
  if [[ ! -e "${token_file}" ]]; then
    install_generated_file_once "${token_file}" "$(generate_hex 32)"
  fi
  if [[ ! -e "${runner_id_file}" ]]; then
    install_generated_file_once "${runner_id_file}" "wsl-$(generate_hex 12)"
  fi
  for file in "${token_file}" "${runner_id_file}"; do
    [[ -f "${file}" && ! -L "${file}" ]] || fail "identity path must be a regular non-symlink file: ${file}"
    [[ "$(readlink -f -- "${file}")" == "${file}" ]] || fail "identity path contains a symlink component: ${file}"
    [[ "$(stat -c '%u' -- "${file}")" == "${EUID}" ]] || fail "identity file owner mismatch: ${file}"
    [[ "$(stat -c '%a' -- "${file}")" == "600" ]] || fail "identity file mode must be 0600: ${file}"
  done
  local token runner_id
  token="$(<"${token_file}")"
  runner_id="$(<"${runner_id_file}")"
  (( ${#token} >= 32 && ${#token} <= 4096 )) || fail "token length is invalid"
  [[ "${token}" != *$'\n'* && "${token}" != *$'\r'* ]] || fail "token contains a line break"
  [[ "${runner_id}" =~ ^[a-z][a-z0-9-]{2,63}$ ]] || fail "Runner id is invalid"
}

read_pid() {
  local value
  [[ -f "${pid_file}" && ! -L "${pid_file}" ]] || return 1
  [[ "$(stat -c '%u' -- "${pid_file}")" == "${EUID}" ]] || return 1
  [[ "$(stat -c '%a' -- "${pid_file}")" == "600" ]] || return 1
  IFS= read -r value <"${pid_file}" || return 1
  [[ "${value}" =~ ^[1-9][0-9]*$ ]] || return 1
  printf '%s' "${value}"
}

process_is_managed() {
  local pid="$1"
  local first_arg state
  [[ -r "/proc/${pid}/cmdline" ]] || return 1
  IFS= read -r -d '' first_arg <"/proc/${pid}/cmdline" || true
  [[ "${first_arg}" == "${binary}" ]] || return 1
  state="$(awk '{print $3}' "/proc/${pid}/stat" 2>/dev/null || true)"
  [[ "${state}" != "Z" ]]
}

remove_pid_if_same() {
  local expected="$1"
  local current
  current="$(read_pid 2>/dev/null || true)"
  if [[ "${current}" == "${expected}" ]]; then
    rm -f -- "${pid_file}"
  fi
}

health_probe() {
  local expected_version="${1:-}"
  local token runner_id
  token="$(<"${token_file}")"
  runner_id="$(<"${runner_id_file}")"
  python3 - "${socket_path}" "${runner_id}" "${expected_version}" 3<<<"${token}" <<'PY'
import json
import socket
import sys

socket_path, expected_runner, expected_version = sys.argv[1:]
token = open(3, encoding="utf-8").read().strip()
request = (
    "GET /internal/v1/capabilities HTTP/1.1\r\n"
    "Host: agent-host.internal\r\n"
    f"Authorization: Bearer {token}\r\n"
    "Accept: application/json\r\n"
    "Connection: close\r\n\r\n"
).encode("utf-8")
client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
client.settimeout(2)
try:
    client.connect(socket_path)
    client.sendall(request)
    chunks = []
    size = 0
    while True:
        chunk = client.recv(8192)
        if not chunk:
            break
        size += len(chunk)
        if size > 65536:
            raise ValueError("response too large")
        chunks.append(chunk)
finally:
    client.close()
head, body = b"".join(chunks).split(b"\r\n\r\n", 1)
status = head.split(b"\r\n", 1)[0]
if status != b"HTTP/1.1 200 OK":
    raise SystemExit(1)
payload = json.loads(body)
if payload.get("protocolVersion") != 1 or payload.get("runnerId") != expected_runner:
    raise SystemExit(1)
if expected_version and payload.get("version") != expected_version:
    raise SystemExit(1)
PY
}

source_version() {
  (
    cd "${project_dir}"
    {
      find backend/cmd/agent-host backend/internal/agenthost -type f \
        \( -name '*.go' -o -name 'go.mod' -o -name 'go.sum' \) -print0
      printf '%s\0' backend/go.mod backend/go.sum
    } | sort -zu | xargs -0 sha256sum | sha256sum | awk '{print "source-" substr($1, 1, 24)}'
  )
}

build_runner() {
  ensure_runtime_layout
  command -v go >/dev/null 2>&1 || fail "Go is required to build the Agent Host"
  local temporary
  temporary="$(mktemp "${state_dir}/.agent-host-binary.XXXXXX")"
  trap 'rm -f -- "${temporary}"' RETURN
  (cd "${backend_dir}" && go build -trimpath -o "${temporary}" ./cmd/agent-host)
  chmod 0700 "${temporary}"
  mv -f -- "${temporary}" "${binary}"
  trap - RETURN
  local version temporary_version
  version="$(source_version)"
  temporary_version="$(mktemp "${state_dir}/.agent-host-version.XXXXXX")"
  printf '%s\n' "${version}" >"${temporary_version}"
  chmod 0600 "${temporary_version}"
  mv -f -- "${temporary_version}" "${version_file}"
  echo "agent-host: built ${binary}"
}

start_runner() {
  ensure_runtime_layout
  ensure_identity_files
  [[ -f "${binary}" && ! -L "${binary}" && -x "${binary}" ]] ||
    fail "Runner binary is missing; run '$0 build' first"
  [[ -f "${version_file}" && ! -L "${version_file}" ]] ||
    fail "Runner version file is missing; run '$0 build' first"
  command -v setsid >/dev/null 2>&1 || fail "setsid is required to detach the Agent Host"
  local version
  version="$(<"${version_file}")"
  [[ "${version}" =~ ^source-[0-9a-f]{24}$ ]] || fail "Runner version file is invalid"

  if health_probe "${version}" >/dev/null 2>&1; then
    echo "agent-host: healthy (Runner $(<"${runner_id_file}"))"
    return 0
  fi

  local old_pid
  old_pid="$(read_pid 2>/dev/null || true)"
  if [[ -n "${old_pid}" ]]; then
    if process_is_managed "${old_pid}"; then
      kill -TERM "${old_pid}"
      for _ in {1..100}; do
        kill -0 "${old_pid}" 2>/dev/null || break
        sleep 0.1
      done
      if kill -0 "${old_pid}" 2>/dev/null; then
        kill -KILL "${old_pid}"
      fi
    elif kill -0 "${old_pid}" 2>/dev/null; then
      fail "PID file points to an unrelated live process; refusing cleanup"
    fi
    remove_pid_if_same "${old_pid}"
  elif [[ -e "${pid_file}" ]]; then
    fail "PID file is unsafe or malformed; refusing cleanup"
  fi

  local pid temporary_pid
  nohup setsid env \
    AGENT_HOST_RUNNER_ID="$(<"${runner_id_file}")" \
    AGENT_HOST_SOCKET="${socket_path}" \
    AGENT_HOST_TOKEN_FILE="${token_file}" \
    AGENT_HOST_SKILLS_ROOT="${skills_root}" \
    MM_CHAT_VERSION="${version}" \
    "${binary}" >>"${log_file}" 2>&1 </dev/null &
  pid=$!
  temporary_pid="$(mktemp "${state_dir}/.agent-host-pid.XXXXXX")"
  printf '%s\n' "${pid}" >"${temporary_pid}"
  chmod 0600 "${temporary_pid}"
  mv -f -- "${temporary_pid}" "${pid_file}"

  for _ in {1..100}; do
    if health_probe >/dev/null 2>&1; then
      echo "agent-host: started PID ${pid} (Runner $(<"${runner_id_file}"))"
      return 0
    fi
    kill -0 "${pid}" 2>/dev/null || break
    sleep 0.1
  done
  if process_is_managed "${pid}"; then
    kill -TERM "${pid}" 2>/dev/null || true
  fi
  remove_pid_if_same "${pid}"
  fail "Runner failed its bounded health check; inspect ${log_file}"
}

status_runner() {
  ensure_runtime_layout
  ensure_identity_files
  if health_probe >/dev/null 2>&1; then
    local pid
    pid="$(read_pid 2>/dev/null || true)"
    echo "agent-host: healthy Runner $(<"${runner_id_file}")${pid:+, PID ${pid}}"
    return 0
  fi
  echo "agent-host: unavailable" >&2
  return 1
}

stop_runner() {
  ensure_runtime_layout
  local pid
  pid="$(read_pid 2>/dev/null || true)"
  if [[ -z "${pid}" ]]; then
    [[ ! -e "${pid_file}" ]] || fail "PID file is unsafe or malformed; refusing cleanup"
    echo "agent-host: no managed process"
    return 0
  fi
  if ! kill -0 "${pid}" 2>/dev/null; then
    remove_pid_if_same "${pid}"
    echo "agent-host: removed stale PID file"
    return 0
  fi
  process_is_managed "${pid}" || fail "PID ${pid} is not the managed Runner; refusing to signal it"
  kill -TERM "${pid}"
  for _ in {1..100}; do
    kill -0 "${pid}" 2>/dev/null || break
    sleep 0.1
  done
  if kill -0 "${pid}" 2>/dev/null; then
    process_is_managed "${pid}" || fail "PID ${pid} identity changed while stopping"
    kill -KILL "${pid}"
  fi
  remove_pid_if_same "${pid}"
  echo "agent-host: stopped PID ${pid}"
}

case "${command}" in
  build) build_runner ;;
  start) start_runner ;;
  prepare) build_runner; start_runner ;;
  status) status_runner ;;
  stop) stop_runner ;;
  restart) stop_runner; build_runner; start_runner ;;
esac
