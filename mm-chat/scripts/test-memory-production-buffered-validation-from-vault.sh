#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
wrapper_source="${script_dir}/run-memory-production-buffered-validation-from-vault.sh"
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/mm-chat-memory-production-buffered-validation-vault-test.XXXXXX")"
trap 'find "${temp_dir}" -depth -delete' EXIT
chmod 700 "${temp_dir}"

project_dir="${temp_dir}/project"
mkdir -p "${project_dir}/scripts" "${project_dir}/data/memory-benchmark/v2-regression"
cp "${wrapper_source}" "${project_dir}/scripts/run-memory-production-buffered-validation-from-vault.sh"
chmod 755 "${project_dir}/scripts/run-memory-production-buffered-validation-from-vault.sh"
: >"${project_dir}/compose.yml"
printf 'TEST_ONLY=true\n' >"${project_dir}/.env.single-server"
chmod 600 "${project_dir}/.env.single-server"
for required in fixtures.json corpus.json audit.json manifest.json; do
  printf '{}\n' >"${project_dir}/data/memory-benchmark/v2-regression/${required}"
  chmod 600 "${project_dir}/data/memory-benchmark/v2-regression/${required}"
done

fake_runner="${project_dir}/scripts/run-memory-regression.sh"
cat >"${fake_runner}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

bge=""
luna=""
output=""
capture_mode=""
provider_id=""
provider_type=""
base_url=""
model=""
live_approval=""
validation_approval=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --credential-file) bge="$2"; shift 2 ;;
    --configured-candidate-judge-credential-file) luna="$2"; shift 2 ;;
    --output-dir) output="$2"; shift 2 ;;
    --capture-mode) capture_mode="$2"; shift 2 ;;
    --configured-candidate-judge-provider-id) provider_id="$2"; shift 2 ;;
    --configured-candidate-judge-provider-type) provider_type="$2"; shift 2 ;;
    --configured-candidate-judge-base-url) base_url="$2"; shift 2 ;;
    --configured-candidate-judge-model) model="$2"; shift 2 ;;
    --live-approval) live_approval="$2"; shift 2 ;;
    --production-buffered-memory-judge-validation-approval) validation_approval="$2"; shift 2 ;;
    --regression-root|--cost-basis|--provider-mode) shift 2 ;;
    *) echo "fake runner: unexpected argument $1" >&2; exit 91 ;;
  esac
done

if [[ "${capture_mode}" != "production_fixed_memory_judge_negative_guard_buffered_validation" ||
  "${provider_id}" != "SERVER_DEFAULT" ||
  "${provider_type}" != "openai_compatible" ||
  "${base_url}" != "https://sub.mumubuku.top/v1" ||
  "${model}" != "gpt-5.6-luna" ||
  "${live_approval}" != "I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA" ||
  "${validation_approval}" != "I_UNDERSTAND_THIS_USES_REAL_FROZEN_BUFFERED_MEMORY_VALIDATION_QUOTA" ]]; then
  echo "fake runner: fixed production buffered Validation tuple drifted" >&2
  exit 92
fi
if [[ ! -f "${bge}" || -L "${bge}" || "$(stat -c '%a' "${bge}")" != "600" ||
  ! -f "${luna}" || -L "${luna}" || "$(stat -c '%a' "${luna}")" != "600" ||
  "$(cat "${bge}")" != "test-bge-secret" ||
  "$(cat "${luna}")" != "test-luna-secret" ||
  "${bge}" -ef "${luna}" ]] || cmp -s "${bge}" "${luna}"; then
  echo "fake runner: credential pair invalid" >&2
  exit 93
fi
printf 'runner-started\n' >>"${FAKE_RUNNER_LOG}"
if [[ -n "${FAKE_RUNNER_MARKER:-}" ]]; then
  : >"${FAKE_RUNNER_MARKER}"
fi
if [[ -n "${FAKE_RUNNER_ARTIFACT:-}" ]]; then
  printf 'aggregate-only-test-artifact\n' >"${output}/aggregate.json"
  chmod 600 "${output}/aggregate.json"
fi
if [[ -n "${FAKE_RUNNER_SLEEP:-}" ]]; then
  sleep "${FAKE_RUNNER_SLEEP}"
fi
exit "${FAKE_RUNNER_STATUS:-0}"
EOF
chmod 755 "${fake_runner}"

fake_docker="${temp_dir}/docker"
cat >"${fake_docker}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%q ' "$@" >>"${FAKE_DOCKER_LOG}"
printf '\n' >>"${FAKE_DOCKER_LOG}"
if [[ "${1:-}" == "compose" && "${2:-}" == "version" ]]; then
  exit 0
fi
if [[ "${1:-}" == "compose" && "${2:-}" == "run" && "${3:-}" == "--help" ]]; then
  printf '%s\n' '      --build' '      --no-deps' '      --pull string'
  if [[ "${FAKE_COMPOSE_RUN_SUPPORTS_NO_BUILD:-true}" == "true" ]]; then
    printf '%s\n' '      --no-build'
  fi
  exit 0
fi
if [[ " $* " == *" run "* ]]; then
  mount=""
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "-v" ]]; then
      mount="$2"
      shift 2
      continue
    fi
    shift
  done
  host_dir="${mount%:/export}"
  if [[ -z "${host_dir}" || "${host_dir}" == "${mount}" ]]; then
    echo "fake docker: export mount missing" >&2
    exit 81
  fi
  printf 'test-bge-secret' >"${host_dir}/bge.key"
  chmod 600 "${host_dir}/bge.key"
  if [[ "${FAKE_ADMIN_MODE:-success}" == "partial_failure" ]]; then
    exit 82
  fi
  printf 'test-luna-secret' >"${host_dir}/luna.key"
  chmod 600 "${host_dir}/luna.key"
  echo "memory validation credentials exported"
fi
EOF
chmod 755 "${fake_docker}"

cost_basis="${temp_dir}/cost-v13.json"
printf '{"schemaVersion":"neo-chat.memory-regression-cost-basis.v13"}\n' >"${cost_basis}"
chmod 600 "${cost_basis}"
wrapper="${project_dir}/scripts/run-memory-production-buffered-validation-from-vault.sh"
docker_log="${temp_dir}/docker.log"
runner_log="${temp_dir}/runner.log"
private_tmp="${temp_dir}/tmp"
mkdir "${private_tmp}"
chmod 700 "${private_tmp}"

signal_launcher="${temp_dir}/signal-launcher.py"
cat >"${signal_launcher}" <<'PY'
import os
import signal
import sys

os.setsid()
for name in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
    signal.signal(name, signal.SIG_DFL)
os.execvpe(sys.argv[1], sys.argv[1:], os.environ)
PY

base_args=(
  --cost-basis "${cost_basis}"
  --credential-export-approval I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS
  --siliconflow-live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA
  --production-buffered-validation-approval I_UNDERSTAND_THIS_USES_REAL_FROZEN_BUFFERED_MEMORY_VALIDATION_QUOTA
)

assert_no_export_directory() {
  if find "${private_tmp}" -mindepth 1 -maxdepth 1 -name 'mm-chat-memory-production-buffered-validation-credentials.*' -print -quit | grep -q .; then
    echo "Memory production buffered Validation Vault test: exported credential directory remains" >&2
    exit 1
  fi
}

run_status_case() {
  local name="$1"
  local expected_status="$2"
  local runner_status="$3"
  local artifact="$4"
  local supports_no_build="${5:-true}"
  local output="${temp_dir}/output-${name}"
  local stdout="${temp_dir}/${name}.stdout"
  local stderr="${temp_dir}/${name}.stderr"
  mkdir "${output}"
  chmod 700 "${output}"
  : >"${docker_log}"
  : >"${runner_log}"
  set +e
  TMPDIR="${private_tmp}" DOCKER_BIN="${fake_docker}" \
    FAKE_DOCKER_LOG="${docker_log}" FAKE_RUNNER_LOG="${runner_log}" \
    FAKE_RUNNER_STATUS="${runner_status}" FAKE_RUNNER_ARTIFACT="${artifact}" \
    FAKE_COMPOSE_RUN_SUPPORTS_NO_BUILD="${supports_no_build}" \
    bash "${wrapper}" "${base_args[@]}" --output-dir "${output}" \
    >"${stdout}" 2>"${stderr}"
  local status=$?
  set -e
  if [[ ${status} -ne ${expected_status} ]]; then
    echo "Memory production buffered Validation Vault test: ${name} status ${status}, want ${expected_status}" >&2
    cat "${stdout}" "${stderr}" >&2
    exit 1
  fi
  if [[ "$(cat "${runner_log}")" != "runner-started" ]]; then
    echo "Memory production buffered Validation Vault test: ${name} runner did not execute exactly once" >&2
    exit 1
  fi
  if grep -Fq 'test-bge-secret' "${stdout}" "${stderr}" ||
    grep -Fq 'test-luna-secret' "${stdout}" "${stderr}"; then
    echo "Memory production buffered Validation Vault test: ${name} leaked a credential" >&2
    exit 1
  fi
  if grep -Fq ' build ' "${docker_log}" ||
    ! grep -Fq -- '--pull never' "${docker_log}" ||
    { [[ "${supports_no_build}" == "true" ]] && ! grep -Fq -- '--no-build' "${docker_log}"; } ||
    { [[ "${supports_no_build}" != "true" ]] && grep -Fq -- '--no-build' "${docker_log}"; }; then
    echo "Memory production buffered Validation Vault test: ${name} mutated or pulled the active backend image" >&2
    exit 1
  fi
  if [[ "${artifact}" == "yes" && ! -f "${output}/aggregate.json" ]]; then
    echo "Memory production buffered Validation Vault test: ${name} lost retained aggregate artifact" >&2
    exit 1
  fi
  assert_no_export_directory
}

run_status_case success 0 0 ""
run_status_case success-compose-v5 0 0 "" false
run_status_case ordinary-failure 7 7 ""
run_status_case metric-failure 9 9 yes

partial_output="${temp_dir}/output-partial"
mkdir "${partial_output}"
chmod 700 "${partial_output}"
: >"${docker_log}"
: >"${runner_log}"
set +e
TMPDIR="${private_tmp}" DOCKER_BIN="${fake_docker}" \
  FAKE_DOCKER_LOG="${docker_log}" FAKE_RUNNER_LOG="${runner_log}" \
  FAKE_ADMIN_MODE=partial_failure \
  bash "${wrapper}" "${base_args[@]}" --output-dir "${partial_output}" \
  >"${temp_dir}/partial.stdout" 2>"${temp_dir}/partial.stderr"
partial_status=$?
set -e
if [[ ${partial_status} -eq 0 || -s "${runner_log}" ]]; then
  echo "Memory production buffered Validation Vault test: partial export did not fail before Validation" >&2
  exit 1
fi
assert_no_export_directory

for signal_name in INT TERM HUP; do
  signal_output="${temp_dir}/output-signal-${signal_name}"
  signal_marker="${temp_dir}/signal-${signal_name}.marker"
  mkdir "${signal_output}"
  chmod 700 "${signal_output}"
  : >"${docker_log}"
  : >"${runner_log}"
  env \
    TMPDIR="${private_tmp}" DOCKER_BIN="${fake_docker}" \
    FAKE_DOCKER_LOG="${docker_log}" FAKE_RUNNER_LOG="${runner_log}" \
    FAKE_RUNNER_SLEEP=30 FAKE_RUNNER_MARKER="${signal_marker}" \
    python3 "${signal_launcher}" bash "${wrapper}" "${base_args[@]}" \
    --output-dir "${signal_output}" \
    >"${temp_dir}/signal-${signal_name}.stdout" \
    2>"${temp_dir}/signal-${signal_name}.stderr" &
  wrapper_pid=$!
  for _ in $(seq 1 100); do
    [[ -f "${signal_marker}" ]] && break
    sleep 0.05
  done
  if [[ ! -f "${signal_marker}" ]]; then
    echo "Memory production buffered Validation Vault test: ${signal_name} runner did not start" >&2
    kill -TERM -- "-${wrapper_pid}" 2>/dev/null || true
    wait "${wrapper_pid}" || true
    exit 1
  fi
  kill -s "${signal_name}" -- "-${wrapper_pid}"
  set +e
  wait "${wrapper_pid}"
  signal_status=$?
  set -e
  if [[ ${signal_status} -eq 0 ]]; then
    echo "Memory production buffered Validation Vault test: ${signal_name} returned success" >&2
    exit 1
  fi
  assert_no_export_directory
done

: >"${docker_log}"
set +e
TMPDIR="${private_tmp}" DOCKER_BIN="${fake_docker}" FAKE_DOCKER_LOG="${docker_log}" \
  bash "${wrapper}" --cost-basis "${cost_basis}" --output-dir "${temp_dir}/denied" \
  >"${temp_dir}/denied.stdout" 2>"${temp_dir}/denied.stderr"
denied_status=$?
set -e
if [[ ${denied_status} -eq 0 || -s "${docker_log}" ]]; then
  echo "Memory production buffered Validation Vault test: missing approvals reached Docker" >&2
  exit 1
fi
assert_no_export_directory

bash -n "${wrapper_source}"
echo "Memory production buffered Validation Vault lifecycle: passed"
