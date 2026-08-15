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

for command in docker go python3 rg; do command -v "${command}" >/dev/null || { echo "Cron worker gate: ${command} is required" >&2; exit 1; }; done
for path in \
  "${backend_dir}/cmd/agent-runtime-cron-worker/main.go" \
  "${backend_dir}/internal/agentcronworker/plan.go" \
  "${backend_dir}/internal/agentcronworker/service.go" \
  "${backend_dir}/internal/agentcronworker/target_postgres.go" \
  "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql" \
  "${backend_dir}/migrations/094_agent_cron_learning_activation.down.sql" \
  "${project_dir}/docs/contracts/schemas/neo-agent-cron-worker-activation.schema.json" \
  "${project_dir}/docs/contracts/schemas/neo-agent-cron-worker-plan.schema.json"; do
  [[ -s "${path}" ]] || { echo "Cron worker gate: missing ${path}" >&2; exit 1; }
done
if rg -n 'agentcronworker|agent-runtime-cron-worker|AGENT_CRON_WORKER_ENABLED' "${backend_dir}/cmd/api" >/dev/null; then
  echo 'Cron worker gate: exact worker leaked into cmd/api' >&2; exit 1
fi
for signature in \
  'CREATE ROLE %I NOLOGIN' "ARRAY['agent_cron_worker','agent_learning_worker']" \
  'CREATE FUNCTION agent_cron_worker_claim_due(' \
  'FROM agent_cron_worker_targets target' \
  'FOR UPDATE OF template SKIP LOCKED' \
  'CREATE FUNCTION agent_cron_worker_reconcile(' \
  'CREATE FUNCTION agent_cron_worker_prune(' \
  'GRANT EXECUTE ON FUNCTION' \
  'TO agent_cron_worker'; do
  rg -Fq "${signature}" "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql" || {
    echo "Cron worker gate: missing migration signature ${signature}" >&2; exit 1;
  }
done
python3 - "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql" <<'PY'
import sys
statements=open(sys.argv[1]).read().split(';')
grants=[value for value in statements if 'GRANT' in value and 'TO agent_cron_worker' in value]
for statement in grants:
    assert not any(token in statement for token in (
        'GRANT SELECT ON agent_cron_', 'GRANT INSERT ON agent_cron_',
        'GRANT UPDATE ON agent_cron_', 'GRANT DELETE ON agent_cron_',
        'agent_cron_worker_provision_target(', 'agent_cron_worker_disable_target(',
    )), statement
PY

(
  cd "${backend_dir}"
  go test -race ./cmd/agent-runtime-cron-worker ./internal/agentactivation \
    ./internal/agentcronworker ./internal/agentcron ./internal/migration
  go vet ./cmd/agent-runtime-cron-worker ./internal/agentactivation \
    ./internal/agentcronworker ./internal/agentcron ./internal/migration
)

for mode in development production; do
  args=(docker compose --project-directory "${project_dir}" --env-file "${example_env}" -f "${project_dir}/compose.single-server.yml")
  [[ "${mode}" == production ]] && args+=( -f "${project_dir}/compose.production.yml" )
  "${args[@]}" --profile agent-runtime-cron-worker config --format json >"${work_dir}/${mode}.json"
done
docker compose --project-directory "${project_dir}" --env-file "${example_env}" \
  -f "${project_dir}/compose.single-server.yml" config --format json >"${work_dir}/default.json"
python3 - "${work_dir}/development.json" "${work_dir}/production.json" "${work_dir}/default.json" <<'PY'
import json,sys
development,production,default=(json.load(open(path)) for path in sys.argv[1:])
assert 'agent-runtime-cron-worker' not in default.get('services',{})
for config in (development,production):
    service=config['services']['agent-runtime-cron-worker']
    assert service['profiles']==['agent-runtime-cron-worker']
    assert service['read_only'] is True and service['cap_drop']==['ALL']
    assert not service.get('ports') and not service.get('secrets')
    assert set(service['networks'])=={'private'}
    assert all(v['read_only'] and not v['bind']['create_host_path'] for v in service['volumes'])
    env=service['environment']
    assert env['AGENT_CRON_WORKER_ENABLED']=='false'
    for flag in ('AGENT_RUNTIME_ENABLED','AGENT_SCHEDULER_ENABLED','AGENT_LEARNING_ENABLED',
                 'AGENT_SKILL_INSTALL_ENABLED','AGENT_BROKER_READ_ONLY_ENABLED',
                 'AGENT_BROKER_MUTATION_ENABLED','AGENT_DELEGATION_ENABLED'):
        assert env[flag]=='false',flag
    for marker in ('RUNNER','S3_','OBJECT','MCP','PROVIDER','VAULT','REDIS','ADMIN','PROMOTE','SECRET'):
        assert not any(marker in key for key in env),marker
    assert env['AGENT_CRON_WORKER_DATABASE_URL'].startswith('postgres://agent_cron_worker_app:')
assert development['services']['agent-runtime-cron-worker']['build']['target']=='runtime'
assert 'build' not in production['services']['agent-runtime-cron-worker']
PY

for credential in AGENT_CRON_WORKER_DATABASE_URL AGENT_CRON_WORKER_PLAN_SOURCE \
  AGENT_CRON_WORKER_ACTIVATION_SOURCE AGENT_CRON_WORKER_RUNNER_TOKEN; do
  if rg -Fq "${credential}" "${project_dir}/deploy/agent-runner/neo-runnerd.env.example"; then
    echo "Cron worker gate: Runner host env contains ${credential}" >&2; exit 1
  fi
done
printf '%s\n' 'Agent Runtime exact Cron worker source, role and Compose verification: passed.'
