#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-approvals-pg17-$RANDOM-$$"
database_name="neo_chat_agent_approvals_drill"
database_password="agent-approvals-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"
cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
trap cleanup EXIT INT TERM
log() { printf 'Chat Agent approvals PostgreSQL 17 drill: %s\n' "$*"; }
for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Chat Agent approvals PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1
docker image inspect "${postgres_image}" >/dev/null 2>&1 ||
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null

docker run -d --rm --name "${container_name}" \
  -e "POSTGRES_DB=${database_name}" -e POSTGRES_USER=postgres \
  -e "POSTGRES_PASSWORD=${database_password}" -p 127.0.0.1::5432 \
  "${postgres_image}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${container_name}" pg_isready -U postgres -d "${database_name}" \
    >/dev/null 2>&1 && break
  sleep 1
done
docker exec "${container_name}" pg_isready -U postgres -d "${database_name}" >/dev/null
host_port="$(docker port "${container_name}" 5432/tcp | sed -n '1s/.*://p')"
database_url="postgres://postgres:${database_password}@127.0.0.1:${host_port}/${database_name}?sslmode=disable"
psql_command() {
  docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align --quiet \
      --username=postgres --dbname="${database_name}" --command "$1"
}
runtime_psql() { psql_command "SET ROLE go_api_runtime; $1"; }

log "applying and replaying schema head 101"
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" up >"${work_dir}/fresh.log" 2>&1
grep -Fq 'up 100_chat_agent_approvals' "${work_dir}/fresh.log"
grep -Fq 'up 101_chat_agent_transcript_blocks' "${work_dir}/fresh.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" up >"${work_dir}/replay.log" 2>&1
grep -Fq 'no migrations changed' "${work_dir}/replay.log"
[[ "$(psql_command 'SELECT max(version) FROM schema_migrations')" == '101' ]]

MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-101.log" 2>&1
grep -Fq 'down 101_chat_agent_transcript_blocks' "${work_dir}/peel-101.log"

log "checking exact gateways and denied direct table mutation"
[[ "$(psql_command "SELECT has_table_privilege('go_api_runtime','chat_agent_approvals','SELECT,INSERT,UPDATE,DELETE')")" == 'f' ]]
[[ "$(psql_command "SELECT has_table_privilege('go_api_runtime','chat_agent_conversation_tool_grants','SELECT,INSERT,UPDATE,DELETE')")" == 'f' ]]
for signature in \
  'chat_agent_create_approval(uuid,uuid,uuid,text,text,text,boolean,timestamptz,timestamptz)' \
  'chat_agent_decide_approval(uuid,uuid,bigint,text,timestamptz)' \
  'chat_agent_recover_approvals(timestamptz)'; do
  [[ "$(psql_command "SELECT has_function_privilege('go_api_runtime','${signature}','EXECUTE')")" == 't' ]]
done
set +e
runtime_psql "UPDATE chat_agent_approvals SET status='denied'" >"${work_dir}/direct-dml.log" 2>&1
dml_status=$?
set -e
[[ "${dml_status}" -ne 0 ]]
grep -Fq 'permission denied for table chat_agent_approvals' "${work_dir}/direct-dml.log"

user_id='a0000000-0000-4000-8000-000000000001'
conversation_id='a0000000-0000-4000-8000-000000000002'
message_id='a0000000-0000-4000-8000-000000000003'
turn_id='a0000000-0000-4000-8000-000000000004'
run_id='a0000000-0000-4000-8000-000000000005'
psql_command "
INSERT INTO users(id,email,display_name)
VALUES ('${user_id}','agent-approvals@example.test','Agent Approvals');
INSERT INTO conversations(id,user_id,title)
VALUES ('${conversation_id}','${user_id}','Agent Approvals');
INSERT INTO messages(id,conversation_id,user_id,sequence_no,role,status,content)
VALUES ('${message_id}','${conversation_id}','${user_id}',1,'assistant','streaming','');
SELECT event_id FROM chat_agent_start_turn(
  '${turn_id}', 'a0000000-0000-4000-8000-000000000006', '${user_id}',
  '${conversation_id}', '${message_id}', '${run_id}',
  TIMESTAMPTZ '2026-08-20 00:00:00+00'
);" >/dev/null

log "proving allow, first-decision-wins, conversation grant, expiry, and restart denial"
create_approval() {
  local id="$1" execution="$2" at="$3"
  runtime_psql "SELECT concat_ws('|', status, revision, COALESCE(decision,''))
  FROM chat_agent_create_approval(
    '${id}', '${turn_id}', '${user_id}', '${execution}', 'terminal', 'execute', TRUE,
    TIMESTAMPTZ '${at}' + interval '5 minutes', TIMESTAMPTZ '${at}'
  );"
}
first_id='a0000000-0000-4000-8000-000000000010'
[[ "$(create_approval "${first_id}" 'local-skill-1-1' '2026-08-20 00:00:01+00')" == 'pending|1|' ]]
allowed="$(runtime_psql "SELECT concat_ws('|', status, revision, decision)
FROM chat_agent_decide_approval(
  '${first_id}', '${user_id}', 1, 'allow_once', TIMESTAMPTZ '2026-08-20 00:00:02+00'
);")"
[[ "${allowed}" == 'allowed|2|allow_once' ]]
duplicate="$(runtime_psql "SELECT concat_ws('|', status, revision, decision)
FROM chat_agent_decide_approval(
  '${first_id}', '${user_id}', 1, 'deny', TIMESTAMPTZ '2026-08-20 00:00:03+00'
);")"
[[ "${duplicate}" == 'allowed|2|allow_once' ]]

grant_id='a0000000-0000-4000-8000-000000000011'
[[ "$(create_approval "${grant_id}" 'local-skill-1-2' '2026-08-20 00:00:04+00')" == 'pending|1|' ]]
[[ "$(runtime_psql "SELECT concat_ws('|', status, revision, decision)
FROM chat_agent_decide_approval(
  '${grant_id}', '${user_id}', 1, 'allow_conversation', TIMESTAMPTZ '2026-08-20 00:00:05+00'
);")" == 'allowed|2|allow_conversation' ]]
auto_id='a0000000-0000-4000-8000-000000000012'
[[ "$(create_approval "${auto_id}" 'local-skill-1-3' '2026-08-20 00:00:06+00')" == 'allowed|1|allow_conversation' ]]

# Remove only the policy so later requests exercise pending decisions again.
psql_command "DELETE FROM chat_agent_conversation_tool_grants
WHERE conversation_id='${conversation_id}'" >/dev/null
expired_id='a0000000-0000-4000-8000-000000000013'
[[ "$(create_approval "${expired_id}" 'local-skill-1-4' '2026-08-20 00:00:07+00')" == 'pending|1|' ]]
[[ "$(runtime_psql "SELECT concat_ws('|', status, revision, decision)
FROM chat_agent_decide_approval(
  '${expired_id}', '${user_id}', 1, 'allow_once', TIMESTAMPTZ '2026-08-20 00:05:08+00'
);")" == 'expired|2|expired' ]]
restart_id='a0000000-0000-4000-8000-000000000014'
[[ "$(create_approval "${restart_id}" 'local-skill-1-5' '2026-08-20 00:00:09+00')" == 'pending|1|' ]]
[[ "$(runtime_psql "SELECT chat_agent_recover_approvals(TIMESTAMPTZ '2026-08-20 00:00:10+00')")" == '1' ]]
[[ "$(psql_command "SELECT concat_ws('|', status, revision, decision)
FROM chat_agent_approvals WHERE id='${restart_id}'")" == 'denied|2|restart_denied' ]]

log "proving dirty down refusal and clean down/up replay"
set +e
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/dirty-down.log" 2>&1
down_status=$?
set -e
[[ "${down_status}" -ne 0 ]]
grep -Fq 'CHAT_AGENT_APPROVALS_DOWN_DATA_EXISTS' "${work_dir}/dirty-down.log"
psql_command "DELETE FROM users WHERE id='${user_id}'" >/dev/null
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/clean-down.log" 2>&1
grep -Fq 'down 100_chat_agent_approvals' "${work_dir}/clean-down.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" up >"${work_dir}/clean-up.log" 2>&1
grep -Fq 'up 100_chat_agent_approvals' "${work_dir}/clean-up.log"
grep -Fq 'up 101_chat_agent_transcript_blocks' "${work_dir}/clean-up.log"
[[ "$(psql_command 'SELECT max(version) FROM schema_migrations')" == '101' ]]
log "passed"
