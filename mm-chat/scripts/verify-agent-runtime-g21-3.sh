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
    echo "G21.3 verification: ${command} is required" >&2
    exit 1
  }
done

for path in \
  "${backend_dir}/cmd/agent-runtime-project-canary/main.go" \
  "${backend_dir}/internal/agentactivation/project_canary.go" \
  "${backend_dir}/internal/agentprojectcanary/service.go" \
  "${backend_dir}/internal/agentbroker/project_mutation.go" \
  "${backend_dir}/internal/agentbroker/project_mutation_postgres.go" \
  "${backend_dir}/migrations/092_agent_project_mutation_canary.up.sql" \
  "${backend_dir}/migrations/092_agent_project_mutation_canary.down.sql"; do
  [[ -s "${path}" ]] || { echo "G21.3 verification: missing ${path}" >&2; exit 1; }
done
if rg -n 'agentprojectcanary|agent-runtime-project-canary|project_mutation_canary' \
  "${backend_dir}/cmd/api" >/dev/null; then
  echo 'G21.3 verification: Project canary leaked into cmd/api' >&2
  exit 1
fi
if [[ "$(find "${backend_dir}/migrations" -maxdepth 1 -name '092_*.up.sql' -printf '%f\n')" != \
  '092_agent_project_mutation_canary.up.sql' ]]; then
  echo 'G21.3 verification: migration 092 tail is missing or widened' >&2
  exit 1
fi

for signature in \
  'agent_project_canary_provision(' 'agent_project_mutation_commit(' \
  'agent_project_mutation_status(' 'agent_project_mutation_cleanup(' \
  'agent_effect_project_mutation_fence(' 'agent_orchestrator_project_mutation_fence(' \
  "v_intent.approval_class<>'per_commit'" 'agent_effect_grant_revocations' \
  'agent_orchestrator_active_kill_mode(v_run.scope_keys)' \
  'TO agent_project_mutation_control' 'REVOKE ALL ON agent_project_canary_resources'; do
  rg -Fq "${signature}" "${backend_dir}/migrations/092_agent_project_mutation_canary.up.sql" || {
    echo "G21.3 verification: Project authority signature missing: ${signature}" >&2
    exit 1
  }
done
for forbidden in \
  'GRANT INSERT ON agent_project_canary_resources' \
  'GRANT UPDATE ON agent_project_canary_resources' \
  'GRANT DELETE ON agent_project_canary_resources' \
  'GRANT agent_project_mutation_owner TO agent_project_mutation_control'; do
  if rg -Fiq "${forbidden}" "${backend_dir}/migrations/092_agent_project_mutation_canary.up.sql"; then
    echo "G21.3 verification: Project authority widened: ${forbidden}" >&2
    exit 1
  fi
done
for signature in \
  'ActivationBindingFingerprint(' 'LoadApproval(' 'ApprovalPerCommit' \
  'project.patch' 'project.write' 'apply_patch' 'ReconcileCleanup('; do
  if ! rg -Fq "${signature}" \
    "${backend_dir}/cmd/agent-runtime-project-canary" \
    "${backend_dir}/internal/agentprojectcanary" \
    "${backend_dir}/internal/agentbroker/project_mutation.go"; then
    echo "G21.3 verification: one-action controller signature missing: ${signature}" >&2
    exit 1
  fi
done
for signature in \
  'projectCanaryCallerIdentity = "spiffe://neo-chat/agent-runtime-project-canary"' \
  'projectRelayIdentity        = "spiffe://neo-chat/neo-runner-project-relay"' \
  'service.WithBrokerRelayForCaller(projectIdentity, relay)' \
  'service.WithBrokerRelayForCaller(brokerIdentity, relay)' \
  'agentrunner.MethodPrepare, agentrunner.MethodCommit'; do
  rg -Fq "${signature}" "${backend_dir}/cmd/neo-runnerd/main.go" || {
    echo "G21.3 verification: caller-specific Runner routing signature missing: ${signature}" >&2
    exit 1
  }
done

bash "${script_dir}/verify-agent-project-canary-activation.sh"
(
  umask 022
  cd "${backend_dir}"
  go test -race \
    ./cmd/agent-runtime-project-canary \
    ./cmd/neo-runnerd \
    ./internal/agentactivation \
    ./internal/agentbroker \
    ./internal/agentbrokerrelay \
    ./internal/agentorchestrator \
    ./internal/agentprojectcanary \
    ./internal/agentrunner
  go vet \
    ./cmd/agent-runtime-project-canary \
    ./cmd/neo-runnerd \
    ./internal/agentactivation \
    ./internal/agentbroker \
    ./internal/agentbrokerrelay \
    ./internal/agentorchestrator \
    ./internal/agentprojectcanary \
    ./internal/agentrunner
)

for mode in development production; do
  args=(docker compose --project-directory "${project_dir}" --env-file "${example_env}" \
    -f "${project_dir}/compose.single-server.yml")
  [[ "${mode}" == production ]] && args+=( -f "${project_dir}/compose.production.yml" )
  "${args[@]}" --profile agent-runtime-project-canary config --format json \
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
assert "agent-runtime-project-canary" not in default.get("services", {})
for config in (development, production):
    service = config["services"]["agent-runtime-project-canary"]
    assert service["profiles"] == ["agent-runtime-project-canary"]
    assert "ports" not in service and service["read_only"] is True and service["init"] is True
    assert service["cap_drop"] == ["ALL"]
    assert "no-new-privileges:true" in service["security_opt"]
    assert set(service["networks"]) == {"private", "agent-project-relay"}
    assert service["networks"]["agent-project-relay"]["ipv4_address"] == "172.31.254.10"
    assert service.get("secrets", []) == []
    assert len(service["volumes"]) == 14
    assert all(volume["read_only"] is True for volume in service["volumes"])
    assert all(volume["bind"]["create_host_path"] is False for volume in service["volumes"])
    env = service["environment"]
    assert env["AGENT_PROJECT_MUTATION_CANARY_ENABLED"] == "false"
    assert env["AGENT_PROJECT_CANARY_DATABASE_URL"].startswith(
        "postgres://agent_project_canary_app:"
    )
    for flag in (
        "AGENT_RUNTIME_ENABLED",
        "AGENT_RUNNER_CONTROL_ENABLED",
        "AGENT_ROOT_RUN_CANARY_ENABLED",
        "AGENT_BROKER_ARTIFACT_CANARY_ENABLED",
        "AGENT_SCHEDULER_ENABLED",
        "AGENT_SKILL_INSTALL_ENABLED",
        "AGENT_LEARNING_ENABLED",
        "AGENT_DELEGATION_ENABLED",
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
    ):
        assert forbidden not in env, forbidden
assert development["services"]["agent-runtime-project-canary"]["build"]["target"] == "runtime"
assert "build" not in production["services"]["agent-runtime-project-canary"]
PY

runner_env="${project_dir}/deploy/agent-runner/neo-runnerd.env.example"
for credential in \
  AGENT_PROJECT_CANARY_DATABASE_URL \
  AGENT_PROJECT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE \
  AGENT_PROJECT_CANARY_APPROVAL_DOCUMENT_SOURCE \
  AGENT_PROJECT_CANARY_RELAY_TLS_KEY_SOURCE; do
  if rg -Fq "${credential}" "${runner_env}"; then
    echo "G21.3 verification: Runner host env contains Project credential ${credential}" >&2
    exit 1
  fi
done
for relay_setting in \
  NEO_RUNNER_PROJECT_CANARY_CLIENT_IDENTITY \
  NEO_RUNNER_PROJECT_RELAY_URL \
  NEO_RUNNER_PROJECT_RELAY_CLIENT_CERT_FILE \
  NEO_RUNNER_PROJECT_RELAY_CLIENT_KEY_FILE \
  NEO_RUNNER_PROJECT_RELAY_SERVER_CA_FILE \
  NEO_RUNNER_PROJECT_RELAY_SERVER_NAME \
  NEO_RUNNER_PROJECT_RELAY_CLIENT_IDENTITY; do
  rg -Fq "${relay_setting}=" "${runner_env}"
done
python3 - "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-project-mutation-canary-plan.valid.json" <<'PY'
import json
import sys

plan = json.load(open(sys.argv[1], encoding="utf-8"))
raw = json.dumps(plan, sort_keys=True).lower()
for forbidden in (
    "database_url",
    "private_key",
    "approval_document",
    "s3_secret",
    "mcp_runner_token",
    "provider_secret",
    "vault",
):
    assert forbidden not in raw, forbidden
assert plan["sandbox"]["networkMode"] == "none"
assert plan["sandbox"]["capabilities"] == []
action = plan["action"]
assert action["toolIdentity"] == "project.patch"
assert action["capability"] == "project.write"
assert action["action"] == "apply_patch"
assert action["approval"] == "per_commit"
assert action["idempotent"] is False
PY

bash "${script_dir}/test-preflight-single-server.sh"
bash "${script_dir}/verify-agent-project-canary-preflight.sh"
bash "${script_dir}/verify-agent-project-mutation-postgres17.sh"

# Phase 0 owns the complete G21.0-G21.2 regression chain and offline contracts.
bash "${script_dir}/verify-agent-runtime-phase0.sh"

set +e
host_output="$(bash "${script_dir}/verify-agent-runner-host.sh" 2>&1)"
host_status=$?
set -e
if [[ "${host_status}" -eq 0 || "${host_output}" != *ISOLATION_UNAVAILABLE* ]]; then
  echo 'G21.3 verification: current host unexpectedly became production-eligible' >&2
  exit 1
fi

echo 'Agent Runtime G21.3 verification: passed; current host remains ISOLATION_UNAVAILABLE.'
