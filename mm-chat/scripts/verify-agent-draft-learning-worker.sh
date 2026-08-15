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

for command in docker go python3 rg; do command -v "${command}" >/dev/null || { echo "Draft worker gate: ${command} is required" >&2; exit 1; }; done
for path in \
  "${backend_dir}/cmd/agent-runtime-draft-learning-worker/main.go" \
  "${backend_dir}/internal/agentlearningworker/plan.go" \
  "${backend_dir}/internal/agentlearningworker/checker.go" \
  "${backend_dir}/internal/agentlearningworker/repository_postgres.go" \
  "${backend_dir}/internal/agentlearningworker/service.go" \
  "${project_dir}/docs/contracts/schemas/neo-agent-draft-learning-worker-activation.schema.json" \
  "${project_dir}/docs/contracts/schemas/neo-agent-draft-learning-worker-plan.schema.json" \
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-runner-rpc.result.valid.json"; do
  [[ -s "${path}" ]] || { echo "Draft worker gate: missing ${path}" >&2; exit 1; }
done
if rg -n 'agentlearningworker|agent-runtime-draft-learning-worker|AGENT_DRAFT_LEARNING_WORKER_ENABLED' \
  "${backend_dir}/cmd/api" >/dev/null; then
  echo 'Draft worker gate: exact worker leaked into cmd/api' >&2; exit 1
fi
for signature in \
  'CREATE FUNCTION agent_learning_worker_claim_checks(' \
  'CREATE FUNCTION agent_learning_worker_begin_runner_check(' \
  'CREATE FUNCTION agent_learning_worker_record_runner_result(' \
  'CREATE FUNCTION agent_learning_worker_complete_runner_cleanup(' \
  'CREATE FUNCTION agent_learning_worker_claim_cleanup(' \
  'CREATE FUNCTION agent_learning_worker_reconcile_runner(' \
  'TO agent_learning_worker'; do
  rg -Fq "${signature}" "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql" || {
    echo "Draft worker gate: missing migration signature ${signature}" >&2; exit 1;
  }
done
python3 - "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql" <<'PY'
import sys
statements=open(sys.argv[1]).read().split(';')
grants=[value for value in statements if 'GRANT' in value and 'TO agent_learning_worker' in value]
for statement in grants:
    assert not any(token in statement for token in (
        'GRANT SELECT ON agent_learning_', 'GRANT INSERT ON agent_learning_',
        'GRANT UPDATE ON agent_learning_', 'GRANT DELETE ON agent_learning_',
        'agent_learning_promote(', 'agent_learning_reject(', 'agent_learning_create_draft(',
        'agent_learning_worker_provision_target(', 'agent_learning_worker_disable_target(',
    )), statement
PY

python3 - "${backend_dir}/cmd/neo-runnerd/main.go" <<'PY'
import sys
source=open(sys.argv[1]).read()
start=source.index('draftIdentity := strings.TrimSpace')
end=source.index('productIdentity := strings.TrimSpace',start)
block=source[start:end]
for method in ('MethodProbe','MethodList','MethodReconcile','MethodLaunch','MethodResult','MethodCancel'):
    assert method in block,method
for forbidden in ('MethodHeartbeat','MethodPrepare','MethodCommit','WithBrokerRelayForCaller'):
    assert forbidden not in block,forbidden
prefix=source[:start]
assert 'MethodResult' not in prefix
PY
rg -Fq 'NEO_RUNNER_DRAFT_LEARNING_CLIENT_IDENTITY=' "${project_dir}/deploy/agent-runner/neo-runnerd.env.example"
for credential in AGENT_DRAFT_LEARNING_WORKER_DATABASE_URL AGENT_DRAFT_LEARNING_WORKER_S3_ACCESS_KEY_ID \
  AGENT_DRAFT_LEARNING_WORKER_S3_SECRET_ACCESS_KEY AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PRIVATE_KEY_SOURCE; do
  if rg -Fq "${credential}" "${project_dir}/deploy/agent-runner/neo-runnerd.env.example"; then
    echo "Draft worker gate: Runner host env contains controller credential ${credential}" >&2; exit 1
  fi
done

(
  cd "${backend_dir}"
  go test -race ./cmd/agent-runtime-draft-learning-worker ./cmd/neo-runnerd \
    ./internal/agentactivation ./internal/agentlearningworker ./internal/agentlearning \
    ./internal/agentrunner ./internal/migration
  go vet ./cmd/agent-runtime-draft-learning-worker ./cmd/neo-runnerd \
    ./internal/agentactivation ./internal/agentlearningworker ./internal/agentlearning \
    ./internal/agentrunner ./internal/migration
)

for mode in development production; do
  args=(docker compose --project-directory "${project_dir}" --env-file "${example_env}" -f "${project_dir}/compose.single-server.yml")
  [[ "${mode}" == production ]] && args+=( -f "${project_dir}/compose.production.yml" )
  "${args[@]}" --profile agent-runtime-draft-learning-worker config --format json >"${work_dir}/${mode}.json"
done
docker compose --project-directory "${project_dir}" --env-file "${example_env}" \
  -f "${project_dir}/compose.single-server.yml" config --format json >"${work_dir}/default.json"
python3 - "${work_dir}/development.json" "${work_dir}/production.json" "${work_dir}/default.json" <<'PY'
import json,sys
development,production,default=(json.load(open(path)) for path in sys.argv[1:])
assert 'agent-runtime-draft-learning-worker' not in default.get('services',{})
for config in (development,production):
    service=config['services']['agent-runtime-draft-learning-worker']
    assert service['profiles']==['agent-runtime-draft-learning-worker']
    assert service['read_only'] is True and service['cap_drop']==['ALL']
    assert not service.get('ports') and not service.get('secrets')
    assert set(service['networks'])=={'private'}
    assert all(v['read_only'] and not v['bind']['create_host_path'] for v in service['volumes'])
    env=service['environment']
    assert env['AGENT_DRAFT_LEARNING_WORKER_ENABLED']=='false'
    assert env['AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY']=='spiffe://neo-chat/agent-runtime-draft-learning'
    assert env['S3_BUCKET_AUTO_CREATE']=='false'
    for flag in ('AGENT_RUNTIME_ENABLED','AGENT_SCHEDULER_ENABLED','AGENT_LEARNING_ENABLED',
                 'AGENT_SKILL_INSTALL_ENABLED','AGENT_BROKER_READ_ONLY_ENABLED',
                 'AGENT_BROKER_MUTATION_ENABLED','AGENT_DELEGATION_ENABLED'):
        assert env[flag]=='false',flag
    for forbidden in ('MCP_RUNNER_TOKEN','PROVIDER_SECRET_KEYRING_FILE','VAULT_ADDR','REDIS_URL',
                      'AGENT_BROKER_CANARY_RELAY_URL','AGENT_PROJECT_CANARY_RELAY_URL'):
        assert forbidden not in env,forbidden
    assert env['AGENT_DRAFT_LEARNING_WORKER_DATABASE_URL'].startswith('postgres://agent_learning_worker_app:')
    assert 'S3_ACCESS_KEY_ID' in env and 'S3_SECRET_ACCESS_KEY' in env
assert development['services']['agent-runtime-draft-learning-worker']['build']['target']=='runtime'
assert 'build' not in production['services']['agent-runtime-draft-learning-worker']
PY

python3 - "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-draft-learning-worker-plan.valid.json" <<'PY'
import json,sys
plan=json.load(open(sys.argv[1]))
assert plan['toolRegistry']['tools']==[] and plan['toolRegistry']['depth']==0
assert plan['sandbox']['networkMode']=='none' and plan['sandbox']['capabilities']==[]
assert plan['sandbox']['rootfsReadOnly'] is True and plan['sandbox']['noNewPrivileges'] is True
assert [item['kind'] for item in plan['checks']]==['isolation','evaluation']
assert plan['callerIdentity']=='spiffe://neo-chat/agent-runtime-draft-learning'
PY
printf '%s\n' 'Agent Runtime exact Draft-learning worker, Runner ACL and Compose verification: passed.'
