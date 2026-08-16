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

log "applying and replaying schema head 096"
[[ "$(psql_command 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/fresh.log"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"
[[ "$(psql_command 'SELECT max(version) FROM schema_migrations')" == "96" ]]

log "checking append-only schema and least privilege"
[[ "$(psql_command "SELECT to_regclass('public.chat_agent_turns') IS NOT NULL")" == "t" ]]
[[ "$(psql_command "SELECT to_regclass('public.chat_agent_events') IS NOT NULL")" == "t" ]]
[[ "$(psql_command "SELECT has_table_privilege('go_api_runtime','chat_agent_events','SELECT')")" == "t" ]]
[[ "$(psql_command "SELECT has_table_privilege('go_api_runtime','chat_agent_events','INSERT,UPDATE,DELETE')")" == "f" ]]
[[ "$(psql_command "SELECT has_function_privilege('go_api_runtime','chat_agent_append_event(uuid,uuid,text,integer,jsonb,timestamptz)','EXECUTE')")" == "t" ]]

log "running repository sequence, replay, interruption, and torn-finalization proofs"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  go test ./internal/chat -run '^TestPostgresChatAgent(EventLog|Recovery)' -count=1)

log "proving dirty down refusal and clean 096 down/up"
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
[[ "$(psql_command 'SELECT max(version) FROM schema_migrations')" == "96" ]]

log "passed"
