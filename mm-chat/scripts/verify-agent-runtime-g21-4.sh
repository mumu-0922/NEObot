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
    echo "G21.4 verification: ${command} is required" >&2
    exit 1
  }
done

for path in \
  "${backend_dir}/cmd/agent-runtime-child-canary/main.go" \
  "${backend_dir}/internal/agentactivation/child_canary.go" \
  "${backend_dir}/internal/agentchildcanary/plan.go" \
  "${backend_dir}/internal/agentchildcanary/reaper.go" \
  "${backend_dir}/internal/agentchildcanary/service.go" \
  "${backend_dir}/migrations/093_agent_child_canary_reap_transport.up.sql" \
  "${backend_dir}/migrations/093_agent_child_canary_reap_transport.down.sql" \
  "${project_dir}/docs/contracts/schemas/neo-agent-child-run-canary-activation.schema.json" \
  "${project_dir}/docs/contracts/schemas/neo-agent-child-run-canary-plan.schema.json" \
  "${script_dir}/verify-agent-child-canary-activation.sh" \
  "${script_dir}/verify-agent-child-canary-preflight.sh" \
  "${script_dir}/verify-agent-child-canary-postgres17.sh"; do
  [[ -s "${path}" ]] || { echo "G21.4 verification: missing ${path}" >&2; exit 1; }
done
if rg -n 'agentchildcanary|agent-runtime-child-canary|child_canary' \
  "${backend_dir}/cmd/api" >/dev/null; then
  echo 'G21.4 verification: Child canary leaked into cmd/api' >&2
  exit 1
fi
if [[ "$(find "${backend_dir}/migrations" -maxdepth 1 -name '093_*.up.sql' -printf '%f\n')" != \
  '093_agent_child_canary_reap_transport.up.sql' ]]; then
  echo 'G21.4 verification: migration 093 tail is missing or widened' >&2
  exit 1
fi

for signature in \
  'CREATE FUNCTION agent_delegation_reap_inventory(' \
  'CREATE OR REPLACE FUNCTION agent_delegation_complete_reap(' \
  "request.method='launch'" 'max(request.expires_at)' \
  'AGENT_REAP_AUTHORITY_ACTIVE' 'AGENT_REAP_SANDBOX_MISMATCH' \
  "UPDATE agent_runner_sandboxes SET state='terminal'" \
  'TO agent_delegation_control'; do
  rg -Fq "${signature}" "${backend_dir}/migrations/093_agent_child_canary_reap_transport.up.sql" || {
    echo "G21.4 verification: reap transport signature missing: ${signature}" >&2
    exit 1
  }
done
for forbidden in \
  'GRANT UPDATE ON agent_runner_sandboxes TO agent_delegation_control' \
  'GRANT INSERT ON agent_runner_' 'GRANT DELETE ON agent_runner_'; do
  if rg -Fiq "${forbidden}" "${backend_dir}/migrations/093_agent_child_canary_reap_transport.up.sql"; then
    echo "G21.4 verification: reap transport authority widened: ${forbidden}" >&2
    exit 1
  fi
done
rg -Fq 'AGENT_CHILD_REAP_TRANSPORT_DOWN_REQUIRES_CLEAN' \
  "${backend_dir}/migrations/093_agent_child_canary_reap_transport.down.sql"

for signature in \
  'childCanaryCallerIdentity   = "spiffe://neo-chat/agent-runtime-child-canary"' \
  'NEO_RUNNER_CHILD_CANARY_CLIENT_IDENTITY' \
  'childIdentity == clientIdentity' 'childIdentity == canaryIdentity' \
  'childIdentity == brokerIdentity' 'childIdentity == projectIdentity'; do
  rg -Fq "${signature}" "${backend_dir}/cmd/neo-runnerd/main.go" || {
    echo "G21.4 verification: fifth caller isolation missing: ${signature}" >&2
    exit 1
  }
done
python3 - "${backend_dir}/cmd/neo-runnerd/main.go" <<'PY'
import sys
from pathlib import Path

source = Path(sys.argv[1]).read_text(encoding="utf-8")
start = source.index('childIdentity := strings.TrimSpace')
end = source.index('draftIdentity := strings.TrimSpace', start)
block = source[start:end]
for method in ('MethodProbe', 'MethodList', 'MethodReconcile', 'MethodLaunch', 'MethodHeartbeat', 'MethodCancel'):
    assert method in block, method
for forbidden in ('MethodPrepare', 'MethodCommit', 'WithBrokerRelayForCaller'):
    assert forbidden not in block, forbidden
PY

bash "${script_dir}/verify-agent-child-canary-activation.sh"
(
  umask 022
  cd "${backend_dir}"
  go test -race \
    ./cmd/agent-runtime-child-canary \
    ./cmd/neo-runnerd \
    ./internal/agentactivation \
    ./internal/agentbroker \
    ./internal/agentchildcanary \
    ./internal/agentdelegation \
    ./internal/agentorchestrator \
    ./internal/agentrunner \
    ./internal/migration
  go vet \
    ./cmd/agent-runtime-child-canary \
    ./cmd/neo-runnerd \
    ./internal/agentactivation \
    ./internal/agentbroker \
    ./internal/agentchildcanary \
    ./internal/agentdelegation \
    ./internal/agentorchestrator \
    ./internal/agentrunner \
    ./internal/migration
)

for mode in development production; do
  args=(docker compose --project-directory "${project_dir}" --env-file "${example_env}" \
    -f "${project_dir}/compose.single-server.yml")
  [[ "${mode}" == production ]] && args+=( -f "${project_dir}/compose.production.yml" )
  "${args[@]}" --profile agent-runtime-child-canary config --format json \
    >"${work_dir}/${mode}.json"
done
docker compose --project-directory "${project_dir}" --env-file "${example_env}" \
  -f "${project_dir}/compose.single-server.yml" config --format json >"${work_dir}/default.json"
python3 - "${work_dir}/development.json" "${work_dir}/production.json" "${work_dir}/default.json" <<'PY'
import json
import sys

development, production, default = (
    json.load(open(path, encoding="utf-8")) for path in sys.argv[1:]
)
assert "agent-runtime-child-canary" not in default.get("services", {})
for config in (development, production):
    service = config["services"]["agent-runtime-child-canary"]
    assert service["profiles"] == ["agent-runtime-child-canary"]
    assert "ports" not in service and service["read_only"] is True and service["init"] is True
    assert service["cap_drop"] == ["ALL"]
    assert "no-new-privileges:true" in service["security_opt"]
    assert list(service["networks"]) == ["private"]
    assert service.get("secrets", []) == []
    assert len(service["volumes"]) == 9
    assert all(volume["read_only"] is True for volume in service["volumes"])
    assert all(volume["bind"]["create_host_path"] is False for volume in service["volumes"])
    env = service["environment"]
    assert env["AGENT_CHILD_CANARY_ENABLED"] == "false"
    assert env["AGENT_DELEGATION_ENABLED"] == "false"
    assert env["AGENT_CHILD_CANARY_CLIENT_IDENTITY"] == "spiffe://neo-chat/agent-runtime-child-canary"
    assert env["AGENT_CHILD_CANARY_DATABASE_URL"].startswith(
        "postgres://agent_child_canary_app:"
    )
    for flag in (
        "AGENT_RUNTIME_ENABLED",
        "AGENT_RUNNER_CONTROL_ENABLED",
        "AGENT_ROOT_RUN_CANARY_ENABLED",
        "AGENT_BROKER_ARTIFACT_CANARY_ENABLED",
        "AGENT_PROJECT_MUTATION_CANARY_ENABLED",
        "AGENT_SCHEDULER_ENABLED",
        "AGENT_SKILL_INSTALL_ENABLED",
        "AGENT_LEARNING_ENABLED",
        "AGENT_BROKER_READ_ONLY_ENABLED",
        "AGENT_BROKER_MUTATION_ENABLED",
    ):
        assert env[flag] == "false", flag
    for forbidden in (
        "MCP_RUNNER_TOKEN",
        "S3_ACCESS_KEY_ID",
        "S3_SECRET_ACCESS_KEY",
        "PROVIDER_SECRET_KEYRING_FILE",
        "REDIS_URL",
        "VAULT_ADDR",
        "AGENT_BROKER_CANARY_RELAY_URL",
        "AGENT_PROJECT_CANARY_RELAY_URL",
    ):
        assert forbidden not in env, forbidden
assert development["services"]["agent-runtime-child-canary"]["build"]["target"] == "runtime"
assert "build" not in production["services"]["agent-runtime-child-canary"]
PY

runner_env="${project_dir}/deploy/agent-runner/neo-runnerd.env.example"
rg -Fq 'NEO_RUNNER_CHILD_CANARY_CLIENT_IDENTITY=' "${runner_env}"
for credential in \
  AGENT_CHILD_CANARY_DATABASE_URL \
  AGENT_CHILD_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE \
  AGENT_CHILD_CANARY_PLAN_SOURCE \
  AGENT_DELEGATION_DATABASE_URL; do
  if rg -Fq "${credential}" "${runner_env}"; then
    echo "G21.4 verification: Runner host env contains Child controller credential ${credential}" >&2
    exit 1
  fi
done
python3 - "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-child-run-canary-plan.valid.json" <<'PY'
import json
import sys

plan = json.load(open(sys.argv[1], encoding="utf-8"))
raw = json.dumps(plan, sort_keys=True).lower()
for forbidden in (
    "database_url", "private_key", "s3_secret", "mcp_runner_token",
    "provider_secret", "vault", "redis", "brokerrelay", "egressallowlist",
):
    assert forbidden not in raw, forbidden
assert plan["parentRequestedTools"] == ["delegate_task"]
assert plan["childRequestedTools"] == ["delegate_task"]
assert plan["parentSandbox"]["networkMode"] == "none"
assert plan["childSandbox"]["networkMode"] == "none"
assert plan["parentSandbox"]["capabilities"] == []
assert plan["childSandbox"]["capabilities"] == []
for dimension in ("maxWallSeconds", "maxModelTokens", "maxToolCalls", "maxArtifactBytes"):
    assert plan["childBudget"][dimension] < plan["parentBudget"][dimension]
PY

bash "${script_dir}/test-preflight-single-server.sh"
bash "${script_dir}/verify-agent-child-canary-preflight.sh"
bash "${script_dir}/verify-agent-child-canary-postgres17.sh"

# G21.4 may only add the isolated depth-one Child lane to the complete G21.0-G21.3 chain.
bash "${script_dir}/verify-agent-runtime-g21-3.sh"

set +e
host_output="$(bash "${script_dir}/verify-agent-runner-host.sh" 2>&1)"
host_status=$?
set -e
if [[ "${host_status}" -eq 0 || "${host_output}" != *ISOLATION_UNAVAILABLE* ]]; then
  echo 'G21.4 verification: current host unexpectedly became production-eligible' >&2
  exit 1
fi

echo 'Agent Runtime G21.4 verification: passed; current host remains ISOLATION_UNAVAILABLE.'
