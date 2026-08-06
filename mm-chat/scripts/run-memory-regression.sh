#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
usage: run-memory-regression.sh \
  --cost-basis <versioned-json> \
  --output-dir <new-run-parent> \
  [--regression-root <protected-root>] \
  [--provider-mode fake_protocol|live_siliconflow] \
  [--capture-mode full_regression|development_calibration|development_cloud_judge|development_memory_tool_route|development_memory_tool_route_diagnostic|development_configured_candidate_judge|development_fixed_memory_judge|development_fixed_memory_judge_accuracy|development_fixed_memory_judge_failure_diagnostic|development_fixed_memory_judge_transport_stable|production_fixed_memory_judge_validation|frozen_validation] \
  [--cloud-judge-model <fixed-model-id>] \
  [--credential-file <mode-0600-file>] \
  [--live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA] \
  [--memory-tool-route-credential-file <independent-mode-0600-file>] \
  [--memory-tool-route-provider-id <configured-provider-id>] \
  [--memory-tool-route-provider-type openai|openai_compatible] \
  [--memory-tool-route-base-url <configured-base-url>] \
  [--memory-tool-route-model <exact-model-id>] \
  [--memory-tool-route-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA] \
  [--configured-candidate-judge-credential-file <independent-mode-0600-file>] \
  [--configured-candidate-judge-provider-id <configured-provider-id>] \
  [--configured-candidate-judge-provider-type openai|openai_compatible] \
  [--configured-candidate-judge-base-url <configured-base-url>] \
  [--configured-candidate-judge-model <exact-model-id>] \
  [--configured-candidate-judge-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA] \
  [--production-memory-judge-validation-approval I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_VALIDATION_QUOTA]

Run the production v1 lexical and native v2 hybrid Memory readers against the
protected machine regression corpus in a random isolated Compose project.
Live mode is split-safe: Development calibration and frozen Validation are
separate runs, and the visible machine holdout is never accepted. Fake protocol
mode has an internal-only network and is not reader-quality evidence. Runtime
objects and temporary credentials are always removed.
EOF
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
compose_file="${project_dir}/compose.memory-regression.yml"
regression_root="${project_dir}/data/memory-benchmark/v2-regression"
provider_mode="fake_protocol"
capture_mode="full_regression"
cost_basis=""
output_parent=""
credential_source=""
live_approval=""
cloud_judge_model="Qwen/Qwen3-8B"
memory_tool_route_credential_source=""
memory_tool_route_provider_id=""
memory_tool_route_provider_type=""
memory_tool_route_base_url=""
memory_tool_route_model=""
memory_tool_route_approval=""
configured_judge_credential_source=""
configured_judge_provider_id=""
configured_judge_provider_type=""
configured_judge_base_url=""
configured_judge_model=""
configured_judge_approval=""
production_validation_approval=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --provider-mode)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      provider_mode="$2"
      shift 2
      ;;
    --cost-basis)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      cost_basis="$2"
      shift 2
      ;;
    --capture-mode)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      capture_mode="$2"
      shift 2
      ;;
    --output-dir)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      output_parent="$2"
      shift 2
      ;;
    --regression-root)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      regression_root="$2"
      shift 2
      ;;
    --credential-file)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      credential_source="$2"
      shift 2
      ;;
    --live-approval)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      live_approval="$2"
      shift 2
      ;;
    --cloud-judge-model)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      cloud_judge_model="$2"
      shift 2
      ;;
    --memory-tool-route-credential-file)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      memory_tool_route_credential_source="$2"
      shift 2
      ;;
    --memory-tool-route-provider-id)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      memory_tool_route_provider_id="$2"
      shift 2
      ;;
    --memory-tool-route-provider-type)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      memory_tool_route_provider_type="$2"
      shift 2
      ;;
    --memory-tool-route-base-url)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      memory_tool_route_base_url="$2"
      shift 2
      ;;
    --memory-tool-route-model)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      memory_tool_route_model="$2"
      shift 2
      ;;
    --memory-tool-route-approval)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      memory_tool_route_approval="$2"
      shift 2
      ;;
    --configured-candidate-judge-credential-file)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      configured_judge_credential_source="$2"
      shift 2
      ;;
    --configured-candidate-judge-provider-id)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      configured_judge_provider_id="$2"
      shift 2
      ;;
    --configured-candidate-judge-provider-type)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      configured_judge_provider_type="$2"
      shift 2
      ;;
    --configured-candidate-judge-base-url)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      configured_judge_base_url="$2"
      shift 2
      ;;
    --configured-candidate-judge-model)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      configured_judge_model="$2"
      shift 2
      ;;
    --configured-candidate-judge-approval)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      configured_judge_approval="$2"
      shift 2
      ;;
    --production-memory-judge-validation-approval)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      production_validation_approval="$2"
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

if [[ -z "${cost_basis}" || -z "${output_parent}" ]]; then
  usage >&2
  exit 2
fi
case "${provider_mode}" in
  fake_protocol)
    if [[ -n "${credential_source}" || -n "${live_approval}" ]]; then
      echo "Memory regression: fake protocol rejects live credential/approval inputs" >&2
      exit 2
    fi
    compose_profile="memory-regression-fake"
    runner_service="memory-regression-fake-runner"
    candidate_prefix="native-v2-hybrid-fake-protocol"
    ;;
  live_siliconflow)
    if [[ -z "${credential_source}" || \
      "${live_approval}" != "I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA" ]]; then
      echo "Memory regression: live SiliconFlow mode requires a private credential file and exact quota approval" >&2
      exit 2
    fi
    compose_profile="memory-regression-live"
    runner_service="memory-regression-live-runner"
    candidate_prefix="native-v2-hybrid"
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
case "${capture_mode}" in
  full_regression)
    if [[ "${provider_mode}" != "fake_protocol" ]]; then
      echo "Memory regression: live full regression is forbidden; use Development calibration or frozen Validation" >&2
      exit 2
    fi
    if [[ -n "${memory_tool_route_credential_source}" || \
      -n "${memory_tool_route_provider_id}" || \
      -n "${memory_tool_route_provider_type}" || \
      -n "${memory_tool_route_base_url}" || \
      -n "${memory_tool_route_model}" || \
      -n "${memory_tool_route_approval}" ]]; then
      echo "Memory regression: Memory Tool-route inputs require development_memory_tool_route mode" >&2
      exit 2
    fi
    if [[ -n "${configured_judge_credential_source}" || \
      -n "${configured_judge_provider_id}" || \
      -n "${configured_judge_provider_type}" || \
      -n "${configured_judge_base_url}" || \
      -n "${configured_judge_model}" || \
      -n "${configured_judge_approval}" ]]; then
      echo "Memory regression: configured candidate-judge inputs require its Development mode" >&2
      exit 2
    fi
    ;;
  development_calibration | development_cloud_judge | frozen_validation)
    if [[ "${capture_mode}" == "development_cloud_judge" && -z "${cloud_judge_model}" ]]; then
      echo "Memory regression: cloud judge model is required" >&2
      exit 2
    fi
    if [[ -n "${memory_tool_route_credential_source}" || \
      -n "${memory_tool_route_provider_id}" || \
      -n "${memory_tool_route_provider_type}" || \
      -n "${memory_tool_route_base_url}" || \
      -n "${memory_tool_route_model}" || \
      -n "${memory_tool_route_approval}" ]]; then
      echo "Memory regression: Memory Tool-route inputs require development_memory_tool_route mode" >&2
      exit 2
    fi
    if [[ -n "${configured_judge_credential_source}" || \
      -n "${configured_judge_provider_id}" || \
      -n "${configured_judge_provider_type}" || \
      -n "${configured_judge_base_url}" || \
      -n "${configured_judge_model}" || \
      -n "${configured_judge_approval}" ]]; then
      echo "Memory regression: configured candidate-judge inputs require its Development mode" >&2
      exit 2
    fi
    ;;
  development_memory_tool_route | development_memory_tool_route_diagnostic)
    if [[ -z "${memory_tool_route_provider_id}" || \
      -z "${memory_tool_route_provider_type}" || \
      -z "${memory_tool_route_base_url}" || \
      -z "${memory_tool_route_model}" ]]; then
      echo "Memory regression: Memory Tool-route mode requires exact Provider ID/type/base URL/model" >&2
      exit 2
    fi
    case "${memory_tool_route_provider_type}" in
      openai | openai_compatible) ;;
      *)
        echo "Memory regression: Memory Tool-route Provider type must be openai or openai_compatible" >&2
        exit 2
        ;;
    esac
    if [[ ! "${memory_tool_route_provider_id}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ||
      ! "${memory_tool_route_model}" =~ ^[!-~]{1,512}$ ||
      ! "${memory_tool_route_base_url}" =~ ^[!-~]{1,2048}$ ]]; then
      echo "Memory regression: Memory Tool-route Provider/model identifier is invalid" >&2
      exit 2
    fi
    if [[ "${provider_mode}" == "live_siliconflow" ]]; then
      if [[ -z "${memory_tool_route_credential_source}" || \
        "${memory_tool_route_approval}" != "I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA" ]]; then
        echo "Memory regression: live Memory Tool-route mode requires an independent credential and exact quota approval" >&2
        exit 2
      fi
    elif [[ -n "${memory_tool_route_credential_source}" || \
      -n "${memory_tool_route_approval}" ]]; then
      echo "Memory regression: fake Memory Tool-route mode rejects live credential/approval inputs" >&2
      exit 2
    fi
    if [[ -n "${configured_judge_credential_source}" || \
      -n "${configured_judge_provider_id}" || \
      -n "${configured_judge_provider_type}" || \
      -n "${configured_judge_base_url}" || \
      -n "${configured_judge_model}" || \
      -n "${configured_judge_approval}" ]]; then
      echo "Memory regression: configured candidate-judge inputs require its Development mode" >&2
      exit 2
    fi
    ;;
  development_configured_candidate_judge | development_fixed_memory_judge | development_fixed_memory_judge_accuracy | development_fixed_memory_judge_failure_diagnostic | development_fixed_memory_judge_transport_stable | production_fixed_memory_judge_validation)
    if [[ -z "${configured_judge_provider_id}" || \
      -z "${configured_judge_provider_type}" || \
      -z "${configured_judge_base_url}" || \
      -z "${configured_judge_model}" ]]; then
      echo "Memory regression: configured candidate-judge mode requires exact Provider ID/type/base URL/model" >&2
      exit 2
    fi
    case "${configured_judge_provider_type}" in
      openai | openai_compatible) ;;
      *)
        echo "Memory regression: configured candidate-judge Provider type must be openai or openai_compatible" >&2
        exit 2
        ;;
    esac
    if [[ ! "${configured_judge_provider_id}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ||
      ! "${configured_judge_model}" =~ ^[!-~]{1,512}$ ||
      ! "${configured_judge_base_url}" =~ ^[!-~]{1,2048}$ ]]; then
      echo "Memory regression: configured candidate-judge Provider/model identifier is invalid" >&2
      exit 2
    fi
    if [[ "${capture_mode}" != "development_configured_candidate_judge" && \
      ("${configured_judge_provider_id}" != "SERVER_DEFAULT" || \
      "${configured_judge_provider_type}" != "openai_compatible" || \
      "${configured_judge_model}" != "gpt-5.6-luna") ]]; then
      echo "Memory regression: fixed Memory Judge authority drifted from SERVER_DEFAULT/openai_compatible/gpt-5.6-luna" >&2
      exit 2
    fi
    if [[ "${provider_mode}" == "live_siliconflow" ]]; then
      if [[ -z "${configured_judge_credential_source}" ]]; then
        echo "Memory regression: live configured candidate-judge mode requires an independent credential" >&2
        exit 2
      fi
      if [[ "${capture_mode}" == "production_fixed_memory_judge_validation" ]]; then
        if [[ "${production_validation_approval}" != "I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_VALIDATION_QUOTA" || \
          -n "${configured_judge_approval}" ]]; then
          echo "Memory regression: production Validation requires its independent exact quota approval" >&2
          exit 2
        fi
      elif [[ "${configured_judge_approval}" != "I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA" || \
        -n "${production_validation_approval}" ]]; then
        echo "Memory regression: live configured candidate-judge mode requires its exact Development quota approval" >&2
        exit 2
      fi
    elif [[ -n "${configured_judge_credential_source}" || \
      -n "${configured_judge_approval}" || \
      -n "${production_validation_approval}" ]]; then
      echo "Memory regression: fake configured candidate-judge mode rejects live credential/approval inputs" >&2
      exit 2
    fi
    if [[ -n "${memory_tool_route_credential_source}" || \
      -n "${memory_tool_route_provider_id}" || \
      -n "${memory_tool_route_provider_type}" || \
      -n "${memory_tool_route_base_url}" || \
      -n "${memory_tool_route_model}" || \
      -n "${memory_tool_route_approval}" ]]; then
      echo "Memory regression: Memory Tool-route inputs require its Development mode" >&2
      exit 2
    fi
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

memory_tool_route_base_url_sha256=""
if [[ "${capture_mode}" == "development_memory_tool_route" ||
  "${capture_mode}" == "development_memory_tool_route_diagnostic" ]]; then
  memory_tool_route_base_url="$(python3 - "${memory_tool_route_base_url}" <<'PY'
import sys
from urllib.parse import urlsplit

value = sys.argv[1].strip()
if value.endswith("#"):
    value = value[:-1]
value = value.rstrip("/")
if not value or value == "default" or len(value) > 2048:
    raise SystemExit("Memory regression: Memory Tool-route base URL is invalid")
if not value.endswith("/v1"):
    value += "/v1"
parsed = urlsplit(value)
if (
    parsed.scheme not in {"http", "https"}
    or not parsed.netloc
    or parsed.username is not None
    or parsed.password is not None
    or parsed.query
    or parsed.fragment
):
    raise SystemExit("Memory regression: Memory Tool-route base URL is invalid")
print(value)
PY
)"
  memory_tool_route_base_url_sha256="$(printf '%s' "${memory_tool_route_base_url}" | sha256sum | awk '{print $1}')"
fi

if [[ "${capture_mode}" != "production_fixed_memory_judge_validation" && \
  -n "${production_validation_approval}" ]]; then
  echo "Memory regression: production Validation approval requires production_fixed_memory_judge_validation mode" >&2
  exit 2
fi

configured_judge_base_url_sha256=""
if [[ "${capture_mode}" == "development_configured_candidate_judge" ||
  "${capture_mode}" == "development_fixed_memory_judge" ||
  "${capture_mode}" == "development_fixed_memory_judge_accuracy" ||
  "${capture_mode}" == "development_fixed_memory_judge_failure_diagnostic" ||
  "${capture_mode}" == "development_fixed_memory_judge_transport_stable" ||
  "${capture_mode}" == "production_fixed_memory_judge_validation" ]]; then
  configured_judge_base_url="$(python3 - "${configured_judge_base_url}" <<'PY'
import sys
from urllib.parse import urlsplit

value = sys.argv[1].strip()
if value.endswith("#"):
    value = value[:-1]
value = value.rstrip("/")
if not value or value == "default" or len(value) > 2048:
    raise SystemExit("Memory regression: configured candidate-judge base URL is invalid")
if not value.endswith("/v1"):
    value += "/v1"
parsed = urlsplit(value)
if (
    parsed.scheme not in {"http", "https"}
    or not parsed.netloc
    or parsed.username is not None
    or parsed.password is not None
    or parsed.query
    or parsed.fragment
):
    raise SystemExit("Memory regression: configured candidate-judge base URL is invalid")
print(value)
PY
)"
  configured_judge_base_url_sha256="$(printf '%s' "${configured_judge_base_url}" | sha256sum | awk '{print $1}')"
  if [[ "${capture_mode}" != "development_configured_candidate_judge" && \
    ("${configured_judge_base_url}" != "https://sub.mumubuku.top/v1" || \
    "${configured_judge_base_url_sha256}" != "3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671") ]]; then
    echo "Memory regression: fixed Memory Judge base URL authority drifted" >&2
    exit 2
  fi
fi

if [[ ! -d "${regression_root}" || -L "${regression_root}" ]]; then
  echo "Memory regression: protected regression root is missing or symlinked" >&2
  exit 1
fi
regression_root="$(realpath "${regression_root}")"

for required in \
  "${compose_file}" \
  "${regression_root}/fixtures.json" \
  "${regression_root}/corpus.json" \
  "${regression_root}/audit.json" \
  "${regression_root}/manifest.json"; do
  if [[ ! -f "${required}" || -L "${required}" ]]; then
    echo "Memory regression: protected input is missing or symlinked" >&2
    exit 1
  fi
done
if [[ ! -f "${cost_basis}" || -L "${cost_basis}" ]]; then
  echo "Memory regression: cost basis must be a regular non-symlink file" >&2
  exit 1
fi
cost_basis="$(realpath "${cost_basis}")"
if [[ "${provider_mode}" == "live_siliconflow" ]]; then
  if [[ ! -f "${credential_source}" || -L "${credential_source}" ]]; then
    echo "Memory regression: credential must be a regular non-symlink file" >&2
    exit 1
  fi
  credential_source="$(realpath "${credential_source}")"
  credential_mode="$(stat -c '%a' "${credential_source}")"
  if [[ "${credential_mode}" != "600" ]]; then
    echo "Memory regression: credential file mode must be 0600" >&2
    exit 1
  fi
  if [[ "${capture_mode}" == "development_memory_tool_route" ||
    "${capture_mode}" == "development_memory_tool_route_diagnostic" ]]; then
    if [[ ! -f "${memory_tool_route_credential_source}" || \
      -L "${memory_tool_route_credential_source}" ]]; then
      echo "Memory regression: Memory Tool-route credential must be a regular non-symlink file" >&2
      exit 1
    fi
    memory_tool_route_credential_source="$(realpath "${memory_tool_route_credential_source}")"
    memory_tool_route_credential_mode="$(stat -c '%a' "${memory_tool_route_credential_source}")"
    if [[ "${memory_tool_route_credential_mode}" != "600" ]]; then
      echo "Memory regression: Memory Tool-route credential file mode must be 0600" >&2
      exit 1
    fi
    if [[ "${credential_source}" -ef "${memory_tool_route_credential_source}" ]] || \
      cmp -s "${credential_source}" "${memory_tool_route_credential_source}"; then
      echo "Memory regression: retrieval and Memory Tool-route credentials must be independent" >&2
      exit 1
    fi
  fi
  if [[ "${capture_mode}" == "development_configured_candidate_judge" ||
    "${capture_mode}" == "development_fixed_memory_judge" ||
    "${capture_mode}" == "development_fixed_memory_judge_accuracy" ||
    "${capture_mode}" == "development_fixed_memory_judge_failure_diagnostic" ||
    "${capture_mode}" == "development_fixed_memory_judge_transport_stable" ||
    "${capture_mode}" == "production_fixed_memory_judge_validation" ]]; then
    if [[ ! -f "${configured_judge_credential_source}" || \
      -L "${configured_judge_credential_source}" ]]; then
      echo "Memory regression: configured candidate-judge credential must be a regular non-symlink file" >&2
      exit 1
    fi
    configured_judge_credential_source="$(realpath "${configured_judge_credential_source}")"
    configured_judge_credential_mode="$(stat -c '%a' "${configured_judge_credential_source}")"
    if [[ "${configured_judge_credential_mode}" != "600" ]]; then
      echo "Memory regression: configured candidate-judge credential file mode must be 0600" >&2
      exit 1
    fi
    if [[ "${credential_source}" -ef "${configured_judge_credential_source}" ]] || \
      cmp -s "${credential_source}" "${configured_judge_credential_source}"; then
      echo "Memory regression: retrieval and configured candidate-judge credentials must be independent" >&2
      exit 1
    fi
  fi
fi

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
            raise SystemExit("Memory regression: output path contains a symlink")
        if probe == raw and not probe.is_dir():
            raise SystemExit("Memory regression: output parent is not a directory")
print(os.path.normpath(str(raw)))
PY
)"
case "${output_parent}" in
  "${project_dir}/data"|"${project_dir}/data/"*|\
  "${project_dir}/secrets"|"${project_dir}/secrets/"*|\
  "${project_dir}/backup"|"${project_dir}/backup/"*)
    echo "Memory regression: output cannot enter protected runtime state" >&2
    exit 1
    ;;
esac

docker_bin="${DOCKER_BIN:-docker}"
docker_uses_windows_paths=false
if ! "${docker_bin}" compose version >/dev/null 2>&1; then
  windows_docker="/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe"
  if [[ -x "${windows_docker}" ]] && "${windows_docker}" compose version >/dev/null 2>&1; then
    docker_bin="${windows_docker}"
    docker_uses_windows_paths=true
  else
    echo "Memory regression: Docker Compose is required" >&2
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

umask 077
mkdir -p "${output_parent}"
if [[ ! -d "${output_parent}" || ! -w "${output_parent}" || ! -x "${output_parent}" ]]; then
  echo "Memory regression: output parent is not a writable directory" >&2
  exit 1
fi
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/mm-chat-memory-regression.XXXXXX")"
env_file="${temp_dir}/runner.env"
cost_copy="${temp_dir}/cost-basis.json"
credential_copy="${temp_dir}/provider.key"
memory_tool_route_credential_copy="${temp_dir}/memory-tool-route-provider.key"
configured_judge_credential_copy="${temp_dir}/configured-candidate-judge-provider.key"
capture_stdout="${temp_dir}/capture.stdout"
capture_stderr="${temp_dir}/capture.stderr"
metadata_snapshot="${temp_dir}/docker-metadata.json"
leak_free_marker="${temp_dir}/leak-free"
run_suffix="$(date -u +%Y%m%dT%H%M%SZ)-$(openssl rand -hex 4)"
run_id="memory-regression-${run_suffix,,}"
project_name="mmchat-memory-regression-${run_suffix,,}"
run_output="${output_parent}/${run_suffix}"
retain_output=false
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
os.remove(path)
PY
}

wipe_temporary_credentials() {
  local failed=false
  wipe_regular_file "${credential_copy}" || failed=true
  wipe_regular_file "${memory_tool_route_credential_copy}" || failed=true
  wipe_regular_file "${configured_judge_credential_copy}" || failed=true
  [[ "${failed}" == false ]]
}

early_cleanup() {
  local status=$?
  trap - EXIT INT TERM HUP
  set +e
  if [[ "${retain_output}" != true && -d "${run_output}" ]]; then
    find "${run_output}" -depth -delete
  fi
  wipe_temporary_credentials || true
  if [[ -d "${temp_dir}" ]]; then
    find "${temp_dir}" -depth -delete
  fi
  exit "${status}"
}

trap early_cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

if [[ -e "${run_output}" ]]; then
  echo "Memory regression: output run path already exists" >&2
  exit 1
fi
mkdir "${run_output}"
chmod 700 "${run_output}"
cp --no-preserve=mode,ownership,timestamps "${cost_basis}" "${cost_copy}"
chmod 600 "${cost_copy}"
if [[ "${provider_mode}" == "live_siliconflow" ]]; then
  cp --no-preserve=mode,ownership,timestamps "${credential_source}" "${credential_copy}"
else
  : >"${credential_copy}"
fi
chmod 600 "${credential_copy}"
if [[ "${provider_mode}" == "live_siliconflow" && \
  ("${capture_mode}" == "development_memory_tool_route" ||
  "${capture_mode}" == "development_memory_tool_route_diagnostic") ]]; then
  cp --no-preserve=mode,ownership,timestamps \
    "${memory_tool_route_credential_source}" \
    "${memory_tool_route_credential_copy}"
else
  : >"${memory_tool_route_credential_copy}"
fi
chmod 600 "${memory_tool_route_credential_copy}"
if [[ "${provider_mode}" == "live_siliconflow" && \
  ("${capture_mode}" == "development_configured_candidate_judge" ||
  "${capture_mode}" == "development_fixed_memory_judge" ||
  "${capture_mode}" == "development_fixed_memory_judge_accuracy" ||
  "${capture_mode}" == "development_fixed_memory_judge_failure_diagnostic" ||
  "${capture_mode}" == "development_fixed_memory_judge_transport_stable" ||
  "${capture_mode}" == "production_fixed_memory_judge_validation") ]]; then
  cp --no-preserve=mode,ownership,timestamps \
    "${configured_judge_credential_source}" \
    "${configured_judge_credential_copy}"
else
  : >"${configured_judge_credential_copy}"
fi
chmod 600 "${configured_judge_credential_copy}"

db_password="$(openssl rand -hex 32)"
memory_tool_route_credential_target=""
configured_judge_credential_target=""
if [[ "${provider_mode}" == "live_siliconflow" && \
  ("${capture_mode}" == "development_memory_tool_route" ||
  "${capture_mode}" == "development_memory_tool_route_diagnostic") ]]; then
  memory_tool_route_credential_target="/run/mm-chat-memory-regression/memory-tool-route-provider.key"
fi
if [[ "${provider_mode}" == "live_siliconflow" && \
  ("${capture_mode}" == "development_configured_candidate_judge" ||
  "${capture_mode}" == "development_fixed_memory_judge" ||
    "${capture_mode}" == "development_fixed_memory_judge_accuracy" ||
    "${capture_mode}" == "development_fixed_memory_judge_failure_diagnostic" ||
    "${capture_mode}" == "development_fixed_memory_judge_transport_stable" ||
    "${capture_mode}" == "production_fixed_memory_judge_validation") ]]; then
  configured_judge_credential_target="/run/mm-chat-memory-regression/configured-candidate-judge-provider.key"
fi
cat >"${env_file}" <<EOF
MEMORY_REGRESSION_DB_PASSWORD=${db_password}
MEMORY_REGRESSION_ROOT_PATH=$(docker_path "${regression_root}")
MEMORY_REGRESSION_COST_BASIS_PATH=$(docker_path "${cost_copy}")
MEMORY_REGRESSION_CREDENTIAL_PATH=$(docker_path "${credential_copy}")
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_CREDENTIAL_PATH=$(docker_path "${memory_tool_route_credential_copy}")
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_CREDENTIAL_TARGET=${memory_tool_route_credential_target}
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_CREDENTIAL_PATH=$(docker_path "${configured_judge_credential_copy}")
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_CREDENTIAL_TARGET=${configured_judge_credential_target}
MEMORY_REGRESSION_OUTPUT_PATH=$(docker_path "${run_output}")
MEMORY_REGRESSION_RUN_ID=${run_id}
MEMORY_REGRESSION_CAPTURE_MODE=${capture_mode}
MEMORY_REGRESSION_CLOUD_JUDGE_MODEL=${cloud_judge_model}
MEMORY_REGRESSION_LIVE_APPROVAL=${live_approval:-NOT_AUTHORIZED}
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_PROVIDER_ID=${memory_tool_route_provider_id}
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_PROVIDER_TYPE=${memory_tool_route_provider_type}
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_BASE_URL=${memory_tool_route_base_url}
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_BASE_URL_SHA256=${memory_tool_route_base_url_sha256}
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_MODEL=${memory_tool_route_model}
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_APPROVAL=${memory_tool_route_approval:-NOT_AUTHORIZED}
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_ID=${configured_judge_provider_id}
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_TYPE=${configured_judge_provider_type}
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_BASE_URL=${configured_judge_base_url}
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_BASE_URL_SHA256=${configured_judge_base_url_sha256}
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_MODEL=${configured_judge_model}
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_APPROVAL=${configured_judge_approval:-NOT_AUTHORIZED}
MEMORY_REGRESSION_PRODUCTION_MEMORY_JUDGE_VALIDATION_APPROVAL=${production_validation_approval:-NOT_AUTHORIZED}
EOF
chmod 600 "${env_file}"
unset db_password

compose=(
  "${docker_bin}" compose
  --project-name "${project_name}"
  --project-directory "$(docker_path "${project_dir}")"
  --env-file "$(docker_path "${env_file}")"
  -f "$(docker_path "${compose_file}")"
  --profile "${compose_profile}"
)

cleanup() {
  local status=$?
  trap - EXIT INT TERM HUP
  set +e

  "${compose[@]}" down --volumes --remove-orphans --timeout 10 >/dev/null 2>&1
  if [[ $? -ne 0 ]]; then
    cleanup_failed=true
  fi
  local containers networks volumes
  containers="$("${docker_bin}" ps -aq --filter "label=com.docker.compose.project=${project_name}" 2>/dev/null)" || cleanup_failed=true
  networks="$("${docker_bin}" network ls -q --filter "label=com.docker.compose.project=${project_name}" 2>/dev/null)" || cleanup_failed=true
  volumes="$("${docker_bin}" volume ls -q --filter "label=com.docker.compose.project=${project_name}" 2>/dev/null)" || cleanup_failed=true
  if [[ -n "${containers}" || -n "${networks}" || -n "${volumes}" ]]; then
    echo "Memory regression: scoped teardown left Compose runtime objects" >&2
    cleanup_failed=true
  fi

  if [[ "${retain_output}" != true && -d "${run_output}" ]]; then
    find "${run_output}" -depth -delete
  fi
  if ! wipe_temporary_credentials; then
    echo "Memory regression: temporary credential overwrite failed" >&2
    cleanup_failed=true
  fi
  if [[ -d "${temp_dir}" ]]; then
    find "${temp_dir}" -depth -delete
  fi
  if [[ -e "${temp_dir}" ]]; then
    echo "Memory regression: temporary credential directory still exists" >&2
    cleanup_failed=true
  fi

  if [[ "${cleanup_failed}" == true && ${status} -eq 0 ]]; then
    status=1
  fi
  if [[ "${cleanup_failed}" == false ]]; then
    echo "Memory regression: isolated runtime destroyed (${project_name})"
  fi
  exit "${status}"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

"${compose[@]}" config --quiet
"${compose[@]}" build memory-regression-postgres memory-regression-migrate "${runner_service}"
"${compose[@]}" up -d --wait --wait-timeout 300 memory-regression-postgres
"${compose[@]}" run --rm --no-deps -T memory-regression-migrate

set +e
"${compose[@]}" run --rm --no-deps -T "${runner_service}" >"${capture_stdout}" 2>"${capture_stderr}"
runner_status=$?
set -e

container_ids="$("${docker_bin}" ps -aq --filter "label=com.docker.compose.project=${project_name}")"
if [[ -n "${container_ids}" ]]; then
  # The Provider key is a bind-mounted file only. This snapshot is retained
  # only inside the temporary directory for exact leak validation.
  "${docker_bin}" inspect ${container_ids} >"${metadata_snapshot}"
else
  printf '[]\n' >"${metadata_snapshot}"
fi

set +e
python3 - \
  "${run_output}" \
  "${regression_root}" \
  "${credential_copy}" \
  "${memory_tool_route_credential_copy}" \
  "${configured_judge_credential_copy}" \
  "${capture_stdout}" \
  "${capture_stderr}" \
  "${metadata_snapshot}" \
  "${provider_mode}" \
  "${candidate_prefix}" \
  "${capture_mode}" \
  "${leak_free_marker}" \
  "${cloud_judge_model}" \
  "${memory_tool_route_provider_id}" \
  "${memory_tool_route_provider_type}" \
  "${memory_tool_route_base_url_sha256}" \
  "${memory_tool_route_model}" \
  "${configured_judge_provider_id}" \
  "${configured_judge_provider_type}" \
  "${configured_judge_base_url_sha256}" \
  "${configured_judge_model}" <<'PY'
import hashlib
import json
import math
import sys
from pathlib import Path

output = Path(sys.argv[1])
root = Path(sys.argv[2])
credential_path = Path(sys.argv[3])
memory_tool_route_credential_path = Path(sys.argv[4])
configured_judge_credential_path = Path(sys.argv[5])
stdout_path = Path(sys.argv[6])
stderr_path = Path(sys.argv[7])
metadata_path = Path(sys.argv[8])
mode = sys.argv[9]
candidate_prefix = sys.argv[10]
capture_mode = sys.argv[11]
leak_free_marker = Path(sys.argv[12])
cloud_judge_model = sys.argv[13]
memory_tool_route_provider_id = sys.argv[14]
memory_tool_route_provider_type = sys.argv[15]
memory_tool_route_base_url_sha256 = sys.argv[16]
memory_tool_route_model = sys.argv[17]
configured_judge_provider_id = sys.argv[18]
configured_judge_provider_type = sys.argv[19]
configured_judge_base_url_sha256 = sys.argv[20]
configured_judge_model = sys.argv[21]

fixtures = json.loads((root / "fixtures.json").read_text(encoding="utf-8"))
corpus = json.loads((root / "corpus.json").read_text(encoding="utf-8"))
forbidden = []
for fixture in fixtures["fixtures"]:
    for memory in fixture["memories"]:
        value = memory.get("canonicalContent", "").encode()
        if len(value) >= 8:
            forbidden.append(value)
for case in corpus["cases"]:
    value = case.get("query", "").encode()
    if len(value) >= 8:
        forbidden.append(value)
credential = credential_path.read_bytes().rstrip(b"\r\n")
if credential:
    forbidden.append(credential)
memory_tool_route_credential = memory_tool_route_credential_path.read_bytes().rstrip(b"\r\n")
if memory_tool_route_credential:
    forbidden.append(memory_tool_route_credential)
configured_judge_credential = configured_judge_credential_path.read_bytes().rstrip(b"\r\n")
if configured_judge_credential:
    forbidden.append(configured_judge_credential)

retained_and_logs = [path.read_bytes() for path in output.iterdir() if path.is_file()]
retained_and_logs += [stdout_path.read_bytes(), stderr_path.read_bytes(), metadata_path.read_bytes()]
for body in retained_and_logs:
    if any(value in body for value in forbidden):
        raise SystemExit("protected plaintext or credential leaked into retained output/log/metadata")
leak_free_marker.write_text("ok\n", encoding="utf-8")
leak_free_marker.chmod(0o600)

if capture_mode == "full_regression":
    expected = {
        "native-v1-lexical.observations.json",
        "native-v1-lexical.report.json",
        f"{candidate_prefix}.observations.json",
        f"{candidate_prefix}.report.json",
        "run-manifest.json",
    }
elif capture_mode == "development_calibration":
    expected = {"relevance-calibration.json", "run-manifest.json"}
elif capture_mode == "development_cloud_judge":
    expected = {"cloud-judge-development.json", "run-manifest.json"}
elif capture_mode == "development_memory_tool_route":
    expected = {"memory-first-tool-round-development.json", "run-manifest.json"}
elif capture_mode == "development_memory_tool_route_diagnostic":
    expected = {"memory-first-tool-round-route-diagnostic-development.json", "run-manifest.json"}
elif capture_mode == "development_configured_candidate_judge":
    expected = {"configured-candidate-judge-development.json", "run-manifest.json"}
elif capture_mode == "development_fixed_memory_judge":
    expected = {"fixed-memory-judge-development.json", "run-manifest.json"}
elif capture_mode == "development_fixed_memory_judge_accuracy":
    expected = {"fixed-memory-judge-accuracy-development.json", "run-manifest.json"}
elif capture_mode == "development_fixed_memory_judge_failure_diagnostic":
    expected = {"fixed-memory-judge-failure-diagnostic-development.json", "run-manifest.json"}
elif capture_mode == "development_fixed_memory_judge_transport_stable":
    expected = {"fixed-memory-judge-transport-stable-development.json", "run-manifest.json"}
elif capture_mode == "production_fixed_memory_judge_validation":
    expected = {"fixed-memory-judge-production-validation.json", "run-manifest.json"}
elif capture_mode == "frozen_validation":
    expected = {"relevance-validation.json", "run-manifest.json"}
else:
    raise SystemExit("capture mode drift")
actual = {path.name for path in output.iterdir() if path.is_file()}
if actual != expected:
    raise SystemExit(f"artifact set drift: {sorted(actual)}")
for path in output.iterdir():
    if path.is_symlink() or not path.is_file() or (path.stat().st_mode & 0o777) != 0o600:
        raise SystemExit("artifact permission/type drift")

manifest = json.loads((output / "run-manifest.json").read_text(encoding="utf-8"))
expected_manifest_schema = {
    "full_regression": "neo-chat.memory-regression-native-run.v1",
    "production_fixed_memory_judge_validation": "neo-chat.memory-regression-relevance-validation-run.v15",
}.get(capture_mode, "neo-chat.memory-regression-relevance-run.v1")
if manifest.get("schemaVersion") != expected_manifest_schema:
    raise SystemExit("invalid run manifest schema")
if manifest.get("corpusClass") != "machine_reviewed_regression":
    raise SystemExit("native run corpus class drift")
expected_admission = {
    "full_regression": "regression_only",
    "development_calibration": "development_calibration_only",
    "development_cloud_judge": "development_cloud_judge_only",
    "development_memory_tool_route": "development_main_model_first_tool_round_only",
    "development_memory_tool_route_diagnostic": "development_main_model_first_tool_round_route_failure_diagnostic_only",
    "development_configured_candidate_judge": "development_configured_candidate_judge_only",
    "development_fixed_memory_judge": "development_fixed_memory_judge_only",
    "development_fixed_memory_judge_accuracy": "development_fixed_memory_judge_accuracy_only",
    "development_fixed_memory_judge_failure_diagnostic": "development_fixed_memory_judge_failure_diagnostic_only",
    "development_fixed_memory_judge_transport_stable": "development_fixed_memory_judge_transport_stable_only",
    "production_fixed_memory_judge_validation": "frozen_production_fixed_memory_judge_validation_only",
    "frozen_validation": "frozen_validation_only",
}[capture_mode]
if manifest.get("admissionMode") != expected_admission or manifest.get("promotionEligible") is not False:
    raise SystemExit("native run gained promotion authority")
if manifest.get("providerMode") != mode:
    raise SystemExit("native run Provider mode drift")
candidate_profile = "native_v2_hybrid" if mode == "live_siliconflow" else "native_v2_hybrid_fake_protocol"
if capture_mode == "full_regression":
    profiles = manifest.get("profiles", [])
    if [(item.get("role"), item.get("profileId")) for item in profiles] != [
        ("baseline", "native_v1_lexical"),
        ("candidate", candidate_profile),
    ]:
        raise SystemExit("native run profile authority drift")
else:
    expected_split = "development" if capture_mode in {
        "development_calibration",
        "development_cloud_judge",
        "development_memory_tool_route",
        "development_memory_tool_route_diagnostic",
        "development_configured_candidate_judge",
        "development_fixed_memory_judge",
        "development_fixed_memory_judge_accuracy",
        "development_fixed_memory_judge_failure_diagnostic",
        "development_fixed_memory_judge_transport_stable",
    } else "validation"
    if manifest.get("captureMode") != capture_mode or manifest.get("split") != expected_split:
        raise SystemExit("relevance run split authority drift")
    if manifest.get("profileId") != candidate_profile:
        raise SystemExit("relevance run profile authority drift")

manifest_artifacts = manifest.get("artifacts", [])
if {item.get("name") for item in manifest_artifacts} != expected - {"run-manifest.json"}:
    raise SystemExit("native run artifact manifest drift")
for item in manifest_artifacts:
    body = (output / item["name"]).read_bytes()
    if item.get("bytes") != len(body) or item.get("sha256") != hashlib.sha256(body).hexdigest():
        raise SystemExit("native run artifact hash drift")

if capture_mode == "full_regression":
    for prefix in ("native-v1-lexical", candidate_prefix):
        observations = json.loads((output / f"{prefix}.observations.json").read_text(encoding="utf-8"))
        report = json.loads((output / f"{prefix}.report.json").read_text(encoding="utf-8"))
        if observations.get("schemaVersion") != "neo-chat.memory-benchmark-regression-observations.v1":
            raise SystemExit("invalid regression observations schema")
        if len(observations.get("cases", [])) != 500:
            raise SystemExit("regression observation case count drift")
        if report.get("schemaVersion") != "neo-chat.memory-benchmark-regression-report.v1":
            raise SystemExit("invalid regression report schema")
        if report.get("corpusClass") != "machine_reviewed_regression":
            raise SystemExit("regression report corpus class drift")
        if report.get("admissionMode") != "regression_only" or report.get("promotionEligible") is not False:
            raise SystemExit("regression report gained promotion authority")
elif capture_mode == "production_fixed_memory_judge_validation":
    report = json.loads(
        (output / "fixed-memory-judge-production-validation.json").read_text(
            encoding="utf-8"
        )
    )
    criteria = report.get("evaluationCriteria")
    execution = report.get("executionPolicy")
    evaluation = report.get("evaluation")
    diagnostics = report.get("diagnostics")
    attempts = report.get("providerAttempts")
    authority = report.get("costAuthority")
    outcome = report.get("outcome")
    expected_clock = "wall_clock_v1" if mode == "live_siliconflow" else "virtual_protocol_v1"
    expected_evidence = "live_validation" if mode == "live_siliconflow" else "fake_protocol_lifecycle_only"
    hash_fields = (
        "configurationSha256",
        "validationCaseOrderSha256",
        "corpusRawSha256",
        "productionRelevancePolicySha256",
        "memoryReadIntentPolicySha256",
        "evaluationCriteriaSha256",
    )
    expected_hash_bindings = (
        "configurationSha256",
        "validationCaseOrderSha256",
        "productionRelevancePolicySha256",
        "memoryReadIntentPolicySha256",
        "evaluationCriteriaSha256",
    )
    if (
        report.get("schemaVersion") != "neo-chat.memory-regression-relevance-validation.v15"
        or report.get("corpusClass") != "machine_reviewed_regression"
        or report.get("admissionMode") != expected_admission
        or report.get("promotionEligible") is not False
        or report.get("releaseEligible") is not False
        or report.get("policySelected") is not False
        or report.get("executionComplete") is not True
        or report.get("evidenceClass") != expected_evidence
        or report.get("split") != "validation"
        or report.get("caseCount") != 100
        or report.get("profileId") != candidate_profile
        or report.get("policyId") != "memory_hybrid_fixed_cloud_candidate_judge_production_v1"
        or report.get("providerEgressPolicy") != "owner_authorized_normal_candidates_v1"
        or report.get("providerCostPolicy") != "owner_authorized_absolute_cap_v1"
        or report.get("providerCostAuthorized") is not True
        or report.get("judgeProviderId") != "SERVER_DEFAULT"
        or report.get("judgeProviderType") != "openai_compatible"
        or report.get("judgeBaseUrlSha256") != "3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671"
        or report.get("judgeModelId") != "gpt-5.6-luna"
        or report.get("judgeAdapter") != "chat-configured-candidate-judge-v1"
        or report.get("judgePromptVersion") != "memory-cloud-candidate-judge-prompt-v1"
        or report.get("judgePromptSha256") != "c004e834f2db572fc8393f088f47750d420379664f972357f987a09d8647f9c8"
        or report.get("judgeDecodingProfile") != "temperature-0_max-output-128_no-thinking_v1"
        or report.get("selectionAlgorithm") != "strict-ordinal_intersect-bge-order_top5-token-budget_v1"
        or report.get("evaluationCriteriaVersion") != "neo-chat.memory-benchmark-criteria.v3"
        or report.get("memoryReadIntentPolicyVersion") != "memory-explicit-read-intent-v1"
        or report.get("memoryReadIntentPolicySha256") != "538d9ccff34fb976cedfca0d9e153078cb3ce36f1baff0691f1d2124d182119c"
        or report.get("failureTaxonomyVersion") != "memory-candidate-judge-failure-taxonomy-v1"
        or report.get("failureTaxonomySha256") != "c22cb137da8b5fda87526237446519dd9abe2c8d221ad703c5445358d9059f8d"
        or report.get("diagnosticCompleteness") != "attempt_terminal_reconciled_fail_closed_v1"
        or any(
            not isinstance(report.get(name), str)
            or len(report[name]) != 64
            or any(character not in "0123456789abcdef" for character in report[name])
            for name in hash_fields
        )
        or any(manifest.get(name) != report.get(name) for name in expected_hash_bindings)
        or manifest.get("policyId") != report.get("policyId")
        or manifest.get("providerCostPolicy") != report.get("providerCostPolicy")
        or not isinstance(criteria, dict)
        or criteria.get("maximumFalseInjectionRate") != 0.02
        or criteria.get("latencyEvaluationMode") != "diagnostic_only_v1"
        or criteria.get("applicationDeadlineMode") != "none_v1"
        or not isinstance(evaluation, dict)
        or not isinstance(diagnostics, dict)
        or not isinstance(attempts, dict)
        or not isinstance(authority, dict)
        or not isinstance(outcome, dict)
    ):
        raise SystemExit("production Memory Judge Validation authority drift")
    if (
        not isinstance(execution, dict)
        or execution.get("sequenceVersion") != "production_bge_m3_rerank_fixed_luna_judge_record_serial_v1"
        or execution.get("globalProviderRequestConcurrency") != 1
        or execution.get("applicationDeadlineMode") != "none_v1"
        or execution.get("providerElapsedTimeoutMode") != "none_v1"
        or execution.get("latencyEvaluationMode") != "diagnostic_only_v1"
        or execution.get("interCaseCooldownMilliseconds") != 1000
        or execution.get("interCaseCooldownClock") != expected_clock
        or execution.get("retryPolicyVersion") != "transient_408_429_5xx_transport_read_judge_twice_v2"
        or execution.get("maximumRetriesPerProviderRequest") != 1
        or execution.get("retryFallbackDelayMilliseconds") != 5000
        or execution.get("maximumJudgeRetriesPerRequest") != 2
        or execution.get("secondJudgeRetryDelayMilliseconds") != 10000
    ):
        raise SystemExit("production Memory Judge Validation execution drift")

    def nonnegative_int(value):
        return type(value) is int and value >= 0

    failure_codes = diagnostics.get("failureCodeCounts")
    terminal_counts = diagnostics.get("judgeTerminalFailureCategoryCounts")
    attempt_counts = attempts.get("judgeAttemptFailureCategoryCounts")
    aggregate_names = (
        "emptyCandidateCaseCount",
        "judgeCompletedCaseCount",
        "failedCaseCount",
    )
    if (
        not isinstance(failure_codes, dict)
        or not isinstance(terminal_counts, dict)
        or not isinstance(attempt_counts, dict)
        or any(not isinstance(value, str) or not nonnegative_int(count) for value, count in failure_codes.items())
        or any(not isinstance(value, str) or not nonnegative_int(count) for value, count in terminal_counts.items())
        or any(not isinstance(value, str) or not nonnegative_int(count) for value, count in attempt_counts.items())
        or any(not nonnegative_int(diagnostics.get(name)) for name in aggregate_names)
        or not nonnegative_int(attempts.get("judgeAttempts"))
        or not nonnegative_int(attempts.get("judgeRetries"))
        or not nonnegative_int(attempts.get("queryEmbeddingAttempts"))
        or not nonnegative_int(attempts.get("queryEmbeddingRetries"))
        or not nonnegative_int(attempts.get("judgeInputTokenUpperBound"))
        or not nonnegative_int(attempts.get("interCaseCooldownCount"))
        or not nonnegative_int(attempts.get("interCaseCooldownMilliseconds"))
        or not nonnegative_int(attempts.get("interCaseCooldownElapsedMilliseconds"))
        or any(
            not nonnegative_int(authority.get(name))
            for name in (
                "authorizedRequestCount",
                "actualRequestCount",
                "authorizedMaximumInputTokens",
                "actualInputTokenUpperBound",
                "authorizedMaximumOutputTokens",
                "actualOutputTokenUpperBound",
            )
        )
    ):
        raise SystemExit("production Memory Judge Validation aggregate types drift")
    logical_judges = diagnostics["judgeCompletedCaseCount"] + failure_codes.get(
        "CANDIDATE_JUDGE_FAILED", 0
    )
    if (
        sum(diagnostics[name] for name in aggregate_names) != 100
        or sum(terminal_counts.values()) != failure_codes.get("CANDIDATE_JUDGE_FAILED", 0)
        or attempts["judgeAttempts"] != logical_judges + attempts["judgeRetries"]
        or attempts["judgeRetries"] > logical_judges * 2
        or attempts["queryEmbeddingAttempts"] != 100 + attempts["queryEmbeddingRetries"]
        or attempts["interCaseCooldownCount"] != 99
        or attempts["interCaseCooldownMilliseconds"] != 99000
        or (mode == "fake_protocol" and attempts["interCaseCooldownElapsedMilliseconds"] != 0)
        or authority["authorizedRequestCount"] != 300
        or authority["actualRequestCount"] != attempts["judgeAttempts"]
        or authority["actualInputTokenUpperBound"] != attempts["judgeInputTokenUpperBound"]
        or authority["actualOutputTokenUpperBound"] != attempts["judgeAttempts"] * 128
        or authority["actualInputTokenUpperBound"] > authority["authorizedMaximumInputTokens"]
        or authority["actualOutputTokenUpperBound"] > authority["authorizedMaximumOutputTokens"]
    ):
        raise SystemExit("production Memory Judge Validation aggregate/cost drift")
    allowed_outcomes = {
        "none": "owner_review_no_automatic_release",
        "yellow": "retain_beta",
        "orange": "disable_memory_recall_preserve_data",
        "red": "disable_memory_tool_loop",
    }
    if (
        outcome.get("requiredAction") != allowed_outcomes.get(outcome.get("severity"))
        or not isinstance(outcome.get("reasons"), list)
        or any(not isinstance(reason, str) for reason in outcome.get("reasons", []))
        or manifest.get("passed") is not report.get("passed")
        or manifest.get("releaseEligible") is not False
        or manifest.get("evidenceClass") != report.get("evidenceClass")
        or manifest.get("outcome") != outcome
    ):
        raise SystemExit("production Memory Judge Validation outcome drift")
    if mode == "fake_protocol":
        if (
            report.get("passed") is not False
            or outcome.get("severity") != "yellow"
            or outcome.get("requiredAction") != "retain_beta"
            or outcome.get("reasons") != ["FAKE_PROTOCOL_NON_EVIDENCE"]
        ):
            raise SystemExit("Fake protocol was accepted as Validation evidence")
    else:
        safety = evaluation.get("safety")
        metrics = evaluation.get("metrics")
        if not isinstance(safety, dict) or not isinstance(metrics, dict):
            raise SystemExit("production Validation evaluation aggregates missing")
        if safety.get("passed") is not True:
            expected_outcome = {
                "severity": "red",
                "requiredAction": "disable_memory_tool_loop",
                "reasons": ["AUTHORIZATION_OR_PRIVACY_RELEASE"],
            }
        elif type(metrics.get("falseInjectionRate")) not in (int, float) or not math.isfinite(
            float(metrics["falseInjectionRate"])
        ):
            raise SystemExit("production Validation false-injection metric drift")
        elif metrics["falseInjectionRate"] > criteria["maximumFalseInjectionRate"]:
            expected_outcome = {
                "severity": "orange",
                "requiredAction": "disable_memory_recall_preserve_data",
                "reasons": ["FALSE_INJECTION_ABOVE_0_02"],
            }
        elif diagnostics["failedCaseCount"] > 0:
            expected_outcome = {
                "severity": "yellow",
                "requiredAction": "retain_beta",
                "reasons": ["PROVIDER_STABILITY_FAILURE"],
            }
        elif evaluation.get("passed") is not True:
            expected_outcome = {
                "severity": "yellow",
                "requiredAction": "retain_beta",
                "reasons": ["QUALITY_OR_TOKEN_GATE_FAILURE"],
            }
        else:
            expected_outcome = {
                "severity": "none",
                "requiredAction": "owner_review_no_automatic_release",
                "reasons": [],
            }
        expected_passed = (
            evaluation.get("passed") is True
            and diagnostics["failedCaseCount"] == 0
            and expected_outcome["severity"] == "none"
        )
        if outcome != expected_outcome or report.get("passed") is not expected_passed:
            raise SystemExit("production Validation pass/failure action drift")

    forbidden_keys = {
        "query",
        "canonicalContent",
        "providerResponse",
        "providerError",
        "rawScore",
        "caseId",
        "caseIds",
        "memoryId",
        "memoryIds",
    }

    def reject_forbidden_keys(value):
        if isinstance(value, dict):
            if forbidden_keys.intersection(value):
                raise SystemExit("production Validation persisted case-level or Provider payload")
            for key, nested in value.items():
                if key == "cases" and isinstance(nested, (dict, list)):
                    raise SystemExit("production Validation persisted case-level identities")
                reject_forbidden_keys(nested)
        elif isinstance(value, list):
            for nested in value:
                reject_forbidden_keys(nested)

    reject_forbidden_keys(report)
    reject_forbidden_keys(manifest)
elif capture_mode == "development_calibration":
    report = json.loads((output / "relevance-calibration.json").read_text(encoding="utf-8"))
    if report.get("schemaVersion") != "neo-chat.memory-regression-relevance-calibration.v3":
        raise SystemExit("invalid calibration report schema")
    if report.get("split") != "development" or report.get("caseCount") != 300:
        raise SystemExit("calibration split drift")
    if report.get("admissionMode") != expected_admission or report.get("promotionEligible") is not False:
        raise SystemExit("calibration report gained promotion authority")
    diagnostics = report.get("diagnostics")
    if not isinstance(diagnostics, dict) or diagnostics.get("version") != "aggregate-threshold-curves-intent-and-attempts-v2":
        raise SystemExit("calibration diagnostics authority drift")
    if (
        report.get("intentSelectionAlgorithm") != "zero-egress_max-recall_highest-intent-margin_v1"
        or report.get("intentEvaluatedThresholdCount") != 201
    ):
        raise SystemExit("calibration intent selection authority drift")
    if not isinstance(diagnostics.get("failurePairCounts"), dict) or not isinstance(diagnostics.get("intentFailureThresholdCounts"), dict):
        raise SystemExit("calibration failure aggregate drift")
    if not isinstance(diagnostics.get("bestSafetyAttempt"), dict) or not isinstance(diagnostics.get("bestRecallAttempt"), dict):
        raise SystemExit("calibration attempt aggregate missing")
    if not isinstance(diagnostics.get("bestIntentSafetyAttempt"), dict) or not isinstance(diagnostics.get("bestIntentRecallAttempt"), dict):
        raise SystemExit("calibration intent attempt aggregate missing")
    for name, minimum, maximum, count in (
        ("memoryIntentMarginCurve", -100, 100, 201),
        ("admissionSimilarityCurve", -100, 100, 201),
        ("maximumRerankScoreCurve", 0, 100, 101),
        ("topTwoRerankMarginCurve", 0, 100, 101),
    ):
        curve = diagnostics.get(name)
        if not isinstance(curve, dict):
            raise SystemExit("calibration curve missing")
        if (
            curve.get("minimumBasisPoints") != minimum
            or curve.get("maximumBasisPoints") != maximum
            or curve.get("stepBasisPoints") != 1
            or len(curve.get("relevantPassingCaseCounts", [])) != count
            or len(curve.get("unrelatedNegativePassingCaseCounts", [])) != count
        ):
            raise SystemExit("calibration curve authority drift")
elif capture_mode == "development_cloud_judge":
    report = json.loads((output / "cloud-judge-development.json").read_text(encoding="utf-8"))
    schema = report.get("schemaVersion")
    if schema not in {
        "neo-chat.memory-regression-relevance-calibration.v4",
        "neo-chat.memory-regression-relevance-calibration.v5",
    }:
        raise SystemExit("invalid cloud-judge Development report schema")
    if report.get("split") != "development" or report.get("caseCount") != 300:
        raise SystemExit("cloud-judge Development split drift")
    if report.get("admissionMode") != expected_admission or report.get("promotionEligible") is not False:
        raise SystemExit("cloud-judge Development report gained promotion authority")
    if report.get("judgeModelId") != cloud_judge_model:
        raise SystemExit("cloud-judge Development model drift")
    if report.get("providerEgressPolicy") != "owner_authorized_normal_candidates_v1":
        raise SystemExit("cloud-judge Development egress policy drift")
    evaluation = report.get("evaluation")
    diagnostics = report.get("diagnostics")
    authority = report.get("costAuthority")
    if (
        not isinstance(evaluation, dict)
        or not isinstance(report.get("passed"), bool)
        or report.get("passed") != evaluation.get("passed")
    ):
        raise SystemExit("cloud-judge Development result drift")
    if not isinstance(diagnostics, dict) or not isinstance(diagnostics.get("failureCodeCounts"), dict):
        raise SystemExit("cloud-judge Development diagnostics missing")
    if sum(
        diagnostics.get(name, -1)
        for name in ("emptyCandidateCaseCount", "judgeCompletedCaseCount", "failedCaseCount")
    ) != 300:
        raise SystemExit("cloud-judge Development diagnostic count drift")
    if not isinstance(authority, dict):
        raise SystemExit("cloud-judge Development cost authority missing")
    actual_requests = authority.get("actualRequestCount")
    if (
        not isinstance(actual_requests, int)
        or actual_requests < 0
        or actual_requests > authority.get("authorizedRequestCount", -1)
        or authority.get("actualInputTokenUpperBound", -1) > authority.get("authorizedMaximumInputTokens", -1)
        or authority.get("actualOutputTokenUpperBound") != actual_requests * 128
        or authority.get("actualOutputTokenUpperBound", -1) > authority.get("authorizedMaximumOutputTokens", -1)
    ):
        raise SystemExit("cloud-judge Development cost authority drift")
    if schema.endswith(".v5"):
        if (
            report.get("providerCostPolicy") != "owner_authorized_absolute_cap_v1"
            or report.get("providerCostAuthorized") is not True
            or "providerCostPassed" in evaluation
            or not authority.get("unit")
            or authority.get("maximumMemoryProviderCostMicrounits", 0)
            < authority.get("maximumJudgeCostMicrounits", 0)
            or manifest.get("providerCostPolicy") != report.get("providerCostPolicy")
        ):
            raise SystemExit("cloud-judge Development owner budget drift")
    elif (
        report.get("providerCostPolicy") is not None
        or manifest.get("providerCostPolicy") is not None
        or not isinstance(evaluation.get("providerCostPassed"), bool)
    ):
        raise SystemExit("legacy cloud-judge Development gained owner budget authority")
elif capture_mode == "development_fixed_memory_judge_accuracy":
    report = json.loads(
        (output / "fixed-memory-judge-accuracy-development.json").read_text(
            encoding="utf-8"
        )
    )
    criteria = report.get("evaluationCriteria")
    execution = report.get("executionPolicy")
    evaluation = report.get("evaluation")
    diagnostics = report.get("diagnostics")
    attempts = report.get("providerAttempts")
    authority = report.get("costAuthority")
    expected_clock = "wall_clock_v1" if mode == "live_siliconflow" else "virtual_protocol_v1"
    if (
        report.get("schemaVersion") != "neo-chat.memory-regression-relevance-calibration.v12"
        or report.get("split") != "development"
        or report.get("caseCount") != 300
        or report.get("admissionMode") != expected_admission
        or report.get("promotionEligible") is not False
        or report.get("providerEgressPolicy") != "owner_authorized_normal_candidates_v1"
        or report.get("providerCostPolicy") != "owner_authorized_absolute_cap_v1"
        or report.get("providerCostAuthorized") is not True
        or report.get("judgeProviderId") != "SERVER_DEFAULT"
        or report.get("judgeProviderType") != "openai_compatible"
        or report.get("judgeBaseUrlSha256") != "3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671"
        or report.get("judgeModelId") != "gpt-5.6-luna"
        or report.get("judgeAdapter") != "chat-configured-candidate-judge-v1"
        or report.get("policyId") != "memory_hybrid_fixed_cloud_candidate_judge_accuracy_development_v2"
        or report.get("evaluationCriteriaVersion") != "neo-chat.memory-benchmark-criteria.v3"
        or not isinstance(criteria, dict)
        or criteria.get("latencyEvaluationMode") != "diagnostic_only_v1"
        or criteria.get("applicationDeadlineMode") != "none_v1"
        or any(
            name in criteria
            for name in (
                "maximumP95LatencyMilliseconds",
                "maximumP99LatencyMilliseconds",
                "hardCutoffMilliseconds",
            )
        )
    ):
        raise SystemExit("accuracy-first Memory Judge authority/criteria drift")
    if (
        not isinstance(execution, dict)
        or execution.get("sequenceVersion") != "bge_query_admission_bge_rerank_luna_judge_record_serial_v1"
        or execution.get("globalProviderRequestConcurrency") != 1
        or execution.get("applicationDeadlineMode") != "none_v1"
        or execution.get("providerElapsedTimeoutMode") != "none_v1"
        or execution.get("latencyEvaluationMode") != "diagnostic_only_v1"
        or execution.get("interCaseCooldownMilliseconds") != 1000
        or execution.get("interCaseCooldownClock") != expected_clock
        or execution.get("retryPolicyVersion") != "transient_408_429_5xx_transport_read_once_v1"
        or execution.get("maximumRetriesPerProviderRequest") != 1
        or execution.get("retryFallbackDelayMilliseconds") != 5000
    ):
        raise SystemExit("accuracy-first Memory Judge execution policy drift")
    if (
        not isinstance(evaluation, dict)
        or not isinstance(report.get("passed"), bool)
        or report.get("passed") != evaluation.get("passed")
        or any(
            name in json.dumps(evaluation, sort_keys=True)
            for name in (
                "latencyPassed",
                "hardCutoffPassed",
                "hardCutoffViolationCount",
            )
        )
        or not isinstance(diagnostics, dict)
        or not isinstance(diagnostics.get("failureCodeCounts"), dict)
        or diagnostics.get("failureCodeCounts", {}).get("HARD_CUTOFF", 0) != 0
        or sum(
            diagnostics.get(name, -1)
            for name in ("emptyCandidateCaseCount", "judgeCompletedCaseCount", "failedCaseCount")
        ) != 300
        or not isinstance(attempts, dict)
        or not isinstance(authority, dict)
    ):
        raise SystemExit("accuracy-first Memory Judge result drift")
    count_pairs = (
        ("passageEmbeddingAttempts", "passageEmbeddingRetries", "passageEmbeddingLatency"),
        ("queryEmbeddingAttempts", "queryEmbeddingRetries", "queryEmbeddingLatency"),
        ("rerankAttempts", "rerankRetries", "rerankLatency"),
        ("judgeAttempts", "judgeRetries", "judgeLatency"),
    )
    for attempt_name, retry_name, latency_name in count_pairs:
        attempt_count = attempts.get(attempt_name)
        retry_count = attempts.get(retry_name)
        latency = attempts.get(latency_name)
        if (
            type(attempt_count) is not int
            or type(retry_count) is not int
            or attempt_count < 0
            or retry_count < 0
            or retry_count > attempt_count
            or not isinstance(latency, dict)
            or latency.get("sampleCount") != attempt_count
            or type(latency.get("totalMilliseconds")) is not int
            or type(latency.get("p95LatencyMilliseconds")) is not int
            or type(latency.get("p99LatencyMilliseconds")) is not int
            or type(latency.get("maximumLatencyMilliseconds")) is not int
            or latency.get("totalMilliseconds") < latency.get("maximumLatencyMilliseconds")
            or latency.get("p95LatencyMilliseconds") > latency.get("p99LatencyMilliseconds")
            or latency.get("p99LatencyMilliseconds") > latency.get("maximumLatencyMilliseconds")
        ):
            raise SystemExit("accuracy-first Provider attempt telemetry drift")
    if (
        attempts.get("passageEmbeddingAttempts", 0) <= 0
        or attempts.get("queryEmbeddingAttempts") != 300 + attempts.get("queryEmbeddingRetries")
        or attempts.get("interCaseCooldownCount") != 299
        or attempts.get("interCaseCooldownMilliseconds") != 299000
        or type(attempts.get("interCaseCooldownElapsedMilliseconds")) is not int
        or attempts.get("interCaseCooldownElapsedMilliseconds") < 0
        or (mode == "fake_protocol" and attempts.get("interCaseCooldownElapsedMilliseconds") != 0)
        or type(attempts.get("judgeInputTokenUpperBound")) is not int
        or attempts.get("judgeInputTokenUpperBound") <= 0
        or type(attempts.get("judgeRetryInputTokenUpperBound")) is not int
        or attempts.get("judgeRetryInputTokenUpperBound") < 0
        or attempts.get("judgeRetryInputTokenUpperBound")
        > attempts.get("judgeInputTokenUpperBound")
        or authority.get("authorizedRequestCount") != 600
        or authority.get("actualRequestCount") != attempts.get("judgeAttempts")
        or authority.get("actualRequestCount", -1) > 600
        or authority.get("actualInputTokenUpperBound")
        != attempts.get("judgeInputTokenUpperBound")
        or authority.get("actualInputTokenUpperBound", -1) > authority.get("authorizedMaximumInputTokens", -1)
        or authority.get("actualOutputTokenUpperBound") != authority.get("actualRequestCount") * 128
        or authority.get("actualOutputTokenUpperBound", -1) > authority.get("authorizedMaximumOutputTokens", -1)
        or authority.get("maximumMemoryProviderCostMicrounits", 0)
        < authority.get("maximumJudgeCostMicrounits", 0)
        or manifest.get("providerCostPolicy") != report.get("providerCostPolicy")
    ):
        raise SystemExit("accuracy-first Memory Judge retry/cost authority drift")
elif capture_mode in {
    "development_fixed_memory_judge_failure_diagnostic",
    "development_fixed_memory_judge_transport_stable",
}:
    transport_stable = capture_mode == "development_fixed_memory_judge_transport_stable"
    report_name = (
        "fixed-memory-judge-transport-stable-development.json"
        if transport_stable
        else "fixed-memory-judge-failure-diagnostic-development.json"
    )
    report = json.loads(
        (output / report_name).read_text(encoding="utf-8")
    )
    criteria = report.get("evaluationCriteria")
    execution = report.get("executionPolicy")
    evaluation = report.get("evaluation")
    diagnostics = report.get("diagnostics")
    attempts = report.get("providerAttempts")
    authority = report.get("costAuthority")
    expected_clock = "wall_clock_v1" if mode == "live_siliconflow" else "virtual_protocol_v1"
    expected_schema = (
        "neo-chat.memory-regression-relevance-calibration.v14"
        if transport_stable
        else "neo-chat.memory-regression-relevance-calibration.v13"
    )
    expected_passed = bool(evaluation.get("passed")) and diagnostics.get("failedCaseCount") == 0
    if (
        report.get("schemaVersion") != expected_schema
        or report.get("split") != "development"
        or report.get("caseCount") != 300
        or report.get("admissionMode") != expected_admission
        or report.get("promotionEligible") is not False
        or report.get("policySelected") is not False
        or report.get("diagnosticComplete") is not True
        or (not transport_stable and report.get("passed") is not False)
        or (transport_stable and report.get("passed") is not expected_passed)
        or manifest.get("passed") is not report.get("passed")
        or report.get("providerEgressPolicy") != "owner_authorized_normal_candidates_v1"
        or report.get("providerCostPolicy") != "owner_authorized_absolute_cap_v1"
        or report.get("providerCostAuthorized") is not True
        or report.get("judgeProviderId") != "SERVER_DEFAULT"
        or report.get("judgeProviderType") != "openai_compatible"
        or report.get("judgeBaseUrlSha256") != "3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671"
        or report.get("judgeModelId") != "gpt-5.6-luna"
        or report.get("judgeAdapter") != "chat-configured-candidate-judge-v1"
        or report.get("policyId") != "memory_hybrid_fixed_cloud_candidate_judge_accuracy_development_v2"
        or report.get("evaluationCriteriaVersion") != "neo-chat.memory-benchmark-criteria.v3"
        or report.get("failureTaxonomyVersion") != "memory-candidate-judge-failure-taxonomy-v1"
        or report.get("failureTaxonomySha256") != "c22cb137da8b5fda87526237446519dd9abe2c8d221ad703c5445358d9059f8d"
        or report.get("diagnosticCompleteness") != "attempt_terminal_reconciled_fail_closed_v1"
        or not isinstance(criteria, dict)
        or criteria.get("latencyEvaluationMode") != "diagnostic_only_v1"
        or criteria.get("applicationDeadlineMode") != "none_v1"
        or not isinstance(evaluation, dict)
        or not isinstance(diagnostics, dict)
        or not isinstance(attempts, dict)
        or not isinstance(authority, dict)
    ):
        raise SystemExit("Memory Judge typed transport authority drift")
    expected_sequence = (
        "bge_query_admission_bge_rerank_luna_judge_record_serial_judge_retry_v2"
        if transport_stable
        else "bge_query_admission_bge_rerank_luna_judge_record_serial_v1"
    )
    expected_retry_policy = (
        "transient_408_429_5xx_transport_read_judge_twice_v2"
        if transport_stable
        else "transient_408_429_5xx_transport_read_once_v1"
    )
    if (
        not isinstance(execution, dict)
        or execution.get("sequenceVersion") != expected_sequence
        or execution.get("globalProviderRequestConcurrency") != 1
        or execution.get("applicationDeadlineMode") != "none_v1"
        or execution.get("providerElapsedTimeoutMode") != "none_v1"
        or execution.get("latencyEvaluationMode") != "diagnostic_only_v1"
        or execution.get("interCaseCooldownMilliseconds") != 1000
        or execution.get("interCaseCooldownClock") != expected_clock
        or execution.get("retryPolicyVersion") != expected_retry_policy
        or execution.get("maximumRetriesPerProviderRequest") != 1
        or execution.get("retryFallbackDelayMilliseconds") != 5000
        or execution.get("maximumJudgeRetriesPerRequest") != (2 if transport_stable else None)
        or execution.get("secondJudgeRetryDelayMilliseconds") != (10000 if transport_stable else None)
    ):
        raise SystemExit("Memory Judge typed transport execution policy drift")
    failure_codes = diagnostics.get("failureCodeCounts")
    attempt_counts = attempts.get("judgeAttemptFailureCategoryCounts")
    terminal_counts = diagnostics.get("judgeTerminalFailureCategoryCounts")
    categories = {
        "CANDIDATE_JUDGE_EVENT_INVALID",
        "CANDIDATE_JUDGE_FAILURE_UNCLASSIFIED",
        "CANDIDATE_JUDGE_INPUT_INVALID",
        "CANDIDATE_JUDGE_OUTPUT_JSON_INVALID",
        "CANDIDATE_JUDGE_OUTPUT_ORDINAL_INVALID",
        "CANDIDATE_JUDGE_OUTPUT_SCHEMA_INVALID",
        "CANDIDATE_JUDGE_OUTPUT_TOO_LARGE",
        "CANDIDATE_JUDGE_PROVENANCE_DRIFT",
        "CANDIDATE_JUDGE_RECORDER_STATE_CONFLICT",
        "CONTEXT_CANCELED",
        "CONTEXT_DEADLINE",
        "PROVIDER_AUTHENTICATION_FAILED",
        "PROVIDER_QUOTA_EXHAUSTED",
        "PROVIDER_RATE_LIMITED",
        "PROVIDER_REQUEST_BUILD_FAILED",
        "PROVIDER_REQUEST_REJECTED",
        "PROVIDER_REQUEST_TIMEOUT",
        "PROVIDER_RESPONSE_INVALID",
        "PROVIDER_STREAM_INCOMPLETE",
        "PROVIDER_STREAM_PARSE_FAILED",
        "PROVIDER_STREAM_READ_FAILED",
        "PROVIDER_STREAM_REMOTE_ERROR",
        "PROVIDER_TRANSPORT_FAILED",
        "PROVIDER_UPSTREAM_FAILED",
    }
    capture_local = {
        "CANDIDATE_JUDGE_PROVENANCE_DRIFT",
        "CANDIDATE_JUDGE_RECORDER_STATE_CONFLICT",
    }
    taxonomy_sha256 = hashlib.sha256(
        json.dumps(sorted(categories), separators=(",", ":")).encode()
    ).hexdigest()
    if (
        not isinstance(failure_codes, dict)
        or not isinstance(attempt_counts, dict)
        or not isinstance(terminal_counts, dict)
        or taxonomy_sha256 != report.get("failureTaxonomySha256")
        or any(key not in categories or type(value) is not int or value <= 0 for key, value in attempt_counts.items())
        or any(key not in categories or type(value) is not int or value <= 0 for key, value in terminal_counts.items())
        or any(key in capture_local for key in attempt_counts)
        or sum(terminal_counts.values()) != failure_codes.get("CANDIDATE_JUDGE_FAILED", 0)
        or sum(attempt_counts.values())
        != attempts.get("judgeRetries", -1)
        + sum(value for key, value in terminal_counts.items() if key not in capture_local)
        or sum(
            diagnostics.get(name, -1)
            for name in ("emptyCandidateCaseCount", "judgeCompletedCaseCount", "failedCaseCount")
        ) != 300
        or failure_codes.get("HARD_CUTOFF", 0) != 0
    ):
        raise SystemExit("Memory Judge typed transport category reconciliation drift")
    count_pairs = (
        ("passageEmbeddingAttempts", "passageEmbeddingRetries", "passageEmbeddingLatency"),
        ("queryEmbeddingAttempts", "queryEmbeddingRetries", "queryEmbeddingLatency"),
        ("rerankAttempts", "rerankRetries", "rerankLatency"),
        ("judgeAttempts", "judgeRetries", "judgeLatency"),
    )
    for attempt_name, retry_name, latency_name in count_pairs:
        attempt_count = attempts.get(attempt_name)
        retry_count = attempts.get(retry_name)
        latency = attempts.get(latency_name)
        if (
            type(attempt_count) is not int
            or type(retry_count) is not int
            or attempt_count < 0
            or retry_count < 0
            or retry_count > attempt_count
            or not isinstance(latency, dict)
            or latency.get("sampleCount") != attempt_count
        ):
            raise SystemExit("Memory Judge typed transport Provider telemetry drift")
    logical_judges = diagnostics.get("judgeCompletedCaseCount", -1) + failure_codes.get(
        "CANDIDATE_JUDGE_FAILED", 0
    )
    authorized_requests = 900 if transport_stable else 600
    maximum_judge_retries = 2 if transport_stable else 1
    if (
        attempts.get("judgeAttempts") != logical_judges + attempts.get("judgeRetries", -1)
        or attempts.get("judgeRetries", -1) > logical_judges * maximum_judge_retries
        or attempts.get("queryEmbeddingAttempts") != 300 + attempts.get("queryEmbeddingRetries", -1)
        or attempts.get("interCaseCooldownCount") != 299
        or attempts.get("interCaseCooldownMilliseconds") != 299000
        or type(attempts.get("interCaseCooldownElapsedMilliseconds")) is not int
        or (mode == "fake_protocol" and attempts.get("interCaseCooldownElapsedMilliseconds") != 0)
        or authority.get("authorizedRequestCount") != authorized_requests
        or authority.get("actualRequestCount") != attempts.get("judgeAttempts")
        or authority.get("actualInputTokenUpperBound") != attempts.get("judgeInputTokenUpperBound")
        or authority.get("actualOutputTokenUpperBound") != attempts.get("judgeAttempts") * 128
        or authority.get("actualInputTokenUpperBound", -1) > authority.get("authorizedMaximumInputTokens", -1)
        or authority.get("actualOutputTokenUpperBound", -1) > authority.get("authorizedMaximumOutputTokens", -1)
        or authority.get("maximumMemoryProviderCostMicrounits", 0)
        < authority.get("maximumJudgeCostMicrounits", 0)
        or manifest.get("providerCostPolicy") != report.get("providerCostPolicy")
    ):
        raise SystemExit("Memory Judge typed transport cost/attempt drift")
elif capture_mode in {"development_configured_candidate_judge", "development_fixed_memory_judge"}:
    fixed_memory_judge = capture_mode == "development_fixed_memory_judge"
    report_name = (
        "fixed-memory-judge-development.json"
        if fixed_memory_judge
        else "configured-candidate-judge-development.json"
    )
    report = json.loads((output / report_name).read_text(encoding="utf-8"))
    expected_schema = (
        "neo-chat.memory-regression-relevance-calibration.v11"
        if fixed_memory_judge
        else "neo-chat.memory-regression-relevance-calibration.v10"
    )
    if (
        report.get("schemaVersion") != expected_schema
        or report.get("split") != "development"
        or report.get("caseCount") != 300
        or report.get("admissionMode") != expected_admission
        or report.get("promotionEligible") is not False
        or report.get("providerEgressPolicy") != "owner_authorized_normal_candidates_v1"
        or report.get("providerCostPolicy") != "owner_authorized_absolute_cap_v1"
        or report.get("providerCostAuthorized") is not True
        or report.get("judgeProviderId") != configured_judge_provider_id
        or report.get("judgeProviderType") != configured_judge_provider_type
        or report.get("judgeBaseUrlSha256") != configured_judge_base_url_sha256
        or report.get("judgeModelId") != configured_judge_model
        or report.get("judgeAdapter") != "chat-configured-candidate-judge-v1"
    ):
        raise SystemExit("configured candidate-judge Development authority drift")
    if fixed_memory_judge:
        criteria = report.get("evaluationCriteria")
        if (
            configured_judge_provider_id != "SERVER_DEFAULT"
            or configured_judge_provider_type != "openai_compatible"
            or configured_judge_base_url_sha256 != "3bc0bbf28d9d817b4f6c8f6058c2c51dd644c541252ed6e2542a8c8a472ff671"
            or configured_judge_model != "gpt-5.6-luna"
            or report.get("policyId") != "memory_hybrid_fixed_cloud_candidate_judge_development_v1"
            or report.get("evaluationCriteriaVersion") != "neo-chat.memory-benchmark-criteria.v2"
            or not isinstance(criteria, dict)
            or criteria.get("maximumP95LatencyMilliseconds") != 1500
            or criteria.get("maximumP99LatencyMilliseconds") != 2500
            or criteria.get("hardCutoffMilliseconds") != 3000
        ):
            raise SystemExit("fixed Memory Judge schema-v11 criteria/authority drift")
    evaluation = report.get("evaluation")
    diagnostics = report.get("diagnostics")
    authority = report.get("costAuthority")
    if (
        not isinstance(evaluation, dict)
        or not isinstance(report.get("passed"), bool)
        or report.get("passed") != evaluation.get("passed")
        or not isinstance(diagnostics, dict)
        or not isinstance(diagnostics.get("failureCodeCounts"), dict)
        or sum(
            diagnostics.get(name, -1)
            for name in ("emptyCandidateCaseCount", "judgeCompletedCaseCount", "failedCaseCount")
        ) != 300
        or not isinstance(authority, dict)
    ):
        raise SystemExit("configured candidate-judge Development result drift")
    actual_requests = authority.get("actualRequestCount")
    if (
        not isinstance(actual_requests, int)
        or actual_requests < 0
        or actual_requests > authority.get("authorizedRequestCount", -1)
        or authority.get("actualInputTokenUpperBound", -1) > authority.get("authorizedMaximumInputTokens", -1)
        or authority.get("actualOutputTokenUpperBound") != actual_requests * 128
        or authority.get("actualOutputTokenUpperBound", -1) > authority.get("authorizedMaximumOutputTokens", -1)
        or authority.get("maximumMemoryProviderCostMicrounits", 0)
        < authority.get("maximumJudgeCostMicrounits", 0)
    ):
        raise SystemExit("configured candidate-judge Development cost authority drift")
elif capture_mode in {"development_memory_tool_route", "development_memory_tool_route_diagnostic"}:
    diagnostic = capture_mode == "development_memory_tool_route_diagnostic"
    report_name = (
        "memory-first-tool-round-route-diagnostic-development.json"
        if diagnostic
        else "memory-first-tool-round-development.json"
    )
    report = json.loads((output / report_name).read_text(encoding="utf-8"))
    expected_schema = (
        "neo-chat.memory-regression-relevance-calibration.v9"
        if diagnostic
        else "neo-chat.memory-regression-relevance-calibration.v7"
    )
    if report.get("schemaVersion") != expected_schema:
        raise SystemExit("invalid Memory Tool-route Development report schema")
    if report.get("split") != "development" or report.get("caseCount") != 300:
        raise SystemExit("Memory Tool-route Development split drift")
    if report.get("admissionMode") != expected_admission or report.get("promotionEligible") is not False:
        raise SystemExit("Memory Tool-route Development report gained promotion authority")
    if (
        report.get("routeProviderId") != memory_tool_route_provider_id
        or report.get("routeProviderType") != memory_tool_route_provider_type
        or report.get("routeBaseUrlSha256") != memory_tool_route_base_url_sha256
        or report.get("routeModelId") != memory_tool_route_model
    ):
        raise SystemExit("Memory Tool-route Provider/model authority drift")
    if (
        report.get("toolName") != "search_memory"
        or report.get("toolContractVersion") != "memory-search-tool-v1"
        or report.get("toolContractSha256") != "f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6"
        or report.get("toolAdapterVersion") != "chat-first-tool-round-memory-decision-v1"
        or "toolDecodingProfile" in report
        or "toolMaximumOutputTokens" in report
        or "toolTemperature" in report
        or "toolDisableThinking" in report
        or report.get("selectionAlgorithm") != "first-tool-round-call_then-bge-order_top5-token-budget_v1"
        or report.get("providerEgressPolicy") != "owner_authorized_normal_candidates_v1"
        or report.get("providerCostPolicy") != "owner_authorized_absolute_cap_v1"
        or report.get("providerCostAuthorized") is not True
        or manifest.get("providerCostPolicy") != report.get("providerCostPolicy")
    ):
        raise SystemExit("Memory Tool-route contract/cost authority drift")
    evaluation = report.get("evaluation")
    diagnostics = report.get("diagnostics")
    authority = report.get("costAuthority")
    if (
        not isinstance(evaluation, dict)
        or not isinstance(report.get("passed"), bool)
        or report.get("passed") != evaluation.get("passed")
    ):
        raise SystemExit("Memory Tool-route Development result drift")
    if not isinstance(diagnostics, dict) or not isinstance(diagnostics.get("failureCodeCounts"), dict):
        raise SystemExit("Memory Tool-route Development diagnostics missing")
    if sum(
        diagnostics.get(name, -1)
        for name in ("routeCompletedCaseCount", "failedCaseCount")
    ) != 300 or sum(
        diagnostics.get(name, -1)
        for name in ("routeUsedCaseCount", "routeAbstainedCaseCount")
    ) != diagnostics.get("routeCompletedCaseCount"):
        raise SystemExit("Memory Tool-route Development diagnostic count drift")
    route_failure_counts = diagnostics.get("routeFailureCategoryCounts")
    if diagnostic:
        retrieval_failure_counts = diagnostics.get("retrievalFailureCodeCounts")
        retrieval_incomplete_count = diagnostics.get("retrievalIncompleteCaseCount")
        if (
            report.get("failureTaxonomyVersion") != "memory-tool-route-failure-taxonomy-v1"
            or report.get("failureTaxonomySha256") != "66f11e91edc0cf5a6a9dbf5dd30336e58a52860adee968fb4658d6ccd70d52a0"
            or report.get("diagnosticCompleteness") != "route_complete_retrieval_fail_closed_v1"
            or not isinstance(route_failure_counts, dict)
            or any(type(value) is not int or value < 0 for value in route_failure_counts.values())
            or sum(route_failure_counts.values()) != diagnostics.get("failedCaseCount")
            or not isinstance(retrieval_failure_counts, dict)
            or any(type(value) is not int or value < 0 for value in retrieval_failure_counts.values())
            or type(retrieval_incomplete_count) is not int
            or retrieval_incomplete_count < 0
            or retrieval_incomplete_count > report.get("caseCount", -1)
            or sum(retrieval_failure_counts.values()) != retrieval_incomplete_count
        ):
            raise SystemExit("Memory Tool-route failure taxonomy drift")
    elif (
        "failureTaxonomyVersion" in report
        or "failureTaxonomySha256" in report
        or "diagnosticCompleteness" in report
        or route_failure_counts is not None
        or "retrievalFailureCodeCounts" in diagnostics
        or "retrievalIncompleteCaseCount" in diagnostics
    ):
        raise SystemExit("schema-v7 gained diagnostic fields")
    if not isinstance(authority, dict):
        raise SystemExit("Memory Tool-route Development cost authority missing")
    actual_requests = authority.get("actualRequestCount")
    if (
        not isinstance(actual_requests, int)
        or actual_requests < 0
        or actual_requests > authority.get("authorizedRequestCount", -1)
        or authority.get("actualInputTokenUpperBound", -1) > authority.get("authorizedMaximumInputTokens", -1)
        or authority.get("actualOutputTokenUpperBound", 0) <= 0
        or authority.get("actualOutputTokenUpperBound", -1) > authority.get("authorizedMaximumOutputTokens", -1)
        or authority.get("maximumMemoryProviderCostMicrounits", 0)
        < authority.get("maximumRouteCostMicrounits", 0)
    ):
        raise SystemExit("Memory Tool-route Development cost authority drift")
elif capture_mode == "frozen_validation":
    report = json.loads((output / "relevance-validation.json").read_text(encoding="utf-8"))
    if report.get("schemaVersion") != "neo-chat.memory-regression-relevance-validation.v1":
        raise SystemExit("invalid validation report schema")
    if report.get("split") != "validation" or report.get("caseCount") != 100:
        raise SystemExit("validation split drift")
    if report.get("admissionMode") != expected_admission or report.get("promotionEligible") is not False:
        raise SystemExit("validation report gained promotion authority")

PY
validation_status=$?
set -e
if [[ ${validation_status} -ne 0 ]]; then
  if [[ -f "${leak_free_marker}" ]]; then
    cat "${capture_stdout}"
    cat "${capture_stderr}" >&2
  fi
  exit "${validation_status}"
fi

retain_output=true
cat "${capture_stdout}"
cat "${capture_stderr}" >&2
if [[ ${runner_status} -eq 0 ]]; then
  echo "Memory regression: retained validated bundle ${run_output}"
else
  echo "Memory regression: retained failed-gate evidence ${run_output}" >&2
fi
exit "${runner_status}"
