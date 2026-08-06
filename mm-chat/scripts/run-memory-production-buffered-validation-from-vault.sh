#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
usage: run-memory-production-buffered-validation-from-vault.sh \
  --cost-basis <schema-v13-json> \
  --output-dir <private-run-parent> \
  --credential-export-approval I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS \
  --siliconflow-live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --production-buffered-validation-approval I_UNDERSTAND_THIS_USES_REAL_FROZEN_BUFFERED_MEMORY_VALIDATION_QUOTA \
  [--env-file <single-server-env>] \
  [--regression-root <protected-regression-root>]

Export only the active attested RAG:SILICONFLOW and fixed
SERVER_DEFAULT/gpt-5.6-luna credentials into a private one-run directory, run
the schema-v18 100-case negative-guard buffered production Memory Validation lane, and wipe the exported
copies on success, metric failure, ordinary failure, or signal.
EOF
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
compose_file="${project_dir}/compose.yml"
runner_script="${script_dir}/run-memory-regression.sh"
env_file="${project_dir}/.env.single-server"
regression_root="${project_dir}/data/memory-benchmark/v2-regression"
cost_basis=""
output_parent=""
credential_export_approval=""
siliconflow_live_approval=""
production_buffered_validation_approval=""
cost_basis_set=false
output_parent_set=false
credential_export_approval_set=false
siliconflow_live_approval_set=false
production_buffered_validation_approval_set=false
env_file_set=false
regression_root_set=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --cost-basis)
      [[ $# -ge 2 && "${cost_basis_set}" == false ]] || { usage >&2; exit 2; }
      cost_basis="$2"
      cost_basis_set=true
      shift 2
      ;;
    --output-dir)
      [[ $# -ge 2 && "${output_parent_set}" == false ]] || { usage >&2; exit 2; }
      output_parent="$2"
      output_parent_set=true
      shift 2
      ;;
    --credential-export-approval)
      [[ $# -ge 2 && "${credential_export_approval_set}" == false ]] || { usage >&2; exit 2; }
      credential_export_approval="$2"
      credential_export_approval_set=true
      shift 2
      ;;
    --siliconflow-live-approval)
      [[ $# -ge 2 && "${siliconflow_live_approval_set}" == false ]] || { usage >&2; exit 2; }
      siliconflow_live_approval="$2"
      siliconflow_live_approval_set=true
      shift 2
      ;;
    --production-buffered-validation-approval)
      [[ $# -ge 2 && "${production_buffered_validation_approval_set}" == false ]] || { usage >&2; exit 2; }
      production_buffered_validation_approval="$2"
      production_buffered_validation_approval_set=true
      shift 2
      ;;
    --env-file)
      [[ $# -ge 2 && "${env_file_set}" == false ]] || { usage >&2; exit 2; }
      env_file="$2"
      env_file_set=true
      shift 2
      ;;
    --regression-root)
      [[ $# -ge 2 && "${regression_root_set}" == false ]] || { usage >&2; exit 2; }
      regression_root="$2"
      regression_root_set=true
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "${cost_basis_set}" != true || "${output_parent_set}" != true ||
  "${credential_export_approval}" != "I_UNDERSTAND_THIS_EXPORTS_ACTIVE_MEMORY_VALIDATION_CREDENTIALS" ||
  "${siliconflow_live_approval}" != "I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA" ||
  "${production_buffered_validation_approval}" != "I_UNDERSTAND_THIS_USES_REAL_FROZEN_BUFFERED_MEMORY_VALIDATION_QUOTA" ]]; then
  echo "Memory production buffered Validation: exact one-run approvals are required" >&2
  exit 2
fi

for required in "${compose_file}" "${runner_script}" "${env_file}" "${cost_basis}"; do
  if [[ ! -f "${required}" || -L "${required}" ]]; then
    echo "Memory production buffered Validation: required input is missing or symlinked" >&2
    exit 1
  fi
done
if [[ "$(stat -c '%a' "${env_file}")" != "600" ||
  "$(stat -c '%a' "${cost_basis}")" != "600" ]]; then
  echo "Memory production buffered Validation: env and cost-basis files must be mode 0600" >&2
  exit 1
fi
if [[ ! -d "${regression_root}" || -L "${regression_root}" ]]; then
  echo "Memory production buffered Validation: protected regression root is missing or symlinked" >&2
  exit 1
fi
for required in fixtures.json corpus.json audit.json manifest.json; do
  if [[ ! -f "${regression_root}/${required}" || -L "${regression_root}/${required}" ]]; then
    echo "Memory production buffered Validation: protected regression input is missing or symlinked" >&2
    exit 1
  fi
done

cost_basis="$(realpath "${cost_basis}")"
env_file="$(realpath "${env_file}")"
regression_root="$(realpath "${regression_root}")"
output_parent="$(python3 - "${output_parent}" <<'PY'
import os
import sys
from pathlib import Path

raw = Path(sys.argv[1]).expanduser()
if not raw.is_absolute():
    raw = Path.cwd() / raw
raw = Path(os.path.abspath(raw))
probe = Path(raw.anchor)
for part in raw.parts[1:]:
    probe = probe / part
    if probe.exists() or probe.is_symlink():
        if probe.is_symlink():
            raise SystemExit("Memory production buffered Validation: output path contains a symlink")
        if probe == raw and not probe.is_dir():
            raise SystemExit("Memory production buffered Validation: output parent is not a directory")
print(os.path.normpath(str(raw)))
PY
)"
case "${output_parent}" in
  "${project_dir}/data"|"${project_dir}/data/"*|\
  "${project_dir}/secrets"|"${project_dir}/secrets/"*|\
  "${project_dir}/backup"|"${project_dir}/backup/"*)
    echo "Memory production buffered Validation: output cannot enter protected runtime state" >&2
    exit 1
    ;;
esac

umask 077
mkdir -p "${output_parent}"
if [[ ! -d "${output_parent}" || ! -w "${output_parent}" || ! -x "${output_parent}" ||
  "$(stat -c '%a' "${output_parent}")" != "700" ]]; then
  echo "Memory production buffered Validation: output parent must be a writable mode-0700 directory" >&2
  exit 1
fi

docker_bin="${DOCKER_BIN:-docker}"
docker_uses_windows_paths=false
if ! "${docker_bin}" compose version >/dev/null 2>&1; then
  windows_docker="/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe"
  if [[ -x "${windows_docker}" ]] && "${windows_docker}" compose version >/dev/null 2>&1; then
    docker_bin="${windows_docker}"
    docker_uses_windows_paths=true
  else
    echo "Memory production buffered Validation: Docker Compose is required" >&2
    exit 1
  fi
fi

docker_path() {
  if [[ "${docker_uses_windows_paths}" == true ]]; then
    wslpath -w "$1"
  else
    printf '%s\n' "$1"
  fi
}

compose=(
  "${docker_bin}" compose
  --project-directory "$(docker_path "${project_dir}")"
  --env-file "$(docker_path "${env_file}")"
  -f "$(docker_path "${compose_file}")"
  --profile ops
)

"${compose[@]}" config --quiet

# Compose v5 removed `run --no-build`; `run` still builds only when the
# positive `--build` flag is supplied. Keep `--pull never` mandatory and use
# `--no-build` when the installed CLI exposes it. This capability check runs
# before credentials are exported or any Provider request can start.
compose_run_no_build=()
compose_run_help="$("${docker_bin}" compose run --help 2>&1)" || {
  echo "Memory production buffered Validation: cannot inspect Compose run capabilities" >&2
  exit 1
}
if grep -Fq -- '--no-build' <<<"${compose_run_help}"; then
  compose_run_no_build=(--no-build)
fi

credential_dir="$(mktemp -d "${TMPDIR:-/tmp}/mm-chat-memory-production-buffered-validation-credentials.XXXXXX")"
chmod 700 "${credential_dir}"
bge_credential="${credential_dir}/bge.key"
luna_credential="${credential_dir}/luna.key"
export_container_name="mmchat-memory-production-buffered-validation-export-$$_$(date -u +%s)"
cleanup_failed=false

wipe_regular_file() {
  local path="$1"
  if [[ ! -f "${path}" || -L "${path}" ]]; then
    return
  fi
  python3 - "${path}" <<'PY'
import os
import sys

path = sys.argv[1]
size = os.path.getsize(path)
with open(path, "r+b", buffering=0) as handle:
    remaining = size
    block = b"\0" * min(65536, max(size, 1))
    while remaining:
        chunk = block[: min(len(block), remaining)]
        handle.write(chunk)
        remaining -= len(chunk)
    handle.flush()
    os.fsync(handle.fileno())
    handle.truncate(0)
os.remove(path)
PY
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM HUP
  set +e
  local export_containers
  export_containers="$("${docker_bin}" ps -aq \
    --filter "name=^/${export_container_name}$" 2>/dev/null)" || cleanup_failed=true
  if [[ -n "${export_containers}" ]]; then
    "${docker_bin}" rm -f ${export_containers} >/dev/null 2>&1 || cleanup_failed=true
  fi
  wipe_regular_file "${bge_credential}" || cleanup_failed=true
  wipe_regular_file "${luna_credential}" || cleanup_failed=true
  if [[ -d "${credential_dir}" ]]; then
    find "${credential_dir}" -depth -delete
  fi
  if [[ -e "${credential_dir}" ]]; then
    cleanup_failed=true
    echo "Memory production buffered Validation: exported credential cleanup failed" >&2
  fi
  if [[ "${cleanup_failed}" == true && ${status} -eq 0 ]]; then
    status=1
  fi
  if [[ "${cleanup_failed}" == false ]]; then
    echo "Memory production buffered Validation: one-run credential copies destroyed"
  fi
  exit "${status}"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

"${compose[@]}" run --rm --no-deps "${compose_run_no_build[@]}" --pull never -T \
  --name "${export_container_name}" \
  -v "$(docker_path "${credential_dir}"):/export" \
  admin memory-validation-credentials-export \
  --bge-output /export/bge.key \
  --luna-output /export/luna.key \
  --approval "${credential_export_approval}"

if [[ ! -f "${bge_credential}" || -L "${bge_credential}" ||
  ! -f "${luna_credential}" || -L "${luna_credential}" ||
  "$(stat -c '%a' "${bge_credential}")" != "600" ||
  "$(stat -c '%a' "${luna_credential}")" != "600" ||
  "${bge_credential}" -ef "${luna_credential}" ]] ||
  cmp -s "${bge_credential}" "${luna_credential}" ||
  [[ "$(find "${credential_dir}" -mindepth 1 -maxdepth 1 -type f | wc -l)" -ne 2 ||
  -n "$(find "${credential_dir}" -mindepth 1 -maxdepth 1 ! -type f -print -quit)" ]]; then
  echo "Memory production buffered Validation: exported credential pair failed preflight" >&2
  exit 1
fi

bash "${runner_script}" \
  --regression-root "${regression_root}" \
  --cost-basis "${cost_basis}" \
  --output-dir "${output_parent}" \
  --provider-mode live_siliconflow \
  --capture-mode production_fixed_memory_judge_negative_guard_buffered_validation \
  --credential-file "${bge_credential}" \
  --live-approval "${siliconflow_live_approval}" \
  --configured-candidate-judge-credential-file "${luna_credential}" \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --production-buffered-memory-judge-validation-approval "${production_buffered_validation_approval}"
