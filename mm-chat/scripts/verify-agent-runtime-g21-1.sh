#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
example_env="${project_dir}/.env.single-server.example"
work_dir="$(mktemp -d)"

cleanup() {
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

for command in bash docker go jq python3 rg; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "G21.1 verification: ${command} is required" >&2
    exit 1
  }
done

bash "${script_dir}/verify-agent-root-canary-activation.sh"

(
  umask 022
  cd "${backend_dir}"
  go test -race \
    ./cmd/agent-runtime-root-canary \
    ./cmd/neo-runnerd \
    ./internal/agentactivation \
    ./internal/agentrootcanary \
    ./internal/agentorchestrator \
    ./internal/agentrunner
  go vet \
    ./cmd/agent-runtime-root-canary \
    ./cmd/neo-runnerd \
    ./internal/agentactivation \
    ./internal/agentrootcanary \
    ./internal/agentorchestrator \
    ./internal/agentrunner
)

if rg -n 'agentrootcanary|agent-runtime-root-canary' "${backend_dir}/cmd/api" >/dev/null; then
  echo 'G21.1 verification: Root canary leaked into cmd/api' >&2
  exit 1
fi
if find "${backend_dir}/migrations" -maxdepth 1 -name '091_*' -print -quit | grep -q .; then
  echo 'G21.1 verification: migration 091 is forbidden in this slice' >&2
  exit 1
fi
for signature in \
  'NewHTTPHandlerWithPolicies' \
  'spiffe://neo-chat/agent-runtime-control' \
  'spiffe://neo-chat/agent-runtime-root-canary' \
  'MethodLaunch, agentrunner.MethodHeartbeat, agentrunner.MethodCancel'; do
  if ! rg -Fq "${signature}" "${backend_dir}/cmd/neo-runnerd/main.go"; then
    echo "G21.1 verification: Runner caller policy signature missing: ${signature}" >&2
    exit 1
  fi
done
for signature in \
  'agent_orchestrator_transition(' \
  'agent_runner_update_sandbox(' \
  'tx.Commit()'; do
  if ! rg -Fq "${signature}" "${backend_dir}/internal/agentrootcanary/repository_postgres.go"; then
    echo "G21.1 verification: atomic terminal signature missing: ${signature}" >&2
    exit 1
  fi
done

for mode in development production; do
  compose_args=(
    docker compose
    --project-directory "${project_dir}"
    --env-file "${example_env}"
    -f "${project_dir}/compose.single-server.yml"
  )
  if [[ "${mode}" == production ]]; then
    compose_args+=( -f "${project_dir}/compose.production.yml" )
  fi
  "${compose_args[@]}" --profile agent-runtime-root-canary config --format json \
    >"${work_dir}/${mode}.json"
done
python3 - "${work_dir}/development.json" "${work_dir}/production.json" <<'PY'
import json
import sys
from pathlib import Path

development = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
production = json.loads(Path(sys.argv[2]).read_text(encoding="utf-8"))

for config in (development, production):
    service = config["services"]["agent-runtime-root-canary"]
    assert service["profiles"] == ["agent-runtime-root-canary"]
    assert "ports" not in service
    assert service["read_only"] is True
    assert service["init"] is True
    assert service["cap_drop"] == ["ALL"]
    assert "no-new-privileges:true" in service["security_opt"]
    assert list(service["networks"]) == ["private"]
    assert service.get("secrets", []) == []
    assert len(service["volumes"]) == 9
    assert all(volume["read_only"] is True for volume in service["volumes"])
    assert all(volume["bind"]["create_host_path"] is False for volume in service["volumes"])
    environment = service["environment"]
    assert environment["AGENT_ROOT_RUN_CANARY_ENABLED"] == "false"
    for name in (
        "AGENT_RUNTIME_ENABLED",
        "AGENT_SCHEDULER_ENABLED",
        "AGENT_SKILL_INSTALL_ENABLED",
        "AGENT_LEARNING_ENABLED",
        "AGENT_DELEGATION_ENABLED",
        "AGENT_BROKER_READ_ONLY_ENABLED",
        "AGENT_BROKER_MUTATION_ENABLED",
    ):
        assert environment[name] == "false", name
    for forbidden in (
        "DATABASE_URL",
        "MIGRATION_DATABASE_URL",
        "PROVIDER_SECRET_KEYRING_FILE",
        "REDIS_URL",
        "S3_ACCESS_KEY_ID",
        "S3_SECRET_ACCESS_KEY",
        "MCP_RUNNER_TOKEN_FILE",
    ):
        assert forbidden not in environment, forbidden

development_service = development["services"]["agent-runtime-root-canary"]
assert development_service["build"]["target"] == "runtime"
assert "build" not in production["services"]["agent-runtime-root-canary"]
PY

bash "${script_dir}/test-preflight-single-server.sh"
bash "${script_dir}/verify-agent-root-canary-postgres17.sh"

# G21.1 may only narrow the already-verified G21.0 control plane.
bash "${script_dir}/verify-agent-runtime-g21-0.sh"

set +e
host_output="$(bash "${script_dir}/verify-agent-runner-host.sh" 2>&1)"
host_status=$?
set -e
if [[ "${host_status}" -eq 0 || "${host_output}" != *ISOLATION_UNAVAILABLE* ]]; then
  echo 'G21.1 verification: current host unexpectedly became production-eligible' >&2
  exit 1
fi

printf '%s\n' 'Agent Runtime G21.1 verification: passed; current host remains ISOLATION_UNAVAILABLE.'
