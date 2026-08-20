#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
docker_bin="${DOCKER_BIN:-docker}"

for command in bash go python3 rg "${docker_bin}"; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "local Agent runtime verification: ${command} is required" >&2
    exit 1
  fi
done

bash -n "${BASH_SOURCE[0]}"

required_paths=(
  backend/internal/agents
  backend/internal/chat
  backend/internal/localskills
  backend/internal/skillsupply
  frontend/src/components/skills/SkillStore.tsx
  frontend/src/services/api/client/server/skillStoreApi.ts
  scripts/verify-chat-artifacts-postgres17.sh
)
for path in "${required_paths[@]}"; do
  if [[ ! -e "${project_dir}/${path}" ]]; then
    echo "local Agent runtime verification: missing ${path}" >&2
    exit 1
  fi
done

retired_paths=(
  backend/cmd/agent-runtime-control
  backend/cmd/agent-runtime-root-canary
  backend/cmd/agent-runtime-broker-canary
  backend/cmd/agent-runtime-project-canary
  backend/cmd/agent-runtime-child-canary
  backend/cmd/agent-runtime-cron-worker
  backend/cmd/agent-runtime-draft-learning-worker
  backend/cmd/agent-runtime-product-canary
  backend/cmd/neo-runnerd
  backend/cmd/neo-runner-probe
  backend/internal/agentcontrol
  backend/internal/agentdelegation
  backend/internal/agentrunner
  config/agent-runner
  deploy/agent-runner
)
for path in "${retired_paths[@]}"; do
  if [[ -e "${project_dir}/${path}" ]]; then
    echo "local Agent runtime verification: retired path remains: ${path}" >&2
    exit 1
  fi
done

if rg -n \
  'mm-chat-agent-runtime-(control|root-canary|broker-canary|project-canary|child-canary|cron-worker|draft-learning-worker|product-canary)' \
  "${project_dir}/backend/Dockerfile"; then
  echo "local Agent runtime verification: retired binary remains in Backend image" >&2
  exit 1
fi

for executable in bash file git jq nodejs npm py3-pip python3 ripgrep unzip zip; do
  if ! rg -q --fixed-strings "${executable}" "${project_dir}/backend/Dockerfile"; then
    echo "local Agent runtime verification: Backend image is missing ${executable}" >&2
    exit 1
  fi
done
rg -q 'mkdir -p /var/lib/mm-chat/agent-skills /workspace' "${project_dir}/backend/Dockerfile"

compose_json="$(mktemp)"
trap 'rm -f "${compose_json}"' EXIT
"${docker_bin}" compose \
  --project-directory "${project_dir}" \
  --env-file "${project_dir}/.env.single-server.example" \
  -f "${project_dir}/compose.single-server.yml" \
  --profile app --profile memory-worker --profile mcp-runner \
  config --format json >"${compose_json}"

python3 - "${compose_json}" "${project_dir}/.env.single-server.example" <<'PY'
import json
import re
import sys
from pathlib import Path

config = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
services = config["services"]

retired_services = {
    "agent-runtime-control",
    "agent-runtime-root-canary",
    "agent-runtime-broker-canary",
    "agent-runtime-project-canary",
    "agent-runtime-child-canary",
    "agent-runtime-cron-worker",
    "agent-runtime-draft-learning-worker",
    "agent-runtime-product-canary",
}
remaining = sorted(retired_services & services.keys())
if remaining:
    raise SystemExit(
        f"local Agent runtime verification: retired Compose services remain: {remaining}"
    )

backend = services.get("backend")
if not backend:
    raise SystemExit("local Agent runtime verification: Backend service is missing")
if backend.get("init") is not True:
    raise SystemExit("local Agent runtime verification: Backend must use init")

environment = backend.get("environment", {})
expected_environment = {
    "AGENT_LOCAL_RUNTIME_ENABLED": "true",
    "AGENT_LOCAL_RUNTIME_ROOT": "/var/lib/mm-chat/agent-skills",
    "AGENT_LOCAL_WORKSPACE_ROOT": "/workspace",
    "AGENT_LOCAL_WORKSPACE_HOST_ROOT": "",
    "AGENT_LOCAL_SHELL": "/bin/bash",
    "AGENT_LOCAL_APPROVAL_MODE": "smart",
}
for name, expected in expected_environment.items():
    if environment.get(name) != expected:
        raise SystemExit(
            f"local Agent runtime verification: {name} must render as {expected!r}"
        )

mounts = {volume.get("target"): volume for volume in backend.get("volumes", [])}
for target in ("/var/lib/mm-chat/agent-skills", "/workspace"):
    volume = mounts.get(target)
    if not volume or volume.get("read_only") is True:
        raise SystemExit(
            f"local Agent runtime verification: writable bind is missing: {target}"
        )
    if volume.get("bind", {}).get("create_host_path") is not False:
        raise SystemExit(
            f"local Agent runtime verification: bind may create a root-owned path: {target}"
        )

env_names = set()
for raw_line in Path(sys.argv[2]).read_text(encoding="utf-8").splitlines():
    if raw_line and not raw_line.startswith("#") and "=" in raw_line:
        env_names.add(raw_line.split("=", 1)[0])
legacy_prefixes = (
    "AGENT_RUNNER_",
    "AGENT_ROOT_",
    "AGENT_BROKER_",
    "AGENT_PROJECT_",
    "AGENT_CHILD_",
    "AGENT_CRON_",
    "AGENT_DRAFT_",
    "AGENT_PRODUCT_",
)
legacy_exact = {
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_DELEGATION_ENABLED",
}
legacy = sorted(
    name
    for name in env_names
    if name in legacy_exact or name.startswith(legacy_prefixes)
)
if legacy:
    raise SystemExit(
        f"local Agent runtime verification: retired environment remains: {legacy}"
    )
local_names = sorted(name for name in env_names if name.startswith("AGENT_LOCAL_"))
expected_local_names = sorted(
    {
        "AGENT_LOCAL_RUNTIME_ENABLED",
        "AGENT_LOCAL_RUNTIME_SOURCE",
        "AGENT_LOCAL_RUNTIME_ROOT",
        "AGENT_LOCAL_WORKSPACE_SOURCE",
        "AGENT_LOCAL_WORKSPACE_ROOT",
        "AGENT_LOCAL_WORKSPACE_HOST_ROOT",
        "AGENT_LOCAL_SHELL",
        "AGENT_LOCAL_APPROVAL_MODE",
        "AGENT_LOCAL_CALL_TIMEOUT",
        "AGENT_LOCAL_RUN_TIMEOUT",
        "AGENT_LOCAL_MAX_OUTPUT_BYTES",
        "AGENT_LOCAL_MAX_CALLS_PER_RUN",
        "AGENT_LOCAL_MAX_ROUNDS_PER_RUN",
        "AGENT_LOCAL_MAX_CONCURRENT",
    }
)
if local_names != expected_local_names:
    raise SystemExit(
        "local Agent runtime verification: AGENT_LOCAL_* example set drifted"
    )
PY

rg -q 'mux.Handle\("/v1/skills"' "${project_dir}/backend/internal/httpserver/server.go"
rg -q '/v1/skills/store' "${project_dir}/frontend/src/services/api/client/server/skillStoreApi.ts"

(
  cd "${project_dir}/backend"
  GOCACHE="${GOCACHE:-/tmp/neo-chat-go-cache}" \
    go test -count=1 ./internal/localskills ./internal/skillsupply ./internal/httpserver
)

echo "local Agent runtime verification: passed (Chat Agent + local_direct + Skill Store; legacy control plane absent)"
