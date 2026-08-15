#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-broker-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
database_name="neo_chat_agent_broker_drill"
database_user="postgres"
database_password="agent-broker-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" "${restore_container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log(){ printf 'Agent Broker PostgreSQL 17 drill: %s\n' "$*"; }
for command in docker openssl; do command -v "${command}" >/dev/null 2>&1 || { echo "${command} is required" >&2; exit 1; }; done
docker version >/dev/null 2>&1 || { echo "Docker is required" >&2; exit 1; }
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null; fi

start_database(){ local name="$1"; docker run -d --rm --name "${name}" -e "POSTGRES_DB=${database_name}" -e "POSTGRES_USER=${database_user}" -e "POSTGRES_PASSWORD=${database_password}" -p 127.0.0.1::5432 "${postgres_image}" >/dev/null; for _ in $(seq 1 60); do docker exec "${name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1 && return; sleep 1; done; exit 1; }
database_url_for(){ local name="$1" port; port="$(docker port "${name}" 5432/tcp | sed -n '1s/.*://p')"; printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable' "${database_user}" "${database_password}" "${port}" "${database_name}"; }
psql_command(){ local name="$1" command="$2"; docker exec -e "PGPASSWORD=${database_password}" "${name}" psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align --username="${database_user}" --dbname="${database_name}" --command "${command}"; }

log "starting disposable database"
start_database "${container_name}"
database_url="$(database_url_for "${container_name}")"
[[ "$(psql_command "${container_name}" 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate(){ MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }

log "applying 001 -> 094, replaying, and peeling the empty product tail"
if ! run_migrate up >"${work_dir}/fresh.log" 2>&1; then
  cat "${work_dir}/fresh.log" >&2
  exit 1
fi
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/fresh.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/fresh.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/fresh.log"
grep -Fq "up 089_agent_draft_learning" "${work_dir}/fresh.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/fresh.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/fresh.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/fresh.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/fresh.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/fresh.log"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"
run_migrate down >"${work_dir}/peel-094-tail-1.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/peel-094-tail-1.log"
run_migrate down >"${work_dir}/peel-093-tail-1.log" 2>&1
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/peel-093-tail-1.log"
run_migrate down >"${work_dir}/peel-092-tail-1.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/peel-092-tail-1.log"
run_migrate down >"${work_dir}/peel-091-tail-1.log" 2>&1
grep -Fq "down 091_agent_artifact_publication" "${work_dir}/peel-091-tail-1.log"
run_migrate down >"${work_dir}/peel-090.log" 2>&1
grep -Fq "down 090_agent_product_shadow" "${work_dir}/peel-090.log"

log "checking effect least privilege"
psql_command "${container_name}" "
DO \$\$
BEGIN
  IF to_regclass('public.agent_effect_intents') IS NULL
     OR NOT has_table_privilege('agent_effect_control','agent_effect_intents','SELECT')
     OR has_table_privilege('agent_effect_control','agent_effect_intents','INSERT')
     OR has_table_privilege('agent_effect_control','agent_effect_intents','UPDATE')
     OR has_table_privilege('go_api_runtime','agent_effect_intents','SELECT')
     OR has_table_privilege('agent_runner_control','agent_effect_intents','SELECT')
     OR has_table_privilege('agent_orchestrator_runtime','agent_effect_intents','SELECT')
     OR NOT has_table_privilege('agent_effect_control','agent_effect_grant_revocations','SELECT')
     OR has_table_privilege('agent_effect_control','agent_effect_grant_revocations','INSERT')
     OR NOT has_table_privilege('agent_effect_control','agent_effect_cancellations','SELECT')
     OR has_table_privilege('agent_effect_control','agent_effect_cancellations','INSERT')
     OR NOT has_function_privilege('agent_effect_control','agent_effect_revoke_grant(text,text,text,text,text)','EXECUTE')
     OR NOT has_function_privilege('agent_effect_control','agent_effect_cancel(text,uuid,text,text,text,text,text)','EXECUTE')
     OR NOT has_function_privilege('agent_effect_control','agent_effect_claim_commit(uuid,text,text,text,bigint,text,text,text,text,text,bigint,text,text,text,text)','EXECUTE') THEN
    RAISE EXCEPTION 'Agent effect grants violate least privilege';
  END IF;
END
\$\$;" >/dev/null

log "running Prepare/approval/Commit/budget/lease integration"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" go test -count=1 -race -run '^TestPostgresAgentBroker' ./internal/agentbroker)

log "creating retained fixture and proving guarded down"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" MM_CHAT_AGENT_BROKER_RETAIN_FIXTURE=1 go test -count=1 -run '^TestPostgresAgentBrokerPrepare' ./internal/agentbroker)
run_migrate down >"${work_dir}/guard-peel-089.log" 2>&1
grep -Fq "down 089_agent_draft_learning" "${work_dir}/guard-peel-089.log"
run_migrate down >"${work_dir}/guard-peel-088.log" 2>&1
grep -Fq "down 088_agent_cron_foundation" "${work_dir}/guard-peel-088.log"
run_migrate down >"${work_dir}/guard-peel-087.log" 2>&1
grep -Fq "down 087_agent_child_delegation" "${work_dir}/guard-peel-087.log"
set +e
run_migrate down >"${work_dir}/guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] || ! grep -Fq 'AGENT_EFFECT_DOWN_DATA_EXISTS' "${work_dir}/guard.log"; then cat "${work_dir}/guard.log" >&2; exit 1; fi

log "dumping and restoring sanitized authority"
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "SELECT concat_ws(',',(SELECT count(*) FROM agent_effect_intents),(SELECT count(*) FROM agent_effect_approvals),(SELECT count(*) FROM agent_effect_receipts),(SELECT count(*) FROM agent_effect_grant_revocations),(SELECT count(*) FROM agent_effect_cancellations),(SELECT count(*) FROM agent_secret_handles));")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "SELECT concat_ws(',',(SELECT count(*) FROM agent_effect_intents),(SELECT count(*) FROM agent_effect_approvals),(SELECT count(*) FROM agent_effect_receipts),(SELECT count(*) FROM agent_effect_grant_revocations),(SELECT count(*) FROM agent_effect_cancellations),(SELECT count(*) FROM agent_secret_handles));")"
[[ "${source_counts}" == "${restore_counts}" ]]

log "proving clean 085 -> 086 -> 085 -> 094"
psql_command "${container_name}" "TRUNCATE agent_effect_grant_revocations;DELETE FROM agent_runs;DELETE FROM agent_run_snapshots;" >/dev/null
run_migrate down >"${work_dir}/down.log" 2>&1
grep -Fq "down 086_agent_broker_foundation" "${work_dir}/down.log"
run_migrate up >"${work_dir}/reup.log" 2>&1
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/reup.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/reup.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/reup.log"
grep -Fq "up 089_agent_draft_learning" "${work_dir}/reup.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/reup.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/reup.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/reup.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/reup.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/reup.log"
run_migrate up >"${work_dir}/final.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/final.log"
log "passed (fresh/replay, least privilege, concurrency, fences, guarded down, dump/restore, clean down/up)"
