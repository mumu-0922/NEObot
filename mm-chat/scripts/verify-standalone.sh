#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
usage: verify-standalone.sh [--full]

Without arguments, verify an isolated mm-chat copy, its manifests, symlink
boundary, absolute-path boundary, and rendered Compose topology.

--full additionally installs and verifies the frontend, runs Go tests, and
creates an isolated Python environment for the RAG quality gates.
EOF
}

full=false
case "${1:-}" in
  "") ;;
  --full) full=true ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
temp_dir="$(mktemp -d)"
copy_dir="${temp_dir}/mm-chat"
trap 'rm -rf "${temp_dir}"' EXIT

docker_bin="${DOCKER_BIN:-docker}"
docker_uses_windows_paths=false
if ! "${docker_bin}" compose version >/dev/null 2>&1; then
  windows_docker="/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe"
  if [[ -x "${windows_docker}" ]] && "${windows_docker}" compose version >/dev/null 2>&1; then
    docker_bin="${windows_docker}"
    docker_uses_windows_paths=true
  else
    echo "standalone verification: docker compose is required for Compose topology rendering" >&2
    echo "standalone verification: start Docker Desktop and enable WSL integration, or set DOCKER_BIN" >&2
    exit 1
  fi
fi

docker_path() {
  local path="$1"
  if [[ "${docker_uses_windows_paths}" == true ]]; then
    wslpath -w "${path}"
  else
    printf '%s\n' "${path}"
  fi
}

mkdir -p "${copy_dir}"
tar \
  --exclude='./.env.single-server' \
  --exclude='./backup' \
  --exclude='./data' \
  --exclude='./secrets' \
  --exclude='./frontend/.next' \
  --exclude='./frontend/.open-next' \
  --exclude='./frontend/node_modules' \
  --exclude='./frontend/tsconfig.tsbuildinfo' \
  --exclude='./backend/mcp-runner-runtime/node_modules' \
  --exclude='./rag/.mypy_cache' \
  --exclude='./rag/.pytest_cache' \
  --exclude='./rag/.ruff_cache' \
  --exclude='./rag/.venv' \
  -C "${project_dir}" -cf - . | tar -C "${copy_dir}" -xf -

required_paths=(
  README.md
  compose.yml
  compose.single-server.yml
  compose.hindsight-fixture.yml
  compose.memory-regression.yml
  compose.production.yml
  frontend/package.json
  frontend/pnpm-lock.yaml
  frontend/next.config.ts
  frontend/Dockerfile
  frontend/src/app
  frontend/public
  backend/go.mod
  backend/Dockerfile
  scripts/run-memory-hindsight-fixture.sh
  scripts/test-memory-hindsight-fixture.sh
  scripts/run-memory-regression.sh
  scripts/test-memory-regression.sh
  scripts/run-memory-production-validation-from-vault.sh
  scripts/test-memory-production-validation-from-vault.sh
  scripts/run-memory-negative-guard-development-from-vault.sh
  scripts/test-memory-negative-guard-development-from-vault.sh
  scripts/run-memory-buffered-judge-development-from-vault.sh
  scripts/test-memory-buffered-judge-development-from-vault.sh
  scripts/run-memory-production-buffered-validation-from-vault.sh
  scripts/test-memory-production-buffered-validation-from-vault.sh
  scripts/run-memory-judge-slice-diagnostic-from-vault.sh
  scripts/test-memory-judge-slice-diagnostic-from-vault.sh
  scripts/run-memory-v20-abstention-diagnostic-from-vault.sh
  scripts/test-memory-v20-abstention-diagnostic-from-vault.sh
  scripts/run-memory-accuracy-repair-development-from-vault.sh
  scripts/test-memory-accuracy-repair-development-from-vault.sh
  scripts/run-memory-abstention-confirmation-development-from-vault.sh
  scripts/test-memory-abstention-confirmation-development-from-vault.sh
  scripts/run-memory-abstention-confirmation-validation-from-vault.sh
  scripts/test-memory-abstention-confirmation-validation-from-vault.sh
  scripts/run-memory-double-confirmation-development-from-vault.sh
  scripts/test-memory-double-confirmation-development-from-vault.sh
  scripts/run-memory-double-confirmation-validation-from-vault.sh
  scripts/test-memory-double-confirmation-validation-from-vault.sh
  scripts/run-memory-single-user-bounded-miss-development-from-vault.sh
  scripts/test-memory-single-user-bounded-miss-development-from-vault.sh
  scripts/run-memory-single-user-bounded-miss-validation-from-vault.sh
  scripts/test-memory-single-user-bounded-miss-validation-from-vault.sh
  scripts/verify-agent-runtime-g21-2.sh
  scripts/verify-agent-broker-canary-activation.sh
  scripts/verify-agent-broker-canary-preflight.sh
  scripts/verify-agent-artifact-publication-postgres17.sh
  scripts/verify-agent-runtime-g21-1.sh
  scripts/verify-agent-root-canary-postgres17.sh
  scripts/verify-agent-runtime-g21-3.sh
  scripts/verify-agent-project-canary-activation.sh
  scripts/verify-agent-project-canary-preflight.sh
  scripts/verify-agent-project-mutation-postgres17.sh
  scripts/verify-agent-runtime-g21-4.sh
  scripts/verify-agent-child-canary-activation.sh
  scripts/verify-agent-child-canary-preflight.sh
  scripts/verify-agent-child-canary-postgres17.sh
  scripts/verify-agent-runtime-g21-5.sh
  scripts/verify-agent-runtime-g21-5-preflight.sh
  scripts/verify-agent-cron-worker.sh
  scripts/verify-agent-cron-worker-postgres17.sh
  scripts/verify-agent-draft-learning-worker.sh
  scripts/verify-agent-draft-learning-worker-postgres17.sh
  scripts/verify-agent-runtime-g21-6.sh
  scripts/verify-agent-runtime-g21-6-preflight.sh
  scripts/verify-agent-product-canary-activation.sh
  scripts/verify-agent-product-canary-postgres17.sh
  scripts/verify-agent-production-closure.sh
  backend/cmd/agent-runtime-product-canary/main.go
  backend/internal/agentproductcanary/service.go
  backend/migrations/095_agent_product_canary_activation.up.sql
  backend/migrations/095_agent_product_canary_activation.down.sql
  docs/contracts/schemas/neo-agent-product-canary-activation.schema.json
  docs/contracts/schemas/neo-agent-product-canary-plan.schema.json
  rag/pyproject.toml
  rag/uv.lock
  rag/Dockerfile
)
for path in "${required_paths[@]}"; do
  if [[ ! -e "${copy_dir}/${path}" ]]; then
    echo "standalone verification: missing ${path}" >&2
    exit 1
  fi
done

if symlink="$(find "${copy_dir}" -type l -print -quit)" && [[ -n "${symlink}" ]]; then
  echo "standalone verification: symbolic link is not allowed: ${symlink#${copy_dir}/}" >&2
  exit 1
fi

if rg -n --hidden \
  --glob '!**/docs/tracking/process.md' \
  --glob '!**/scripts/verify-standalone.sh' \
  '/home/mumu/projects/neo-chat|\\\\wsl\.localhost\\Ubuntu\\home\\mumu\\projects\\neo-chat' \
  "${copy_dir}" >"${temp_dir}/outer-paths.txt"; then
  echo "standalone verification: outer-project absolute path found" >&2
  cat "${temp_dir}/outer-paths.txt" >&2
  exit 1
fi

compose_json="${temp_dir}/compose.json"
"${docker_bin}" compose \
  --project-directory "$(docker_path "${copy_dir}")" \
  -f "$(docker_path "${copy_dir}/compose.yml")" \
  --profile app --profile ops --profile memory-worker \
  --profile mcp-runner \
  --profile agent-runtime-control \
  --profile agent-runtime-root-canary \
  --profile agent-runtime-broker-canary \
  --profile agent-runtime-project-canary \
  --profile agent-runtime-child-canary \
  --profile agent-runtime-cron-worker \
  --profile agent-runtime-draft-learning-worker \
  --profile agent-runtime-product-canary \
  --profile rag-worker --profile rag-ops \
  config --format json >"${compose_json}"

python3 - "${compose_json}" "${copy_dir}" <<'PY'
import json
import sys
from pathlib import Path

config = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
root = Path(sys.argv[2]).resolve()


def normalize_path(value: str) -> Path:
    # Windows docker.exe may render WSL paths as UNC paths such as
    # \\wsl.localhost\Ubuntu\tmp\...; convert those back before comparing
    # against the Linux-side clean-copy root.
    normalized = value.replace("\\", "/")
    for prefix in ("//wsl.localhost/Ubuntu", "//wsl$/Ubuntu"):
        if normalized.startswith(prefix + "/"):
            normalized = normalized[len(prefix) :]
            break
    return Path(normalized).resolve()


services = config["services"]
required = {
    "frontend",
    "backend",
    "memory-worker",
    "mcp-runner",
    "agent-runtime-control",
    "agent-runtime-root-canary",
    "agent-runtime-broker-canary",
    "agent-runtime-project-canary",
    "agent-runtime-child-canary",
    "agent-runtime-cron-worker",
    "agent-runtime-draft-learning-worker",
    "agent-runtime-product-canary",
    "postgres",
    "redis",
    "minio",
    "minio-init",
    "migrate",
    "admin",
    "rag-worker",
    "rag-replay",
}
missing = sorted(required - services.keys())
if missing:
    raise SystemExit(f"standalone verification: missing services: {missing}")

for name, service in services.items():
    build = service.get("build")
    if not build:
        continue
    context = normalize_path(str(build["context"]))
    if context != root and root not in context.parents:
        raise SystemExit(
            f"standalone verification: {name} build context escapes project: {context}"
        )

frontend = services["frontend"]
if frontend["environment"]["NEXT_PUBLIC_API_MODE"] != "server":
    raise SystemExit("standalone verification: frontend is not in server mode")
if frontend["environment"]["NEXT_PUBLIC_API_BASE_URL"] != "/mm-api":
    raise SystemExit("standalone verification: frontend API edge is not /mm-api")
if "backend" not in frontend.get("depends_on", {}):
    raise SystemExit("standalone verification: frontend does not depend on backend")

backend = services["backend"]
memory_worker = services["memory-worker"]
agent_control = services["agent-runtime-control"]
root_canary = services["agent-runtime-root-canary"]
broker_canary = services["agent-runtime-broker-canary"]
project_canary = services["agent-runtime-project-canary"]
child_canary = services["agent-runtime-child-canary"]
cron_worker = services["agent-runtime-cron-worker"]
draft_learning_worker = services["agent-runtime-draft-learning-worker"]
product_canary = services["agent-runtime-product-canary"]
if backend.get("init") is not True:
    raise SystemExit("standalone verification: Backend must use init for local command reaping")
if backend.get("environment", {}).get("AGENT_LOCAL_RUNTIME_ENABLED") != "true":
    raise SystemExit("standalone verification: local_direct must default enabled")
if backend["environment"].get("AGENT_LOCAL_RUNTIME_ROOT") != "/var/lib/mm-chat/agent-skills":
    raise SystemExit("standalone verification: local Skill runtime root drifted")
if backend["environment"].get("AGENT_LOCAL_WORKSPACE_ROOT") != "/workspace":
    raise SystemExit("standalone verification: local Skill workspace root drifted")
backend_mounts = {volume.get("target"): volume for volume in backend.get("volumes", [])}
for target in ("/var/lib/mm-chat/agent-skills", "/workspace"):
    if target not in backend_mounts or backend_mounts[target].get("read_only") is True:
        raise SystemExit(f"standalone verification: writable local Skill mount missing: {target}")
    if backend_mounts[target].get("bind", {}).get("create_host_path") is not False:
        raise SystemExit(f"standalone verification: local Skill bind may create a root-owned host path: {target}")
if memory_worker.get("profiles") != ["memory-worker"]:
    raise SystemExit("standalone verification: Memory Worker profile drifted")
if memory_worker.get("ports"):
    raise SystemExit("standalone verification: Memory Worker exposes a host port")
if set(memory_worker.get("networks", {})) != {"private"}:
    raise SystemExit("standalone verification: Memory Worker is not private-only")
if agent_control.get("profiles") != ["agent-runtime-control"]:
    raise SystemExit("standalone verification: Agent control profile drifted")
if agent_control.get("ports"):
    raise SystemExit("standalone verification: Agent control exposes a host port")
if set(agent_control.get("networks", {})) != {"private"}:
    raise SystemExit("standalone verification: Agent control is not private-only")
if agent_control["environment"]["AGENT_RUNNER_CONTROL_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent control must default false")
for name in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_DELEGATION_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
):
    if agent_control["environment"][name] != "false":
        raise SystemExit(f"standalone verification: {name} must default false")
if root_canary.get("profiles") != ["agent-runtime-root-canary"]:
    raise SystemExit("standalone verification: Agent Root canary profile drifted")
if root_canary.get("ports"):
    raise SystemExit("standalone verification: Agent Root canary exposes a host port")
if set(root_canary.get("networks", {})) != {"private"}:
    raise SystemExit("standalone verification: Agent Root canary is not private-only")
if root_canary["environment"]["AGENT_ROOT_RUN_CANARY_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent Root canary must default false")
for name in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_DELEGATION_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
):
    if root_canary["environment"][name] != "false":
        raise SystemExit(f"standalone verification: Root canary {name} must default false")
if broker_canary.get("profiles") != ["agent-runtime-broker-canary"]:
    raise SystemExit("standalone verification: Agent Broker canary profile drifted")
if broker_canary.get("ports"):
    raise SystemExit("standalone verification: Agent Broker canary exposes a host port")
if set(broker_canary.get("networks", {})) != {"private", "mcp-control", "agent-broker-relay"}:
    raise SystemExit("standalone verification: Agent Broker canary network boundary drifted")
if broker_canary["environment"]["AGENT_BROKER_ARTIFACT_CANARY_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent Broker canary must default false")
if broker_canary["environment"]["S3_BUCKET_AUTO_CREATE"] != "false":
    raise SystemExit("standalone verification: Agent Broker canary may not create buckets")
if broker_canary.get("secrets") != [
    {"source": "mm_chat_mcp_runner_token", "target": "mm_chat_mcp_runner_token"}
]:
    raise SystemExit("standalone verification: Agent Broker canary secret boundary drifted")
if len(broker_canary.get("volumes", [])) != 15:
    raise SystemExit("standalone verification: Agent Broker canary mount set drifted")
if sum(volume.get("read_only") is not True for volume in broker_canary["volumes"]) != 1:
    raise SystemExit("standalone verification: Agent Broker canary writable mounts drifted")
for name in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_DELEGATION_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
):
    if broker_canary["environment"][name] != "false":
        raise SystemExit(f"standalone verification: Broker canary {name} must default false")
for forbidden in (
    "DATABASE_URL",
    "MIGRATION_DATABASE_URL",
    "PROVIDER_SECRET_KEYRING_FILE",
    "REDIS_URL",
    "MCP_RUNNER_TOKEN",
):
    if forbidden in broker_canary["environment"]:
        raise SystemExit(f"standalone verification: Broker canary received {forbidden}")
if not config["networks"]["agent-broker-relay"]["internal"]:
    raise SystemExit("standalone verification: Agent Broker relay is not internal")
if project_canary.get("profiles") != ["agent-runtime-project-canary"]:
    raise SystemExit("standalone verification: Agent Project canary profile drifted")
if project_canary.get("ports"):
    raise SystemExit("standalone verification: Agent Project canary exposes a host port")
if set(project_canary.get("networks", {})) != {"private", "agent-project-relay"}:
    raise SystemExit("standalone verification: Agent Project canary network boundary drifted")
if project_canary["environment"]["AGENT_PROJECT_MUTATION_CANARY_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent Project canary must default false")
if project_canary.get("secrets", []) != [] or len(project_canary.get("volumes", [])) != 14:
    raise SystemExit("standalone verification: Agent Project canary material boundary drifted")
if not config["networks"]["agent-project-relay"]["internal"]:
    raise SystemExit("standalone verification: Agent Project relay is not internal")
if child_canary.get("profiles") != ["agent-runtime-child-canary"]:
    raise SystemExit("standalone verification: Agent Child canary profile drifted")
if child_canary.get("ports"):
    raise SystemExit("standalone verification: Agent Child canary exposes a host port")
if set(child_canary.get("networks", {})) != {"private"}:
    raise SystemExit("standalone verification: Agent Child canary is not private-only")
if child_canary["environment"]["AGENT_CHILD_CANARY_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent Child canary must default false")
if child_canary["environment"]["AGENT_DELEGATION_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent Child delegation must default false")
if child_canary.get("secrets", []) != [] or len(child_canary.get("volumes", [])) != 9:
    raise SystemExit("standalone verification: Agent Child canary material boundary drifted")
for name in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
):
    if child_canary["environment"][name] != "false":
        raise SystemExit(f"standalone verification: Child canary {name} must default false")
for forbidden in (
    "MCP_RUNNER_TOKEN",
    "S3_ACCESS_KEY_ID",
    "S3_SECRET_ACCESS_KEY",
    "PROVIDER_SECRET_KEYRING_FILE",
    "REDIS_URL",
    "VAULT_ADDR",
):
    if forbidden in child_canary["environment"]:
        raise SystemExit(f"standalone verification: Child canary received {forbidden}")
if cron_worker.get("profiles") != ["agent-runtime-cron-worker"]:
    raise SystemExit("standalone verification: Agent Cron worker profile drifted")
if cron_worker.get("ports") or cron_worker.get("secrets", []) != []:
    raise SystemExit("standalone verification: Agent Cron worker exposure drifted")
if set(cron_worker.get("networks", {})) != {"private"}:
    raise SystemExit("standalone verification: Agent Cron worker is not private-only")
if cron_worker["environment"]["AGENT_CRON_WORKER_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent Cron worker must default false")
if "AGENT_DRAFT_LEARNING_WORKER_ENABLED" in cron_worker["environment"]:
    raise SystemExit("standalone verification: Agent Cron worker received the Draft flag")
if len(cron_worker.get("volumes", [])) != 8 or any(
    volume.get("read_only") is not True for volume in cron_worker.get("volumes", [])
):
    raise SystemExit("standalone verification: Agent Cron worker mount set drifted")
for name in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_DELEGATION_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
):
    if cron_worker["environment"][name] != "false":
        raise SystemExit(f"standalone verification: Agent Cron worker {name} must default false")
for forbidden in (
    "AGENT_DRAFT_LEARNING_WORKER_DATABASE_URL",
    "AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL",
    "MCP_RUNNER_TOKEN",
    "S3_ACCESS_KEY_ID",
    "S3_SECRET_ACCESS_KEY",
    "PROVIDER_SECRET_KEYRING_FILE",
    "REDIS_URL",
    "VAULT_ADDR",
):
    if forbidden in cron_worker["environment"]:
        raise SystemExit(f"standalone verification: Agent Cron worker received {forbidden}")
if draft_learning_worker.get("profiles") != ["agent-runtime-draft-learning-worker"]:
    raise SystemExit("standalone verification: Agent Draft-learning worker profile drifted")
if draft_learning_worker.get("ports") or draft_learning_worker.get("secrets", []) != []:
    raise SystemExit("standalone verification: Agent Draft-learning worker exposure drifted")
if set(draft_learning_worker.get("networks", {})) != {"private"}:
    raise SystemExit("standalone verification: Agent Draft-learning worker is not private-only")
if draft_learning_worker["environment"]["AGENT_DRAFT_LEARNING_WORKER_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent Draft-learning worker must default false")
if "AGENT_CRON_WORKER_ENABLED" in draft_learning_worker["environment"]:
    raise SystemExit("standalone verification: Agent Draft-learning worker received the Cron flag")
if len(draft_learning_worker.get("volumes", [])) != 15 or any(
    volume.get("read_only") is not True
    for volume in draft_learning_worker.get("volumes", [])
):
    raise SystemExit("standalone verification: Agent Draft-learning worker mount set drifted")
if draft_learning_worker["environment"]["S3_BUCKET_AUTO_CREATE"] != "false":
    raise SystemExit("standalone verification: Agent Draft-learning worker may not create buckets")
for name in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_DELEGATION_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
):
    if draft_learning_worker["environment"][name] != "false":
        raise SystemExit(
            f"standalone verification: Agent Draft-learning worker {name} must default false"
        )
if product_canary.get("profiles") != ["agent-runtime-product-canary"]:
    raise SystemExit("standalone verification: Agent Product canary profile drifted")
if product_canary.get("ports") or product_canary.get("secrets", []) != []:
    raise SystemExit("standalone verification: Agent Product canary exposure drifted")
if set(product_canary.get("networks", {})) != {"private"}:
    raise SystemExit("standalone verification: Agent Product canary is not private-only")
if product_canary["environment"]["AGENT_PRODUCT_CANARY_ENABLED"] != "false":
    raise SystemExit("standalone verification: Agent Product canary must default false")
if product_canary["environment"]["AGENT_PRODUCT_CANARY_CLIENT_IDENTITY"] != (
    "spiffe://neo-chat/agent-runtime-product-canary"
):
    raise SystemExit("standalone verification: Agent Product canary identity drifted")
if len(product_canary.get("volumes", [])) != 9 or any(
    volume.get("read_only") is not True for volume in product_canary.get("volumes", [])
):
    raise SystemExit("standalone verification: Agent Product canary mount set drifted")
for name in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
):
    if product_canary["environment"][name] != "false":
        raise SystemExit(
            f"standalone verification: Agent Product canary {name} must default false"
        )
for forbidden in (
    "MCP_RUNNER_TOKEN",
    "S3_ACCESS_KEY_ID",
    "S3_SECRET_ACCESS_KEY",
    "PROVIDER_SECRET_KEYRING_FILE",
    "REDIS_URL",
    "VAULT_ADDR",
    "AGENT_BROKER_CANARY_DATABASE_URL",
    "AGENT_PROJECT_CANARY_DATABASE_URL",
    "AGENT_CHILD_CANARY_DATABASE_URL",
    "AGENT_CRON_WORKER_DATABASE_URL",
    "AGENT_DRAFT_LEARNING_WORKER_DATABASE_URL",
):
    if forbidden in product_canary["environment"]:
        raise SystemExit(
            f"standalone verification: Agent Product canary received {forbidden}"
        )
for forbidden in (
    "AGENT_CRON_WORKER_DATABASE_URL",
    "MCP_RUNNER_TOKEN",
    "PROVIDER_SECRET_KEYRING_FILE",
    "REDIS_URL",
    "VAULT_ADDR",
):
    if forbidden in draft_learning_worker["environment"]:
        raise SystemExit(
            f"standalone verification: Agent Draft-learning worker received {forbidden}"
        )
if (
    backend["environment"]["MEMORY_HYBRID_SHADOW_ENABLED"]
    != memory_worker["environment"]["MEMORY_HYBRID_SHADOW_ENABLED"]
):
    raise SystemExit("standalone verification: Memory hybrid flags disagree")
if backend["environment"]["MEMORY_TOOL_LOOP_ENABLED"] != "false":
    raise SystemExit("standalone verification: Memory Tool Loop must default false")
if "MEMORY_TOOL_LOOP_ENABLED" in memory_worker["environment"]:
    raise SystemExit("standalone verification: Memory Worker received the Tool Loop flag")
if backend["environment"]["MEMORY_TOOL_LOOP_CANARY_USER_IDS"] != "":
    raise SystemExit("standalone verification: Memory Tool canary must default empty")
if "MEMORY_TOOL_LOOP_CANARY_USER_IDS" in memory_worker["environment"]:
    raise SystemExit("standalone verification: Memory Worker received the Tool canary")
if (
    backend["environment"]["MEMORY_L2_SCENE_SHADOW_ENABLED"]
    != memory_worker["environment"]["MEMORY_L2_SCENE_SHADOW_ENABLED"]
):
    raise SystemExit("standalone verification: Memory L2 Scene shadow flags disagree")
if backend["environment"]["MEMORY_L2_SCENE_READER_ENABLED"] != "false":
    raise SystemExit("standalone verification: Memory L2 Scene reader must default false")
if "MEMORY_L2_SCENE_READER_ENABLED" in memory_worker["environment"]:
    raise SystemExit("standalone verification: Memory Worker received the L2 reader flag")
if (
    backend["environment"]["MEMORY_L3_PERSONA_SHADOW_ENABLED"]
    != memory_worker["environment"]["MEMORY_L3_PERSONA_SHADOW_ENABLED"]
):
    raise SystemExit("standalone verification: Memory L3 Persona shadow flags disagree")
if backend["environment"]["MEMORY_L3_PERSONA_READER_ENABLED"] != "false":
    raise SystemExit("standalone verification: Memory L3 Persona reader must default false")
if "MEMORY_L3_PERSONA_READER_ENABLED" in memory_worker["environment"]:
    raise SystemExit("standalone verification: Memory Worker received the L3 reader flag")
PY

DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-hindsight-fixture.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-regression.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-production-validation-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-negative-guard-development-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-buffered-judge-development-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-production-buffered-validation-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-judge-slice-diagnostic-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-v20-abstention-diagnostic-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-accuracy-repair-development-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-abstention-confirmation-development-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-abstention-confirmation-validation-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-double-confirmation-development-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-double-confirmation-validation-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-single-user-bounded-miss-development-from-vault.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-single-user-bounded-miss-validation-from-vault.sh"

if [[ "${full}" == true ]]; then
  bash "${copy_dir}/scripts/verify-agent-runtime-g21-6.sh"
  rag_python="${RAG_PYTHON:-python3.13}"
  rag_uv="${RAG_UV:-uv}"
  if ! command -v "${rag_python}" >/dev/null 2>&1; then
    echo "standalone verification: Python 3.13 is required for RAG checks" >&2
    exit 1
  fi
  if ! "${rag_python}" -c \
    'import sys; raise SystemExit(sys.version_info[:2] != (3, 13))'; then
    echo "standalone verification: RAG checks require Python 3.13" >&2
    exit 1
  fi
  if ! command -v "${rag_uv}" >/dev/null 2>&1; then
    echo "standalone verification: uv is required for frozen RAG dependency sync" >&2
    exit 1
  fi

  (
    cd "${copy_dir}/frontend"
    corepack pnpm install --frozen-lockfile
    corepack pnpm format:check
    corepack pnpm lint
    corepack pnpm typecheck
    corepack pnpm test
    NEXT_PUBLIC_API_MODE=server \
      NEXT_PUBLIC_API_BASE_URL=/mm-api \
      MM_CHAT_BACKEND_INTERNAL_URL=http://backend:8080 \
      corepack pnpm build
  )
  (
    cd "${copy_dir}/backend"
    test -z "$(gofmt -l .)"
    go vet ./...
    go test ./...
  )
  (
    cd "${copy_dir}/rag"
    "${rag_uv}" sync --frozen --all-groups --python "${rag_python}"
    .venv/bin/ruff check .
    .venv/bin/ruff format --check .
    .venv/bin/mypy src
    .venv/bin/pytest
  )
fi

if [[ "${full}" == true ]]; then
  echo "standalone verification: passed (full)"
else
  echo "standalone verification: passed (structure)"
fi
