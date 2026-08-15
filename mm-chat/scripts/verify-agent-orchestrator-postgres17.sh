#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-orchestrator-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
database_name="neo_chat_agent_orchestrator_drill"
database_user="postgres"
database_password="agent-orchestrator-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" "${restore_container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
trap cleanup EXIT INT TERM

log() { printf 'Agent Orchestrator PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Agent Orchestrator PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Agent Orchestrator PostgreSQL 17 drill: Docker is required" >&2
  exit 1
}
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  log "building the project PostgreSQL image"
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

start_database() {
  local name="$1"
  docker run -d --rm --name "${name}" \
    -e "POSTGRES_DB=${database_name}" \
    -e "POSTGRES_USER=${database_user}" \
    -e "POSTGRES_PASSWORD=${database_password}" \
    -p 127.0.0.1::5432 "${postgres_image}" >/dev/null
  for _ in $(seq 1 60); do
    docker exec "${name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1 && return
    sleep 1
  done
  echo "Agent Orchestrator PostgreSQL 17 drill: ${name} did not become ready" >&2
  exit 1
}

database_url_for() {
  local name="$1" port
  port="$(docker port "${name}" 5432/tcp | awk -F: 'NR == 1 {print $NF}')"
  printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable' \
    "${database_user}" "${database_password}" "${port}" "${database_name}"
}

psql_command() {
  local name="$1" command="$2"
  docker exec -e "PGPASSWORD=${database_password}" "${name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
    --username="${database_user}" --dbname="${database_name}" --command "${command}"
}

log "starting disposable authority database"
start_database "${container_name}"
database_url="$(database_url_for "${container_name}")"
server_major="$(psql_command "${container_name}" 'SHOW server_version_num' | cut -c1-2)"
[[ "${server_major}" == "17" ]] || { echo "expected PostgreSQL 17" >&2; exit 1; }

log "building and applying 001 -> 094"
(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/mm-chat-migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/mm-chat-migrate" "$@"; }
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/fresh.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/fresh.log"
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

log "checking authority schema and least privilege"
psql_command "${container_name}" "
DO \$\$
BEGIN
  IF to_regclass('public.agent_runs') IS NULL
     OR to_regclass('public.agent_steps') IS NULL
     OR to_regclass('public.agent_attempts') IS NULL
     OR to_regclass('public.agent_run_events') IS NULL
     OR to_regclass('public.agent_kill_switches') IS NULL THEN
    RAISE EXCEPTION 'Agent Orchestrator tables are missing';
  END IF;
  IF NOT has_table_privilege('agent_orchestrator_runtime','agent_runs','SELECT')
     OR has_table_privilege('agent_orchestrator_runtime','agent_runs','INSERT')
     OR has_table_privilege('agent_orchestrator_runtime','agent_run_events','UPDATE')
     OR has_table_privilege('agent_orchestrator_runtime','agent_attempts','DELETE')
     OR has_table_privilege('go_api_runtime','agent_runs','SELECT')
     OR has_function_privilege('go_api_runtime',
       'agent_orchestrator_enqueue_run(text,text,uuid,text,text,text,jsonb,jsonb,text[],text[])','EXECUTE')
     OR has_function_privilege('agent_orchestrator_runtime',
       'agent_orchestrator_rebuild_projection(uuid,text)','EXECUTE')
     OR NOT has_function_privilege('agent_orchestrator_runtime',
       'agent_orchestrator_acquire_step(uuid,text,text,text,text,text,text[],integer,text,text,text)','EXECUTE') THEN
    RAISE EXCEPTION 'Agent Orchestrator grants violate least privilege';
  END IF;
END
\$\$;
" >/dev/null

log "running idempotency, race, lease, recovery, rebuild, Kill Switch and retention integration"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  go test -count=1 -race -run '^TestPostgresAuthority' ./internal/agentorchestrator)

log "creating retained authority for guarded down and dump/restore"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  MM_CHAT_AGENT_RETAIN_FIXTURE=1 \
  go test -count=1 -run '^TestPostgresAuthority' ./internal/agentorchestrator)

run_migrate down >"${work_dir}/guard-peel-089.log" 2>&1
grep -Fq "down 089_agent_draft_learning" "${work_dir}/guard-peel-089.log"
run_migrate down >"${work_dir}/guard-peel-088.log" 2>&1
grep -Fq "down 088_agent_cron_foundation" "${work_dir}/guard-peel-088.log"
run_migrate down >"${work_dir}/guard-peel-087.log" 2>&1
grep -Fq "down 087_agent_child_delegation" "${work_dir}/guard-peel-087.log"
run_migrate down >"${work_dir}/guard-peel-086.log" 2>&1
grep -Fq "down 086_agent_broker_foundation" "${work_dir}/guard-peel-086.log"
run_migrate down >"${work_dir}/guard-peel-085.log" 2>&1
grep -Fq "down 085_agent_runner_foundation" "${work_dir}/guard-peel-085.log"
set +e
run_migrate down >"${work_dir}/guarded-down.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] || ! grep -Fq "AGENT_ORCHESTRATOR_DOWN_DATA_EXISTS" "${work_dir}/guarded-down.log"; then
  cat "${work_dir}/guarded-down.log" >&2
  echo "Agent Orchestrator PostgreSQL 17 drill: non-empty down did not fail closed" >&2
  exit 1
fi
run_migrate up >"${work_dir}/guard-reapply-tail.log" 2>&1
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 089_agent_draft_learning" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/guard-reapply-tail.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/guard-reapply-tail.log"

log "dumping and restoring content-free control-plane authority"
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" \
  >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "
SELECT concat_ws(',',
  (SELECT count(*) FROM agent_runs),
  (SELECT count(*) FROM agent_steps),
  (SELECT count(*) FROM agent_attempts),
  (SELECT count(*) FROM agent_run_events),
  (SELECT count(*) FROM agent_kill_switches));")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" \
  psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" \
  <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "
SELECT concat_ws(',',
  (SELECT count(*) FROM agent_runs),
  (SELECT count(*) FROM agent_steps),
  (SELECT count(*) FROM agent_attempts),
  (SELECT count(*) FROM agent_run_events),
  (SELECT count(*) FROM agent_kill_switches));")"
[[ "${source_counts}" == "${restore_counts}" ]] || {
  echo "Agent Orchestrator PostgreSQL 17 drill: restore counts differ" >&2
  exit 1
}
psql_command "${restore_container_name}" "
DO \$\$
DECLARE item RECORD;
BEGIN
  FOR item IN SELECT user_id,id FROM agent_runs LOOP
    PERFORM agent_orchestrator_rebuild_projection(item.user_id,item.id);
  END LOOP;
END
\$\$;
" >/dev/null

log "rolling back empty 094 through 085, then proving clean 083 -> 084 -> 083 -> 094 replay"
run_migrate down >"${work_dir}/peel-094-tail-2.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/peel-094-tail-2.log"
run_migrate down >"${work_dir}/peel-093-tail-2.log" 2>&1
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/peel-093-tail-2.log"
run_migrate down >"${work_dir}/peel-092-tail-2.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/peel-092-tail-2.log"
run_migrate down >"${work_dir}/peel-091-tail-2.log" 2>&1
grep -Fq "down 091_agent_artifact_publication" "${work_dir}/peel-091-tail-2.log"
run_migrate down >"${work_dir}/down-090.log" 2>&1
grep -Fq "down 090_agent_product_shadow" "${work_dir}/down-090.log"
run_migrate down >"${work_dir}/down-089.log" 2>&1
grep -Fq "down 089_agent_draft_learning" "${work_dir}/down-089.log"
run_migrate down >"${work_dir}/down-088.log" 2>&1
grep -Fq "down 088_agent_cron_foundation" "${work_dir}/down-088.log"
run_migrate down >"${work_dir}/down-087.log" 2>&1
grep -Fq "down 087_agent_child_delegation" "${work_dir}/down-087.log"
run_migrate down >"${work_dir}/down-086.log" 2>&1
grep -Fq "down 086_agent_broker_foundation" "${work_dir}/down-086.log"
run_migrate down >"${work_dir}/down-085.log" 2>&1
grep -Fq "down 085_agent_runner_foundation" "${work_dir}/down-085.log"
psql_command "${container_name}" "
TRUNCATE TABLE agent_kill_switches;
DELETE FROM agent_runs;
DELETE FROM agent_run_snapshots;
UPDATE agent_kill_switch_state SET epoch=0 WHERE singleton;
" >/dev/null
run_migrate down >"${work_dir}/down.log" 2>&1
grep -Fq "down 084_agent_orchestrator_foundation" "${work_dir}/down.log"
run_migrate up >"${work_dir}/reup.log" 2>&1
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/reup.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/reup.log"
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/reup.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/reup.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/reup.log"
grep -Fq "up 089_agent_draft_learning" "${work_dir}/reup.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/reup.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/reup.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/reup.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/reup.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/reup.log"
run_migrate up >"${work_dir}/final-replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/final-replay.log"

log "passed (fresh/replay, privileges, idempotency/race/lease/restart/rebuild, Kill Switch, retention, guarded down, dump/restore, clean down/up; no Sandbox launch)"
