#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
example_env="${project_dir}/.env.single-server.example"
work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

for command in bash docker go jq python3 rg; do
  command -v "${command}" >/dev/null 2>&1 || { echo "G21.2 verification: ${command} is required" >&2; exit 1; }
done
for path in \
  "${backend_dir}/cmd/agent-runtime-broker-canary/main.go" \
  "${backend_dir}/internal/agentbrokercanary/service.go" \
  "${backend_dir}/internal/agentbrokerrelay/handler.go" \
  "${backend_dir}/migrations/091_agent_artifact_publication.up.sql" \
  "${backend_dir}/migrations/091_agent_artifact_publication.down.sql"; do
  [[ -s "${path}" ]] || { echo "G21.2 verification: missing ${path}" >&2; exit 1; }
done
if rg -n 'agentbrokercanary|agentbrokerrelay|agent-runtime-broker-canary' "${backend_dir}/cmd/api" >/dev/null; then
  echo 'G21.2 verification: Broker canary leaked into cmd/api' >&2
  exit 1
fi
if [[ "$(find "${backend_dir}/migrations" -maxdepth 1 -name '091_*.up.sql' -printf '%f\n')" != \
  '091_agent_artifact_publication.up.sql' ]]; then
  echo 'G21.2 verification: migration 091 tail is missing or widened' >&2
  exit 1
fi
for signature in \
  'agent_artifact_authorize(' 'agent_artifact_attach(' 'agent_artifact_control' \
  'agent_orchestrator_artifact_fence(' 'REVOKE ALL ON agent_artifacts'; do
  rg -Fq "${signature}" "${backend_dir}/migrations/091_agent_artifact_publication.up.sql" || {
    echo "G21.2 verification: Artifact authority signature missing: ${signature}" >&2; exit 1;
  }
done
for signature in \
  'MethodPrepare' 'MethodCommit' 'handler.authority.Verify' 'Retryable: false'; do
  rg -Fq "${signature}" "${backend_dir}/internal/agentbrokerrelay/handler.go" || {
    echo "G21.2 verification: relay signature missing: ${signature}" >&2; exit 1;
  }
done
for forbidden in MethodLaunch MethodHeartbeat MethodCancel MethodProbe MethodList MethodReconcile; do
  if rg -n "case agentrunner.${forbidden}" "${backend_dir}/internal/agentbrokerrelay" >/dev/null; then
    echo "G21.2 verification: relay widened to ${forbidden}" >&2
    exit 1
  fi
done
rg -Fq 'PossibleSend: true' "${backend_dir}/internal/agentbrokercanary/executors.go"
rg -Fq 'agentrunner.ErrOutcomeUnknown' "${backend_dir}/internal/agentbrokercanary/service.go"

bash "${script_dir}/verify-agent-broker-canary-activation.sh"
(
  umask 022
  cd "${backend_dir}"
  go test -race \
    ./cmd/agent-runtime-broker-canary \
    ./cmd/neo-runnerd \
    ./internal/agentactivation \
    ./internal/agentbroker \
    ./internal/agentbrokercanary \
    ./internal/agentbrokerrelay \
    ./internal/agentorchestrator \
    ./internal/agentrunner
  go vet \
    ./cmd/agent-runtime-broker-canary \
    ./cmd/neo-runnerd \
    ./internal/agentactivation \
    ./internal/agentbroker \
    ./internal/agentbrokercanary \
    ./internal/agentbrokerrelay \
    ./internal/agentorchestrator \
    ./internal/agentrunner
)

for mode in development production; do
  args=(docker compose --project-directory "${project_dir}" --env-file "${example_env}" -f "${project_dir}/compose.single-server.yml")
  [[ "${mode}" == production ]] && args+=( -f "${project_dir}/compose.production.yml" )
  "${args[@]}" --profile mcp-runner --profile agent-runtime-broker-canary config --format json \
    >"${work_dir}/${mode}.json"
done
docker compose --project-directory "${project_dir}" --env-file "${example_env}" \
  -f "${project_dir}/compose.single-server.yml" config --format json >"${work_dir}/default.json"
python3 - "${work_dir}/development.json" "${work_dir}/production.json" "${work_dir}/default.json" <<'PY'
import json,sys
development,production,default=(json.load(open(path,encoding="utf-8")) for path in sys.argv[1:])
assert "agent-runtime-broker-canary" not in default.get("services",{})
for config in (development,production):
    service=config["services"]["agent-runtime-broker-canary"]
    assert service["profiles"]==["agent-runtime-broker-canary"]
    assert "ports" not in service and service["read_only"] is True and service["init"] is True
    assert service["cap_drop"]==["ALL"] and "no-new-privileges:true" in service["security_opt"]
    assert set(service["networks"])=={"private","mcp-control","agent-broker-relay"}
    assert service["networks"]["agent-broker-relay"]["ipv4_address"]=="172.31.254.2"
    assert service["secrets"]==[{"source":"mm_chat_mcp_runner_token","target":"mm_chat_mcp_runner_token"}]
    env=service["environment"]
    assert env["AGENT_BROKER_ARTIFACT_CANARY_ENABLED"]=="false"
    for flag in ("AGENT_RUNTIME_ENABLED","AGENT_SCHEDULER_ENABLED","AGENT_SKILL_INSTALL_ENABLED","AGENT_LEARNING_ENABLED","AGENT_DELEGATION_ENABLED","AGENT_BROKER_READ_ONLY_ENABLED","AGENT_BROKER_MUTATION_ENABLED"):
        assert env[flag]=="false",flag
    assert env["S3_BUCKET_AUTO_CREATE"]=="false"
    assert env["AGENT_BROKER_CANARY_MCP_RUNNER_TOKEN_FILE"]=="/run/secrets/mm_chat_mcp_runner_token"
    for forbidden in ("DATABASE_URL","MIGRATION_DATABASE_URL","MCP_RUNNER_TOKEN","PROVIDER_SECRET_KEYRING_FILE","REDIS_URL"):
        assert forbidden not in env,forbidden
    assert len(service["volumes"])==15
    assert sum(volume.get("read_only") is not True for volume in service["volumes"])==1
    assert all(volume["bind"]["create_host_path"] is False for volume in service["volumes"])
assert development["services"]["agent-runtime-broker-canary"]["build"]["target"]=="runtime"
assert "build" not in production["services"]["agent-runtime-broker-canary"]
PY

for credential in AGENT_BROKER_CANARY_DATABASE_URL S3_SECRET_ACCESS_KEY MCP_RUNNER_TOKEN_SOURCE \
  AGENT_BROKER_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE AGENT_BROKER_CANARY_RELAY_TLS_KEY_SOURCE; do
  if rg -Fq "${credential}" "${project_dir}/deploy/agent-runner/neo-runnerd.env.example"; then
    echo "G21.2 verification: Runner host env contains Broker credential ${credential}" >&2
    exit 1
  fi
done
for relay_setting in NEO_RUNNER_BROKER_RELAY_URL NEO_RUNNER_BROKER_RELAY_CLIENT_CERT_FILE \
  NEO_RUNNER_BROKER_RELAY_CLIENT_KEY_FILE NEO_RUNNER_BROKER_RELAY_SERVER_CA_FILE \
  NEO_RUNNER_BROKER_RELAY_CLIENT_IDENTITY; do
  rg -Fq "${relay_setting}=" "${project_dir}/deploy/agent-runner/neo-runnerd.env.example"
done
python3 - "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-broker-artifact-canary-plan.valid.json" <<'PY'
import json,sys
plan=json.load(open(sys.argv[1],encoding="utf-8"))
raw=json.dumps(plan,sort_keys=True).lower()
for forbidden in ("database_url","s3_secret","mcp_runner_token","private_key","provider_secret","vault"):
    assert forbidden not in raw,forbidden
assert plan["sandbox"]["networkMode"]=="none" and plan["sandbox"]["capabilities"]==[]
PY

bash "${script_dir}/test-preflight-single-server.sh"
bash "${script_dir}/verify-agent-broker-canary-preflight.sh"
bash "${script_dir}/verify-agent-artifact-publication-postgres17.sh"

# G21.2 may only narrow the already-reviewed G21.0/G21.1 production seams.
bash "${script_dir}/verify-agent-runtime-g21-1.sh"

set +e
host_output="$(bash "${script_dir}/verify-agent-runner-host.sh" 2>&1)"; host_status=$?
set -e
if [[ "${host_status}" -eq 0 || "${host_output}" != *ISOLATION_UNAVAILABLE* ]]; then
  echo 'G21.2 verification: current host unexpectedly became production-eligible' >&2
  exit 1
fi

echo 'Agent Runtime G21.2 verification: passed; current host remains ISOLATION_UNAVAILABLE.'
