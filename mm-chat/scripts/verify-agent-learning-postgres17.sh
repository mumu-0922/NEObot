#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-learning-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
database_name="neo_chat_agent_learning_drill"
database_user="postgres"
database_password="agent-learning-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker stop "${container_name}" "${restore_container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent Learning PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Docker is required" >&2
  exit 1
}
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
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
    if docker exec "${name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  exit 1
}
database_url_for() {
  local name="$1" port
  port="$(docker port "${name}" 5432/tcp | sed -n '1s/.*://p')"
  printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable' \
    "${database_user}" "${database_password}" "${port}" "${database_name}"
}
psql_command() {
  local name="$1" command="$2"
  docker exec -e "PGPASSWORD=${database_password}" "${name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
      --username="${database_user}" --dbname="${database_name}" --command "${command}"
}

log "starting disposable database"
start_database "${container_name}"
database_url="$(database_url_for "${container_name}")"
[[ "$(psql_command "${container_name}" 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }

log "applying 001 -> 096, replaying, and peeling the empty product tail"
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 089_agent_draft_learning" "${work_dir}/fresh.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/fresh.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/fresh.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/fresh.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/fresh.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/fresh.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/fresh.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/fresh.log"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"
run_migrate down >"${work_dir}/peel-096-chat-agent-event-tail.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/peel-096-chat-agent-event-tail.log"
run_migrate down >"${work_dir}/peel-095-tail-1.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/peel-095-tail-1.log"
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

log "checking Agent Learning least privilege"
psql_command "${container_name}" "
DO \$\$
BEGIN
  IF NOT has_table_privilege('agent_learning_control','agent_learning_drafts','SELECT')
     OR has_table_privilege('agent_learning_control','agent_learning_drafts','INSERT')
     OR has_table_privilege('agent_learning_control','agent_learning_drafts','UPDATE')
     OR has_table_privilege('agent_learning_control','agent_learning_drafts','DELETE')
     OR has_table_privilege('go_api_runtime','agent_learning_drafts','SELECT')
     OR has_table_privilege('agent_orchestrator_runtime','agent_learning_drafts','SELECT')
     OR has_table_privilege('agent_runner_control','agent_learning_drafts','SELECT')
     OR has_table_privilege('agent_effect_control','agent_learning_drafts','SELECT')
     OR has_table_privilege('agent_delegation_control','agent_learning_drafts','SELECT')
     OR has_table_privilege('agent_cron_control','agent_learning_drafts','SELECT')
     OR NOT has_function_privilege('agent_learning_control','agent_learning_create_draft(text,uuid,text,jsonb,text,timestamp with time zone,text)','EXECUTE')
     OR NOT has_function_privilege('agent_learning_control','agent_learning_promote(text,uuid,bigint,text,text,uuid,text,text,text,text,jsonb,text)','EXECUTE')
     OR has_function_privilege('go_api_runtime','agent_learning_promote(text,uuid,bigint,text,text,uuid,text,text,text,text,jsonb,text)','EXECUTE')
     OR has_function_privilege('agent_runner_control','agent_learning_complete_checks(text,text,bigint,jsonb,text)','EXECUTE')
     OR pg_has_role('agent_learning_control','agent_learning_owner','MEMBER')
     OR (SELECT rolcanlogin OR rolsuper OR rolcreaterole OR rolbypassrls FROM pg_roles WHERE rolname='agent_learning_owner')
     OR (SELECT rolcanlogin OR rolsuper OR rolcreaterole OR rolbypassrls FROM pg_roles WHERE rolname='agent_learning_control') THEN
    RAISE EXCEPTION 'Agent Learning grants violate least privilege';
  END IF;
END
\$\$;" >/dev/null

log "running provenance, checks, promotion replay, stale claim, and cleanup matrix"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  go test -count=1 -race -run '^TestPostgresAgentLearning' ./internal/agentlearning)

log "creating retained fixture and proving guarded down"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  MM_CHAT_AGENT_LEARNING_RETAIN_FIXTURE=1 \
  go test -count=1 -run '^TestPostgresAgentLearning' ./internal/agentlearning)
set +e
run_migrate down >"${work_dir}/guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] || ! grep -Fq 'AGENT_LEARNING_DOWN_DATA_EXISTS' "${work_dir}/guard.log"; then
  cat "${work_dir}/guard.log" >&2
  exit 1
fi

log "dumping and restoring Draft authority"
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "
SELECT concat_ws(',',
  (SELECT count(*) FROM agent_learning_drafts),
  (SELECT count(*) FROM agent_learning_check_results),
  (SELECT count(*) FROM agent_learning_decisions),
  (SELECT count(*) FROM agent_learning_cleanup_queue),
  (SELECT count(*) FROM agent_learning_audit_events),
  (SELECT count(*) FROM skill_package_candidates WHERE source_type='learning')); ")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" \
  psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" \
  <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "
SELECT concat_ws(',',
  (SELECT count(*) FROM agent_learning_drafts),
  (SELECT count(*) FROM agent_learning_check_results),
  (SELECT count(*) FROM agent_learning_decisions),
  (SELECT count(*) FROM agent_learning_cleanup_queue),
  (SELECT count(*) FROM agent_learning_audit_events),
  (SELECT count(*) FROM skill_package_candidates WHERE source_type='learning')); ")"
if [[ "${source_counts}" != "${restore_counts}" ]]; then
  printf 'Agent Learning dump/restore mismatch: source=%s restore=%s\n' \
    "${source_counts}" "${restore_counts}" >&2
  exit 1
fi

log "proving clean 088 -> 089 -> 088 -> 095"
psql_command "${container_name}" "
TRUNCATE TABLE agent_learning_audit_events,agent_learning_cleanup_queue,agent_learning_decisions,
  agent_learning_check_results,agent_learning_drafts;
DELETE FROM skill_package_candidates WHERE source_type='learning';" >/dev/null
run_migrate down >"${work_dir}/down.log" 2>&1
grep -Fq "down 089_agent_draft_learning" "${work_dir}/down.log"
run_migrate up >"${work_dir}/reup.log" 2>&1
grep -Fq "up 089_agent_draft_learning" "${work_dir}/reup.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/reup.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/reup.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/reup.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/reup.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/reup.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/reup.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/reup.log"
run_migrate up >"${work_dir}/final.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/final.log"
log "passed (fresh/replay, least privilege, provenance/checks/promotion/cleanup, guarded down, dump/restore, clean down/up)"
