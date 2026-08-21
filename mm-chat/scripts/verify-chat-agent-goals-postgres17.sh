#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-goals-pg17-$RANDOM-$$"
database_name="neo_chat_agent_goals_drill"
database_user="postgres"
database_password="agent-goals-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
trap cleanup EXIT INT TERM
log() { printf 'Chat Agent Goals PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Chat Agent Goals PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Chat Agent Goals PostgreSQL 17 drill: Docker is required" >&2
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
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align --quiet \
      --username="${database_user}" --dbname="${database_name}" --command "$1"
}
runtime_psql() {
  psql_command "SET ROLE go_api_runtime; $1"
}

log "applying and replaying schema head 101"
[[ "$(psql_command 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/fresh.log"
grep -Fq "up 097_chat_agent_goals" "${work_dir}/fresh.log"
grep -Fq "up 098_retire_legacy_agent_control_plane" "${work_dir}/fresh.log"
grep -Fq "up 099_chat_agent_event_log_function_repair" "${work_dir}/fresh.log"
grep -Fq "up 100_chat_agent_approvals" "${work_dir}/fresh.log"
grep -Fq "up 101_chat_agent_transcript_blocks" "${work_dir}/fresh.log"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"
[[ "$(psql_command 'SELECT max(version) FROM schema_migrations')" == "101" ]]

log "checking schema, exact Function grants, and denied direct mutation"
[[ "$(psql_command "SELECT to_regclass('public.chat_agent_goals') IS NOT NULL")" == "t" ]]
[[ "$(psql_command "SELECT has_table_privilege('go_api_runtime','chat_agent_goals','SELECT')")" == "t" ]]
[[ "$(psql_command "SELECT has_table_privilege('go_api_runtime','chat_agent_goals','INSERT,UPDATE,DELETE')")" == "f" ]]
for signature in \
  'chat_agent_create_goal(uuid,uuid,uuid,text,integer,timestamptz)' \
  'chat_agent_change_goal(uuid,uuid,uuid,bigint,text,text,integer,text,timestamptz)' \
  'chat_agent_cancel_goal(uuid,uuid,uuid,bigint,timestamptz)' \
  'chat_agent_start_goal_round(uuid,uuid,uuid,bigint,integer,timestamptz)'; do
  [[ "$(psql_command "SELECT has_function_privilege('go_api_runtime','${signature}','EXECUTE')")" == "t" ]]
done
set +e
runtime_psql "UPDATE chat_agent_goals SET objective='forbidden'" \
  >"${work_dir}/direct-update.log" 2>&1
direct_update_status=$?
set -e
[[ "${direct_update_status}" -ne 0 ]]
grep -Fq 'permission denied for table chat_agent_goals' "${work_dir}/direct-update.log"

log "running repository CAS, round, event, and cancellation proof"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  go test ./internal/chat -run '^TestPostgresChatAgentGoalCASRoundsEventsAndCancellation$' -count=1)

user_id='97000000-0000-4000-8000-000000000001'
conversation_id='97000000-0000-4000-8000-000000000002'
message_id='97000000-0000-4000-8000-000000000003'
turn_id='97000000-0000-4000-8000-000000000004'
run_id='97000000-0000-4000-8000-000000000005'
goal_id='97000000-0000-4000-8000-000000000006'

psql_command "
INSERT INTO users(id,email,display_name)
VALUES ('${user_id}','agent-goals@example.test','Agent Goals');
INSERT INTO conversations(id,user_id,title)
VALUES ('${conversation_id}','${user_id}','Agent Goals');
INSERT INTO messages(id,conversation_id,user_id,sequence_no,role,status,content)
VALUES ('${message_id}','${conversation_id}','${user_id}',1,'assistant','streaming','');
SELECT event_id FROM chat_agent_start_turn(
  '${turn_id}',
  '97000000-0000-4000-8000-000000000007',
  '${user_id}',
  '${conversation_id}',
  '${message_id}',
  '${run_id}',
  TIMESTAMPTZ '2026-08-16 00:00:00+00'
);" >/dev/null

created="$(runtime_psql "
SELECT concat_ws('|', id, phase, revision, rounds_started, max_goal_rounds)
FROM chat_agent_create_goal(
  '${turn_id}',
  '97000000-0000-4000-8000-000000000008',
  '${goal_id}',
  'produce and verify the fixture',
  3,
  TIMESTAMPTZ '2026-08-16 00:00:01+00'
);")"
[[ "${created}" == "${goal_id}|active|1|0|3" ]]

set +e
runtime_psql "SELECT id FROM chat_agent_change_goal(
  '${turn_id}',
  '97000000-0000-4000-8000-000000000009',
  '${goal_id}',
  9,
  'pause',
  NULL,
  NULL,
  NULL,
  TIMESTAMPTZ '2026-08-16 00:00:02+00'
);" >"${work_dir}/stale-cas.log" 2>&1
stale_status=$?
set -e
[[ "${stale_status}" -ne 0 ]]
grep -Fq 'CHAT_AGENT_GOAL_STALE_REVISION' "${work_dir}/stale-cas.log"

round="$(runtime_psql "
SELECT concat_ws('|', revision, rounds_started)
FROM chat_agent_start_goal_round(
  '${turn_id}',
  '97000000-0000-4000-8000-00000000000a',
  '${goal_id}',
  1,
  1,
  TIMESTAMPTZ '2026-08-16 00:00:03+00'
);")"
[[ "${round}" == "1|1" ]]

paused="$(runtime_psql "
SELECT concat_ws('|', phase, revision, rounds_started)
FROM chat_agent_change_goal(
  '${turn_id}',
  '97000000-0000-4000-8000-00000000000b',
  '${goal_id}',
  1,
  'pause',
  NULL,
  NULL,
  NULL,
  TIMESTAMPTZ '2026-08-16 00:00:04+00'
);")"
[[ "${paused}" == "paused|2|1" ]]

cleared="$(runtime_psql "
SELECT concat_ws('|', goal_id, revision)
FROM chat_agent_cancel_goal(
  '${turn_id}',
  '97000000-0000-4000-8000-00000000000c',
  '${goal_id}',
  2,
  TIMESTAMPTZ '2026-08-16 00:00:05+00'
);")"
[[ "${cleared}" == "${goal_id}|3" ]]
[[ "$(psql_command "SELECT count(*) FROM chat_agent_goals WHERE conversation_id='${conversation_id}'")" == "0" ]]
event_projection="$(psql_command "
SELECT string_agg(sequence::text || ':' || event_type, ',' ORDER BY sequence)
FROM chat_agent_events
WHERE turn_id='${turn_id}';")"
[[ "${event_projection}" == '1:turn.started,2:goal.changed,3:goal.round.started,4:goal.changed,5:goal.changed' ]]
[[ "$(psql_command "
SELECT payload->>'operation'
FROM chat_agent_events
WHERE turn_id='${turn_id}' AND event_type='goal.changed'
ORDER BY sequence DESC LIMIT 1;")" == "cancel" ]]

log "proving dirty 097 down refusal and clean down/up replay"
runtime_psql "SELECT id FROM chat_agent_create_goal(
  '${turn_id}',
  '97000000-0000-4000-8000-00000000000d',
  '97000000-0000-4000-8000-00000000000e',
  'retain the rollback guard',
  3,
  TIMESTAMPTZ '2026-08-16 00:00:06+00'
);" >/dev/null
run_migrate down >"${work_dir}/peel-101-transcript.log" 2>&1
grep -Fq 'down 101_chat_agent_transcript_blocks' "${work_dir}/peel-101-transcript.log"
run_migrate down >"${work_dir}/peel-100-approvals.log" 2>&1
grep -Fq 'down 100_chat_agent_approvals' "${work_dir}/peel-100-approvals.log"
run_migrate down >"${work_dir}/peel-099-function-repair.log" 2>&1
grep -Fq 'down 099_chat_agent_event_log_function_repair' "${work_dir}/peel-099-function-repair.log"
run_migrate down >"${work_dir}/peel-098-retirement.log" 2>&1
grep -Fq 'down 098_retire_legacy_agent_control_plane' "${work_dir}/peel-098-retirement.log"
set +e
run_migrate down >"${work_dir}/dirty-down.log" 2>&1
dirty_status=$?
set -e
[[ "${dirty_status}" -ne 0 ]]
grep -Fq 'CHAT_AGENT_GOALS_DOWN_DATA_EXISTS' "${work_dir}/dirty-down.log"
runtime_psql "SELECT goal_id FROM chat_agent_cancel_goal(
  '${turn_id}',
  '97000000-0000-4000-8000-00000000000f',
  '97000000-0000-4000-8000-00000000000e',
  1,
  TIMESTAMPTZ '2026-08-16 00:00:07+00'
);" >/dev/null
run_migrate down >"${work_dir}/clean-down.log" 2>&1
grep -Fq 'down 097_chat_agent_goals' "${work_dir}/clean-down.log"
run_migrate up >"${work_dir}/clean-reup.log" 2>&1
grep -Fq 'up 097_chat_agent_goals' "${work_dir}/clean-reup.log"
grep -Fq 'up 098_retire_legacy_agent_control_plane' "${work_dir}/clean-reup.log"
grep -Fq 'up 099_chat_agent_event_log_function_repair' "${work_dir}/clean-reup.log"
grep -Fq 'up 100_chat_agent_approvals' "${work_dir}/clean-reup.log"
grep -Fq 'up 101_chat_agent_transcript_blocks' "${work_dir}/clean-reup.log"
[[ "$(psql_command 'SELECT max(version) FROM schema_migrations')" == "101" ]]
psql_command "DELETE FROM users WHERE id='${user_id}'" >/dev/null

log "passed"
