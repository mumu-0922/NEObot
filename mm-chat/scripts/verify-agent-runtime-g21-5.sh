#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"

for command in bash docker go jq python3 rg; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "G21.5 verification: ${command} is required" >&2
    exit 1
  }
done

for path in \
  "${backend_dir}/cmd/agent-runtime-cron-worker/main.go" \
  "${backend_dir}/cmd/agent-runtime-draft-learning-worker/main.go" \
  "${backend_dir}/internal/agentactivation/cron_learning_workers.go" \
  "${backend_dir}/internal/agentcronworker/service.go" \
  "${backend_dir}/internal/agentlearningworker/service.go" \
  "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql" \
  "${backend_dir}/migrations/094_agent_cron_learning_activation.down.sql" \
  "${project_dir}/docs/contracts/schemas/neo-agent-cron-worker-activation.schema.json" \
  "${project_dir}/docs/contracts/schemas/neo-agent-cron-worker-plan.schema.json" \
  "${project_dir}/docs/contracts/schemas/neo-agent-draft-learning-worker-activation.schema.json" \
  "${project_dir}/docs/contracts/schemas/neo-agent-draft-learning-worker-plan.schema.json" \
  "${script_dir}/verify-agent-cron-worker.sh" \
  "${script_dir}/verify-agent-cron-worker-postgres17.sh" \
  "${script_dir}/verify-agent-draft-learning-worker.sh" \
  "${script_dir}/verify-agent-draft-learning-worker-postgres17.sh" \
  "${script_dir}/verify-agent-runtime-g21-5-preflight.sh"; do
  [[ -s "${path}" ]] || {
    echo "G21.5 verification: missing ${path}" >&2
    exit 1
  }
done

if rg -n 'agentcronworker|agentlearningworker|agent-runtime-(cron-worker|draft-learning)' \
  "${backend_dir}/cmd/api" >/dev/null; then
  echo 'G21.5 verification: Cron or Draft-learning worker leaked into cmd/api' >&2
  exit 1
fi
if [[ "$(find "${backend_dir}/migrations" -maxdepth 1 -name '094_*.up.sql' -printf '%f\n')" != \
  '094_agent_cron_learning_activation.up.sql' ]]; then
  echo 'G21.5 verification: migration 094 tail is missing or widened' >&2
  exit 1
fi

for signature in \
  'CREATE TABLE agent_cron_worker_targets(' \
  'CREATE TABLE agent_learning_worker_targets(' \
  'CREATE TABLE agent_learning_runner_attempts(' \
  'CREATE TABLE agent_learning_runner_results(' \
  'CREATE TABLE agent_learning_runner_requests(' \
  'CREATE FUNCTION agent_cron_worker_claim_due(' \
  'CREATE FUNCTION agent_learning_worker_claim_checks(' \
  'CREATE FUNCTION agent_learning_worker_issue_runner_authority(' \
  'CREATE FUNCTION agent_learning_worker_record_runner_result(' \
  'CREATE FUNCTION agent_learning_worker_complete_checks(' \
  'TO agent_cron_worker;' \
  'TO agent_learning_worker;'; do
  rg -Fq "${signature}" "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql" || {
    echo "G21.5 verification: migration authority signature missing: ${signature}" >&2
    exit 1
  }
done
for forbidden in \
  'GRANT SELECT ON agent_cron_worker_targets TO agent_cron_worker' \
  'GRANT SELECT ON agent_learning_worker_targets TO agent_learning_worker' \
  'GRANT agent_cron_control TO agent_cron_worker' \
  'GRANT agent_learning_control TO agent_learning_worker' \
  'agent_learning_worker_provision_target(TEXT,TEXT,UUID' \
  'agent_learning_promote(TEXT,UUID'; do
  if [[ "${forbidden}" == agent_learning_worker_provision_target* || \
        "${forbidden}" == agent_learning_promote* ]]; then
    if sed -n '/GRANT EXECUTE ON FUNCTION/,/TO agent_learning_worker;/p' \
      "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql" | \
      rg -Fq "${forbidden}"; then
      echo "G21.5 verification: Draft worker authority widened: ${forbidden}" >&2
      exit 1
    fi
  elif rg -Fiq "${forbidden}" "${backend_dir}/migrations/094_agent_cron_learning_activation.up.sql"; then
    echo "G21.5 verification: worker authority widened: ${forbidden}" >&2
    exit 1
  fi
done
for guard in \
  AGENT_WORKER_DOWN_ACTIVE_TARGET \
  AGENT_WORKER_DOWN_LIVE_CLAIM \
  AGENT_WORKER_DOWN_UNRESOLVED_RUNNER_ATTEMPT \
  AGENT_WORKER_DOWN_CLEANUP_PENDING \
  AGENT_WORKER_DOWN_LOGIN_MEMBERSHIP \
  AGENT_WORKER_DOWN_RETAINED_ACTIVATION_FACTS; do
  rg -Fq "${guard}" "${backend_dir}/migrations/094_agent_cron_learning_activation.down.sql"
done

bash "${script_dir}/verify-agent-cron-worker.sh"
bash "${script_dir}/verify-agent-draft-learning-worker.sh"
bash "${script_dir}/verify-agent-runtime-g21-5-preflight.sh"
bash "${script_dir}/verify-agent-cron-worker-postgres17.sh"
bash "${script_dir}/verify-agent-draft-learning-worker-postgres17.sh"

# G21.5 may only add the two independent exact-target workers to the complete
# G21.0-G21.4 production-path chain.
bash "${script_dir}/verify-agent-runtime-g21-4.sh"

set +e
host_output="$(bash "${script_dir}/verify-agent-runner-host.sh" 2>&1)"
host_status=$?
set -e
if [[ "${host_status}" -eq 0 || "${host_output}" != *ISOLATION_UNAVAILABLE* ]]; then
  echo 'G21.5 verification: current host unexpectedly became production-eligible' >&2
  exit 1
fi

echo 'Agent Runtime G21.5 verification: passed; current host remains ISOLATION_UNAVAILABLE.'
