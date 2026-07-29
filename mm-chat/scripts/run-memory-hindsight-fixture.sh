#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
usage: run-memory-hindsight-fixture.sh [--output-dir <directory>]

Run both synthetic-only Hindsight fixture profiles in a random, isolated
Compose project. The Hindsight database, role, key, containers, network, and
volume are destroyed on success, failure, or signal. Only content-free reports
are retained.
EOF
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
compose_file="${project_dir}/compose.hindsight-fixture.yml"
manifest_file="${project_dir}/docs/contracts/memory-hindsight-fixture-draft.json"
golden_file="${project_dir}/docs/contracts/memory-benchmark-golden-draft-template.json"
output_dir="${project_dir}/docs/tracking/hindsight-fixture-reports"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output-dir)
      [[ $# -ge 2 ]] || { usage >&2; exit 2; }
      output_dir="$2"
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

output_dir="$(realpath -m "${output_dir}")"
case "${output_dir}" in
  "${project_dir}/data"|"${project_dir}/data/"*|"${project_dir}/secrets"|"${project_dir}/secrets/"*|"${project_dir}/backup"|"${project_dir}/backup/"*)
    echo "Hindsight fixture: report output cannot enter protected runtime state" >&2
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
    echo "Hindsight fixture: Docker Compose is required" >&2
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

for required in "${compose_file}" "${manifest_file}" "${golden_file}"; do
  if [[ ! -f "${required}" ]]; then
    echo "Hindsight fixture: required checked-in input is missing" >&2
    exit 1
  fi
done

umask 077
temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/mm-chat-hindsight-fixture.XXXXXX")"
env_file="${temp_dir}/fixture.env"
run_suffix="$(date -u +%Y%m%dT%H%M%SZ)-$(openssl rand -hex 4)"
project_name="mmchat-hindsight-fixture-${run_suffix,,}"
db_password="$(openssl rand -hex 32)"
api_key="$(openssl rand -hex 32)"
pending_reports=()
cleanup_failed=false

cat >"${env_file}" <<EOF
HINDSIGHT_FIXTURE_DB_PASSWORD=${db_password}
HINDSIGHT_FIXTURE_API_KEY=${api_key}
EOF
chmod 600 "${env_file}"
unset db_password api_key

compose=(
  "${docker_bin}" compose
  --project-name "${project_name}"
  --project-directory "$(docker_path "${project_dir}")"
  --env-file "$(docker_path "${env_file}")"
  -f "$(docker_path "${compose_file}")"
  --profile memory-hindsight-fixture
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
  if ! containers="$("${docker_bin}" ps -aq --filter "label=com.docker.compose.project=${project_name}" 2>/dev/null)"; then
    cleanup_failed=true
  fi
  if ! networks="$("${docker_bin}" network ls -q --filter "label=com.docker.compose.project=${project_name}" 2>/dev/null)"; then
    cleanup_failed=true
  fi
  if ! volumes="$("${docker_bin}" volume ls -q --filter "label=com.docker.compose.project=${project_name}" 2>/dev/null)"; then
    cleanup_failed=true
  fi
  if [[ -n "${containers}" || -n "${networks}" || -n "${volumes}" ]]; then
    echo "Hindsight fixture: scoped teardown left project runtime objects" >&2
    cleanup_failed=true
  fi

  for pending in "${pending_reports[@]:-}"; do
    if [[ -n "${pending}" && -f "${pending}" ]]; then
      python3 - "${pending}" <<'PY'
import os
import sys
os.unlink(sys.argv[1])
PY
    fi
  done
  if [[ -d "${temp_dir}" ]]; then
    find "${temp_dir}" -depth -delete
  fi
  if [[ -e "${temp_dir}" ]]; then
    echo "Hindsight fixture: credential temporary directory still exists" >&2
    cleanup_failed=true
  fi

  if [[ "${cleanup_failed}" == true && ${status} -eq 0 ]]; then
    status=1
  fi
  if [[ "${cleanup_failed}" == false ]]; then
    echo "Hindsight fixture: isolated project runtime destroyed (${project_name})"
  fi
  exit "${status}"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

mkdir -p "${output_dir}"
chmod 750 "${output_dir}"

"${compose[@]}" config --quiet
"${compose[@]}" pull hindsight-postgres hindsight-api
"${compose[@]}" build hindsight-runner
"${compose[@]}" up -d --wait --wait-timeout 600 hindsight-postgres hindsight-api

overall_status=0
for mode in end_to_end retrieval_only; do
  final_report="${output_dir}/${run_suffix}-${mode}.json"
  temporary_report="$(mktemp "${output_dir}/.${run_suffix}-${mode}.XXXXXX.tmp")"
  pending_reports+=("${temporary_report}")
  chmod 600 "${temporary_report}"

  set +e
  "${compose[@]}" run --rm --no-deps -T hindsight-runner \
    -manifest /fixtures/manifest.json \
    -golden /fixtures/golden.json \
    -mode "${mode}" \
    -pretty >"${temporary_report}"
  runner_status=$?
  set -e

  if ! python3 - \
    "${temporary_report}" "${manifest_file}" "${golden_file}" "${env_file}" "${mode}" <<'PY'
import json
import sys
from pathlib import Path

report_path, manifest_path, golden_path, env_path, mode = sys.argv[1:6]
report_text = Path(report_path).read_text(encoding="utf-8")
report = json.loads(report_text)
manifest = json.loads(Path(manifest_path).read_text(encoding="utf-8"))
golden = json.loads(Path(golden_path).read_text(encoding="utf-8"))

if report.get("schemaVersion") != "neo-chat.memory-hindsight-fixture-report.v1":
    raise SystemExit("invalid report schema")
if report.get("promotionEligible") is not False:
    raise SystemExit("report must remain promotion-ineligible")
if report.get("profile", {}).get("mode") != mode:
    raise SystemExit("report mode mismatch")
if report.get("profile", {}).get("remoteProviderCalls") != 0:
    raise SystemExit("report claims a remote Provider call")

for fixture in manifest["fixtures"]:
    for memory in fixture["memories"]:
        for field in ("canonicalContent", "rawEventContent"):
            if memory[field] in report_text:
                raise SystemExit("fixture plaintext leaked into report")
for case in golden["cases"]:
    if case["query"] in report_text:
        raise SystemExit("query plaintext leaked into report")
for line in Path(env_path).read_text(encoding="utf-8").splitlines():
    _, value = line.split("=", 1)
    if value and value in report_text:
        raise SystemExit("ephemeral credential leaked into report")
PY
  then
    echo "Hindsight fixture: invalid or non-content-free ${mode} report" >&2
    overall_status=1
    continue
  fi

  if ! ln "${temporary_report}" "${final_report}" 2>/dev/null; then
    echo "Hindsight fixture: report already exists or cannot be published" >&2
    overall_status=1
    continue
  fi
  chmod 600 "${final_report}"
  python3 - "${temporary_report}" <<'PY'
import os
import sys
os.unlink(sys.argv[1])
PY
  pending_reports[${#pending_reports[@]}-1]=""
  echo "Hindsight fixture: retained content-free report ${final_report}"
  if [[ ${runner_status} -ne 0 ]]; then
    overall_status=1
  fi
done

exit "${overall_status}"
