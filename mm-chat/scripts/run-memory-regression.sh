#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
usage: run-memory-regression.sh \
  --cost-basis <versioned-json> \
  --output-dir <new-run-parent> \
  [--regression-root <protected-root>] \
  [--provider-mode fake_protocol|live_siliconflow] \
  [--credential-file <mode-0600-file>] \
  [--live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA]

Run the production v1 lexical and native v2 hybrid Memory readers against the
protected 500-case machine regression corpus in a random isolated Compose
project. Fake protocol mode has an internal-only network and is not reader-
quality evidence. Live mode requires a fresh SiliconFlow key file and exact
quota approval. Runtime objects and temporary credentials are always removed.
EOF
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
compose_file="${project_dir}/compose.memory-regression.yml"
regression_root="${project_dir}/data/memory-benchmark/v2-regression"
provider_mode="fake_protocol"
cost_basis=""
output_parent=""
credential_source=""
live_approval=""

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
capture_stdout="${temp_dir}/capture.stdout"
capture_stderr="${temp_dir}/capture.stderr"
metadata_snapshot="${temp_dir}/docker-metadata.json"
run_suffix="$(date -u +%Y%m%dT%H%M%SZ)-$(openssl rand -hex 4)"
run_id="memory-regression-${run_suffix,,}"
project_name="mmchat-memory-regression-${run_suffix,,}"
run_output="${output_parent}/${run_suffix}"
retain_output=false
cleanup_failed=false

early_cleanup() {
  local status=$?
  trap - EXIT INT TERM HUP
  set +e
  if [[ "${retain_output}" != true && -d "${run_output}" ]]; then
    find "${run_output}" -depth -delete
  fi
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

db_password="$(openssl rand -hex 32)"
cat >"${env_file}" <<EOF
MEMORY_REGRESSION_DB_PASSWORD=${db_password}
MEMORY_REGRESSION_ROOT_PATH=$(docker_path "${regression_root}")
MEMORY_REGRESSION_COST_BASIS_PATH=$(docker_path "${cost_copy}")
MEMORY_REGRESSION_CREDENTIAL_PATH=$(docker_path "${credential_copy}")
MEMORY_REGRESSION_OUTPUT_PATH=$(docker_path "${run_output}")
MEMORY_REGRESSION_RUN_ID=${run_id}
MEMORY_REGRESSION_LIVE_APPROVAL=${live_approval:-NOT_AUTHORIZED}
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

python3 - \
  "${run_output}" \
  "${regression_root}" \
  "${credential_copy}" \
  "${capture_stdout}" \
  "${capture_stderr}" \
  "${metadata_snapshot}" \
  "${provider_mode}" \
  "${candidate_prefix}" <<'PY'
import json
import hashlib
import sys
from pathlib import Path

output = Path(sys.argv[1])
root = Path(sys.argv[2])
credential_path = Path(sys.argv[3])
stdout_path = Path(sys.argv[4])
stderr_path = Path(sys.argv[5])
metadata_path = Path(sys.argv[6])
mode = sys.argv[7]
candidate_prefix = sys.argv[8]

expected = {
    "native-v1-lexical.observations.json",
    "native-v1-lexical.report.json",
    f"{candidate_prefix}.observations.json",
    f"{candidate_prefix}.report.json",
    "run-manifest.json",
}
actual = {path.name for path in output.iterdir() if path.is_file()}
if actual != expected:
    raise SystemExit(f"artifact set drift: {sorted(actual)}")
for path in output.iterdir():
    if path.is_symlink() or not path.is_file() or (path.stat().st_mode & 0o777) != 0o600:
        raise SystemExit("artifact permission/type drift")

manifest = json.loads((output / "run-manifest.json").read_text(encoding="utf-8"))
if manifest.get("schemaVersion") != "neo-chat.memory-regression-native-run.v1":
    raise SystemExit("invalid native run manifest schema")
if manifest.get("corpusClass") != "machine_reviewed_regression":
    raise SystemExit("native run corpus class drift")
if manifest.get("admissionMode") != "regression_only" or manifest.get("promotionEligible") is not False:
    raise SystemExit("native run gained promotion authority")
if manifest.get("providerMode") != mode:
    raise SystemExit("native run Provider mode drift")
profiles = manifest.get("profiles", [])
candidate_profile = "native_v2_hybrid" if mode == "live_siliconflow" else "native_v2_hybrid_fake_protocol"
if [(item.get("role"), item.get("profileId")) for item in profiles] != [
    ("baseline", "native_v1_lexical"),
    ("candidate", candidate_profile),
]:
    raise SystemExit("native run profile authority drift")

manifest_artifacts = manifest.get("artifacts", [])
if {item.get("name") for item in manifest_artifacts} != expected - {"run-manifest.json"}:
    raise SystemExit("native run artifact manifest drift")
for item in manifest_artifacts:
    body = (output / item["name"]).read_bytes()
    if item.get("bytes") != len(body) or item.get("sha256") != hashlib.sha256(body).hexdigest():
        raise SystemExit("native run artifact hash drift")

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

retained_and_logs = [path.read_bytes() for path in output.iterdir()]
retained_and_logs += [stdout_path.read_bytes(), stderr_path.read_bytes(), metadata_path.read_bytes()]
for body in retained_and_logs:
    if any(value in body for value in forbidden):
        raise SystemExit("protected plaintext or credential leaked into retained output/log/metadata")
PY

retain_output=true
cat "${capture_stdout}"
cat "${capture_stderr}" >&2
if [[ ${runner_status} -eq 0 ]]; then
  echo "Memory regression: retained validated bundle ${run_output}"
else
  echo "Memory regression: retained failed-gate evidence ${run_output}" >&2
fi
exit "${runner_status}"
