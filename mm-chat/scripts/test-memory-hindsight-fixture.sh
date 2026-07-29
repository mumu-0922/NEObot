#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
compose_file="${project_dir}/compose.hindsight-fixture.yml"
docker_bin="${DOCKER_BIN:-docker}"
docker_uses_windows_paths=false

if ! "${docker_bin}" compose version >/dev/null 2>&1; then
  windows_docker="/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe"
  if [[ -x "${windows_docker}" ]] && "${windows_docker}" compose version >/dev/null 2>&1; then
    docker_bin="${windows_docker}"
    docker_uses_windows_paths=true
  else
    echo "Hindsight fixture topology: Docker Compose is required" >&2
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

temp_dir="$(mktemp -d)"
trap 'find "${temp_dir}" -depth -delete' EXIT
env_file="${temp_dir}/fixture.env"
rendered="${temp_dir}/compose.json"
cat >"${env_file}" <<'EOF'
HINDSIGHT_FIXTURE_DB_PASSWORD=fixture-render-password-not-runtime
HINDSIGHT_FIXTURE_API_KEY=fixture-render-api-key-0000000000000000
EOF

"${docker_bin}" compose \
  --project-name mmchat-hindsight-fixture-static \
  --project-directory "$(docker_path "${project_dir}")" \
  --env-file "$(docker_path "${env_file}")" \
  -f "$(docker_path "${compose_file}")" \
  --profile memory-hindsight-fixture \
  config --format json >"${rendered}"

python3 - "${rendered}" "${project_dir}" <<'PY'
import json
import sys
from pathlib import Path

config = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
root = Path(sys.argv[2]).resolve()
services = config.get("services", {})
expected = {"hindsight-postgres", "hindsight-api", "hindsight-runner"}


def normalize_path(value: str) -> Path:
    normalized = value.replace("\\", "/")
    for prefix in ("//wsl.localhost/Ubuntu", "//wsl$/Ubuntu"):
        if normalized.startswith(prefix + "/"):
            normalized = normalized[len(prefix):]
            break
    return Path(normalized).resolve()


if set(services) != expected:
    raise SystemExit(f"Hindsight fixture topology: service drift: {sorted(services)}")

for name, service in services.items():
    if service.get("profiles") != ["memory-hindsight-fixture"]:
        raise SystemExit(f"Hindsight fixture topology: {name} profile drift")
    if service.get("ports"):
        raise SystemExit(f"Hindsight fixture topology: {name} publishes a host port")
    if set(service.get("networks", {})) != {"hindsight-private"}:
        raise SystemExit(f"Hindsight fixture topology: {name} network drift")
    if not service.get("mem_limit") or not service.get("cpus") or not service.get("pids_limit"):
        raise SystemExit(f"Hindsight fixture topology: {name} resource limit missing")

network = config.get("networks", {}).get("hindsight-private", {})
if network.get("internal") is not True:
    raise SystemExit("Hindsight fixture topology: private network is not internal")

postgres = services["hindsight-postgres"]
if postgres.get("image") != "pgvector/pgvector:pg17@sha256:d2ef61f42ef767baa5a1475393303cc235bcd92febd9d7014eddb48b41f3bad0":
    raise SystemExit("Hindsight fixture topology: PostgreSQL digest drift")
api = services["hindsight-api"]
if api.get("image") != "ghcr.io/vectorize-io/hindsight-api:0.8.5@sha256:35d88f6fc2d63ba37e8118dc02945097bf34e4ad04d4f3299e3c426db72c04ba":
    raise SystemExit("Hindsight fixture topology: API digest drift")
offline_environment = {
    "HF_HUB_OFFLINE": "1",
    "TRANSFORMERS_OFFLINE": "1",
    "HF_HUB_DISABLE_TELEMETRY": "1",
    "DO_NOT_TRACK": "1",
    "LITELLM_LOCAL_MODEL_COST_MAP": "True",
}
for key, value in offline_environment.items():
    if api["environment"].get(key) != value:
        raise SystemExit(f"Hindsight fixture topology: offline fence drift: {key}")
fixed_api_environment = {
    "HINDSIGHT_API_ENABLE_BANK_CONFIG_API": "true",
    "HINDSIGHT_API_LLM_PROVIDER": "mock",
    "HINDSIGHT_API_LLM_MODEL": "mock",
    "HINDSIGHT_API_EMBEDDINGS_PROVIDER": "local",
    "HINDSIGHT_API_EMBEDDINGS_LOCAL_MODEL": "BAAI/bge-small-en-v1.5",
    "HINDSIGHT_API_EMBEDDINGS_LOCAL_TRUST_REMOTE_CODE": "false",
    "HINDSIGHT_API_RERANKER_PROVIDER": "local",
    "HINDSIGHT_API_RERANKER_LOCAL_MODEL": "cross-encoder/ms-marco-MiniLM-L-6-v2",
    "HINDSIGHT_API_RERANKER_LOCAL_TRUST_REMOTE_CODE": "false",
    "HINDSIGHT_API_AUDIT_LOG_ENABLED": "false",
    "HINDSIGHT_API_LLM_TRACE_ENABLED": "false",
    "HINDSIGHT_API_OTEL_TRACES_ENABLED": "false",
}
for key, value in fixed_api_environment.items():
    if api["environment"].get(key) != value:
        raise SystemExit(f"Hindsight fixture topology: fixed API profile drift: {key}")

runner = services["hindsight-runner"]
if runner.get("read_only") is not True or runner.get("cap_drop") != ["ALL"]:
    raise SystemExit("Hindsight fixture topology: runner hardening drift")
if "no-new-privileges:true" not in runner.get("security_opt", []):
    raise SystemExit("Hindsight fixture topology: runner privilege boundary drift")
if runner.get("build", {}).get("target") != "hindsight-fixture":
    raise SystemExit("Hindsight fixture topology: runner Docker target drift")

allowed_sources = {
    (root / "docs/contracts/memory-hindsight-fixture-draft.json").resolve(),
    (root / "docs/contracts/memory-benchmark-golden-draft-template.json").resolve(),
}
mounted_sources = set()
for mount in runner.get("volumes", []):
    if mount.get("type") != "bind" or mount.get("read_only") is not True:
        raise SystemExit("Hindsight fixture topology: runner mount is not exact read-only bind")
    mounted_sources.add(normalize_path(str(mount["source"])))
if mounted_sources != allowed_sources:
    raise SystemExit("Hindsight fixture topology: runner fixture mount drift")

for forbidden in ("private", "rag-private", "mm_chat_provider_keyring"):
    if forbidden in config.get("networks", {}) or forbidden in config.get("secrets", {}):
        raise SystemExit(f"Hindsight fixture topology: main runtime authority leaked: {forbidden}")
PY

if rg -n '\.env\.single-server|mm-chat/(data|secrets|backup)|\.\./(data|secrets|backup)' "${compose_file}"; then
  echo "Hindsight fixture topology: protected Native runtime path referenced" >&2
  exit 1
fi

bash -n "${script_dir}/run-memory-hindsight-fixture.sh"
echo "Hindsight fixture topology: passed"
