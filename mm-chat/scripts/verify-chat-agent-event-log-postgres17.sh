#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-events-pg17-$RANDOM-$$"
database_name="neo_chat_agent_events_drill"
database_user="postgres"
database_password="agent-events-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
trap cleanup EXIT INT TERM
log() { printf 'Chat Agent event log PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Chat Agent event log PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Chat Agent event log PostgreSQL 17 drill: Docker is required" >&2
  exit 1
}
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

docker run -d --rm --name "${container_name}" \
  -e "POSTGRES_DB=${database_name}" -e "POSTGRES_USER=${database_user}" \
  -e "POSTGRES_PASSWORD=${database_password}" -p 127.0.0.1::5432 \
  "${postgres_image}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" \
    >/dev/null 2>&1 && break
  sleep 1
done
docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null

host_port="$(docker port "${container_name}" 5432/tcp | sed -n '1s/.*://p')"
database_url="postgres://${database_user}:${database_password}@127.0.0.1:${host_port}/${database_name}?sslmode=disable"
psql_command() {
  docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
      --username="${database_user}" --dbname="${database_name}" --command "$1"
}

log "applying and replaying schema head 100"
[[ "$(psql_command 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }
peel_to_event_log() {
  local prefix="$1"
  run_migrate down >"${work_dir}/${prefix}-100.log" 2>&1
  grep -Fq "down 100_chat_agent_approvals" "${work_dir}/${prefix}-100.log"
  run_migrate down >"${work_dir}/${prefix}-099.log" 2>&1
  grep -Fq "down 099_chat_agent_event_log_function_repair" "${work_dir}/${prefix}-099.log"
  run_migrate down >"${work_dir}/${prefix}-098.log" 2>&1
  grep -Fq "down 098_retire_legacy_agent_control_plane" "${work_dir}/${prefix}-098.log"
  run_migrate down >"${work_dir}/${prefix}-097.log" 2>&1
  grep -Fq "down 097_chat_agent_goals" "${work_dir}/${prefix}-097.log"
}
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/fresh.log"
grep -Fq "up 097_chat_agent_goals" "${work_dir}/fresh.log"
grep -Fq "up 098_retire_legacy_agent_control_plane" "${work_dir}/fresh.log"
grep -Fq "up 099_chat_agent_event_log_function_repair" "${work_dir}/fresh.log"
grep -Fq "up 100_chat_agent_approvals" "${work_dir}/fresh.log"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"
[[ "$(psql_command 'SELECT max(version) FROM schema_migrations')" == "100" ]]
[[ "$(psql_command "SELECT checksum FROM schema_migrations WHERE version=96")" == \
  "f7c6227d3dd559cb53b22a28af1d77bc570d45a42288bf1f348b22136ef1b042" ]]

log "checking repaired gateways, append-only schema, and least privilege"
[[ "$(psql_command "SELECT to_regclass('public.chat_agent_turns') IS NOT NULL")" == "t" ]]
[[ "$(psql_command "SELECT to_regclass('public.chat_agent_events') IS NOT NULL")" == "t" ]]
[[ "$(psql_command "SELECT has_table_privilege('go_api_runtime','chat_agent_events','SELECT')")" == "t" ]]
[[ "$(psql_command "SELECT has_table_privilege('go_api_runtime','chat_agent_events','INSERT,UPDATE,DELETE')")" == "f" ]]
[[ "$(psql_command "SELECT has_function_privilege('go_api_runtime','chat_agent_append_event(uuid,uuid,text,integer,jsonb,timestamptz)','EXECUTE')")" == "t" ]]
[[ "$(psql_command "SELECT pg_get_functiondef('chat_agent_start_turn(uuid,uuid,uuid,uuid,uuid,uuid,timestamptz)'::regprocedure) LIKE '%ON CONFLICT ON CONSTRAINT chat_agent_events_pkey%'")" == "t" ]]
[[ "$(psql_command "SELECT pg_get_functiondef('chat_agent_append_event(uuid,uuid,text,integer,jsonb,timestamptz)'::regprocedure) LIKE '%UPDATE messages AS message%'")" == "t" ]]
[[ "$(psql_command "SELECT bool_and(
  routine.prosecdef
  AND pg_get_userbyid(routine.proowner) <> 'go_api_runtime'
  AND 'search_path=public, pg_catalog, pg_temp' = ANY(routine.proconfig)
) FROM pg_proc AS routine
WHERE routine.oid IN (
  'chat_agent_start_turn(uuid,uuid,uuid,uuid,uuid,uuid,timestamptz)'::regprocedure,
  'chat_agent_append_event(uuid,uuid,text,integer,jsonb,timestamptz)'::regprocedure
)")" == "t" ]]

peel_to_event_log "initial-peel"

log "running repository sequence, replay, interruption, and torn-finalization proofs"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  go test ./internal/chat -run '^TestPostgresChatAgent(EventLog|Recovery)' -count=1)

log "proving dirty 096 down refusal and clean 096 -> 100 replay"
# Each integration test calls Runner.Up independently, so the first test
# reapplies the 097-099 tail that this drill peeled before invoking `go test`.
# Peel it again before exercising the 096 dirty-data guard. Both forward-only
# down paths retain the repaired gateway bodies.
peel_to_event_log "post-test-peel"
psql_command "
INSERT INTO users(id,email,display_name)
VALUES ('96000000-0000-4000-8000-000000000001','agent-events@example.test','Agent Events');
INSERT INTO conversations(id,user_id,title)
VALUES ('96000000-0000-4000-8000-000000000002','96000000-0000-4000-8000-000000000001','Agent Events');
INSERT INTO messages(id,conversation_id,user_id,sequence_no,role,status,content)
VALUES (
  '96000000-0000-4000-8000-000000000003',
  '96000000-0000-4000-8000-000000000002',
  '96000000-0000-4000-8000-000000000001',
  1,'assistant','streaming',''
);
SELECT event_id FROM chat_agent_start_turn(
  '96000000-0000-4000-8000-000000000004',
  '96000000-0000-4000-8000-000000000005',
  '96000000-0000-4000-8000-000000000001',
  '96000000-0000-4000-8000-000000000002',
  '96000000-0000-4000-8000-000000000003',
  '96000000-0000-4000-8000-000000000006',
  TIMESTAMPTZ '2026-08-17 00:00:00+00'
);" >/dev/null
set +e
run_migrate down >"${work_dir}/dirty-down.log" 2>&1
dirty_status=$?
set -e
[[ "${dirty_status}" -ne 0 ]]
grep -Fq "CHAT_AGENT_EVENT_LOG_DOWN_DATA_EXISTS" "${work_dir}/dirty-down.log"
psql_command "TRUNCATE chat_agent_events, chat_agent_turns" >/dev/null
run_migrate down >"${work_dir}/clean-down.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/clean-down.log"
run_migrate up >"${work_dir}/clean-reup.log" 2>&1
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/clean-reup.log"
grep -Fq "up 097_chat_agent_goals" "${work_dir}/clean-reup.log"
grep -Fq "up 098_retire_legacy_agent_control_plane" "${work_dir}/clean-reup.log"
grep -Fq "up 099_chat_agent_event_log_function_repair" "${work_dir}/clean-reup.log"
grep -Fq "up 100_chat_agent_approvals" "${work_dir}/clean-reup.log"
[[ "$(psql_command 'SELECT max(version) FROM schema_migrations')" == "100" ]]

log "passed"
