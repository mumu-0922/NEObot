#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
source "${script_dir}/migration-drill-tail.sh"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-legacy-agent-cleanup-pg17-$RANDOM-$$"
fresh_database="neo_chat_agent_cleanup_fresh"
upgrade_database="neo_chat_agent_cleanup_upgrade"
database_user="postgres"
database_password="agent-cleanup-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
finish() {
  local status=$?
  if ((status != 0)); then
    for log_file in "${work_dir}"/*.log; do
      [[ -f "${log_file}" ]] || continue
      printf '\n--- %s ---\n' "$(basename "${log_file}")" >&2
      tail -n 80 "${log_file}" >&2
    done
  fi
  cleanup
  exit "${status}"
}
trap finish EXIT INT TERM
log() { printf 'Legacy Agent cleanup PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl python3; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Legacy Agent cleanup PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Legacy Agent cleanup PostgreSQL 17 drill: Docker is required" >&2
  exit 1
}
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

docker run -d --rm --name "${container_name}" \
  -e "POSTGRES_DB=${fresh_database}" -e "POSTGRES_USER=${database_user}" \
  -e "POSTGRES_PASSWORD=${database_password}" -p 127.0.0.1::5432 \
  "${postgres_image}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${container_name}" pg_isready -U "${database_user}" -d "${fresh_database}" \
    >/dev/null 2>&1 && break
  sleep 1
done
docker exec "${container_name}" pg_isready -U "${database_user}" -d "${fresh_database}" >/dev/null

host_port="$(docker port "${container_name}" 5432/tcp | sed -n '1s/.*://p')"
database_url() {
  printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable' \
    "${database_user}" "${database_password}" "${host_port}" "$1"
}
psql_command() {
  local database="$1" command="$2"
  docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align --quiet \
      --username="${database_user}" --dbname="${database}" --command "${command}"
}
psql_file() {
  local database="$1" path="$2"
  docker exec -i -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --quiet \
      --username="${database_user}" --dbname="${database}" <"${path}"
}

log "building the migration binary"
(cd "${backend_dir}" && GOCACHE="${GOCACHE:-/tmp/neo-chat-go-cache}" \
  go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() {
  local database="$1"
  shift
  MIGRATION_DATABASE_URL="$(database_url "${database}")" "${work_dir}/migrate" "$@"
}

assert_retired_objects_absent() {
  local database="$1"
  [[ "$(psql_command "${database}" "
    SELECT count(*) FROM pg_class relation
    JOIN pg_namespace namespace ON namespace.oid=relation.relnamespace
    WHERE namespace.nspname='public'
      AND relation.relname LIKE 'agent\\_%' ESCAPE '\\'
      AND relation.relname NOT LIKE 'chat\\_agent\\_%' ESCAPE '\\'
      AND relation.relkind IN ('r','p','v','m');")" == "0" ]]
  [[ "$(psql_command "${database}" "
    SELECT count(*) FROM pg_proc routine
    JOIN pg_namespace namespace ON namespace.oid=routine.pronamespace
    WHERE namespace.nspname='public'
      AND routine.proname LIKE 'agent\\_%' ESCAPE '\\';")" == "0" ]]
  [[ "$(psql_command "${database}" "
    SELECT count(*) FROM pg_roles WHERE rolname LIKE 'agent\\_%' ESCAPE '\\';")" == "0" ]]
}

assert_retained_schema() {
  local database="$1"
  [[ "$(psql_command "${database}" "SELECT concat_ws('|',
    to_regclass('public.chat_agent_turns') IS NOT NULL,
    to_regclass('public.chat_agent_events') IS NOT NULL,
    to_regclass('public.chat_agent_goals') IS NOT NULL,
    to_regclass('public.skill_package_versions') IS NOT NULL,
    to_regclass('public.skill_package_candidates') IS NOT NULL,
    to_regclass('public.skill_installations') IS NOT NULL);")" == "t|t|t|t|t|t" ]]
}

log "proving a fresh replay to head 099"
[[ "$(psql_command "${fresh_database}" 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
run_migrate "${fresh_database}" up >"${work_dir}/fresh.log" 2>&1
grep -Fq 'up 098_retire_legacy_agent_control_plane' "${work_dir}/fresh.log"
grep -Fq 'up 099_chat_agent_event_log_function_repair' "${work_dir}/fresh.log"
[[ "$(psql_command "${fresh_database}" 'SELECT max(version) FROM schema_migrations')" == "99" ]]
[[ "$(psql_command "${fresh_database}" 'SELECT checksum FROM schema_migrations WHERE version=96')" == \
  "f7c6227d3dd559cb53b22a28af1d77bc570d45a42288bf1f348b22136ef1b042" ]]
assert_retired_objects_absent "${fresh_database}"
assert_retained_schema "${fresh_database}"
run_migrate "${fresh_database}" up >"${work_dir}/fresh-replay.log" 2>&1
grep -Fq 'no migrations changed' "${work_dir}/fresh-replay.log"

log "proving the forward-only 099 and irreversible 098 down/re-up remain safe no-ops"
run_migrate "${fresh_database}" down >"${work_dir}/fresh-down.log" 2>&1
grep -Fq 'down 099_chat_agent_event_log_function_repair' "${work_dir}/fresh-down.log"
run_migrate "${fresh_database}" down >"${work_dir}/fresh-down-098.log" 2>&1
grep -Fq 'down 098_retire_legacy_agent_control_plane' "${work_dir}/fresh-down-098.log"
assert_retired_objects_absent "${fresh_database}"
run_migrate "${fresh_database}" up >"${work_dir}/fresh-reup.log" 2>&1
grep -Fq 'up 098_retire_legacy_agent_control_plane' "${work_dir}/fresh-reup.log"
grep -Fq 'up 099_chat_agent_event_log_function_repair' "${work_dir}/fresh-reup.log"
assert_retired_objects_absent "${fresh_database}"

log "preparing a schema-097 upgrade with the live 096 checksum and repaired functions"
psql_command postgres "CREATE DATABASE ${upgrade_database}" >/dev/null
psql_command "${upgrade_database}" "$(migration_drill_deferred_tail_sql "${backend_dir}" \
  098_retire_legacy_agent_control_plane \
  099_chat_agent_event_log_function_repair)" >/dev/null
run_migrate "${upgrade_database}" up >"${work_dir}/through-097.log" 2>&1
[[ "$(psql_command "${upgrade_database}" "SELECT count(*) FROM schema_migrations")" == "99" ]]
[[ "$(psql_command "${upgrade_database}" "SELECT to_regclass('public.agent_runs') IS NOT NULL")" == "t" ]]
[[ "$(psql_command "${upgrade_database}" 'SELECT checksum FROM schema_migrations WHERE version=96')" == \
  "f7c6227d3dd559cb53b22a28af1d77bc570d45a42288bf1f348b22136ef1b042" ]]
psql_command "${upgrade_database}" 'DELETE FROM schema_migrations WHERE version IN (98,99)' >/dev/null
# Production already has these corrected bodies while retaining the original
# 096 ledger checksum. Rehearse that exact state, then prove 099 is idempotent.
psql_file "${upgrade_database}" \
  "${backend_dir}/migrations/099_chat_agent_event_log_function_repair.up.sql" >/dev/null

psql_command "${upgrade_database}" "
INSERT INTO users(id,email,display_name) VALUES
  ('98000000-0000-4000-8000-000000000001','cleanup-one@example.test','Cleanup One'),
  ('98000000-0000-4000-8000-000000000002','cleanup-two@example.test','Cleanup Two');
INSERT INTO conversations(id,user_id,title) VALUES
  ('98000000-0000-4000-8000-000000000011','98000000-0000-4000-8000-000000000001','One'),
  ('98000000-0000-4000-8000-000000000012','98000000-0000-4000-8000-000000000002','Two');
INSERT INTO messages(id,conversation_id,user_id,sequence_no,role,status,content) VALUES
  ('98000000-0000-4000-8000-000000000021','98000000-0000-4000-8000-000000000011','98000000-0000-4000-8000-000000000001',1,'assistant','streaming',''),
  ('98000000-0000-4000-8000-000000000022','98000000-0000-4000-8000-000000000012','98000000-0000-4000-8000-000000000002',1,'assistant','completed','retained');
SELECT event_id FROM chat_agent_start_turn(
  '98000000-0000-4000-8000-000000000031',
  '98000000-0000-4000-8000-000000000032',
  '98000000-0000-4000-8000-000000000001',
  '98000000-0000-4000-8000-000000000011',
  '98000000-0000-4000-8000-000000000021',
  '98000000-0000-4000-8000-000000000033',
  clock_timestamp());
INSERT INTO skill_package_versions(
  package_fingerprint,sbom_fingerprint,name,version,description,has_runtime,
  file_count,package_bytes,expanded_bytes,package_object_key,sbom_object_key
) VALUES (
  'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
  'cleanup-proof','1.0.0','retained Skill package',false,1,1,1,
  'skill-packages/sha256/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.zip',
  'skill-sboms/sha256/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.cdx.json'
);
INSERT INTO agent_kill_switches(
  switch_id,revision,epoch,scope_type,scope_value,scope_key,mode,active,
  actor_type,actor_id,reason_code
) VALUES (
  'switch_cleanupblock0001',1,1,'global','*','global:*','deny_new',true,
  'operator','cleanup-drill','CLEANUP_DRILL'
);" >/dev/null

business_counts_before="$(psql_command "${upgrade_database}" "SELECT concat_ws('|',
  (SELECT count(*) FROM users WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM conversations WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM messages WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM chat_agent_turns WHERE id='98000000-0000-4000-8000-000000000031'),
  (SELECT count(*) FROM chat_agent_events WHERE turn_id='98000000-0000-4000-8000-000000000031'),
  (SELECT count(*) FROM skill_package_versions WHERE name='cleanup-proof'));")"
[[ "${business_counts_before}" == '2|2|2|1|1|1' ]]

log "proving a nonempty legacy fact table aborts the whole migration"
set +e
run_migrate "${upgrade_database}" up >"${work_dir}/blocked.log" 2>&1
blocked_status=$?
set -e
[[ "${blocked_status}" -ne 0 ]]
grep -Fq 'LEGACY_AGENT_CONTROL_PLANE_DATA_EXISTS' "${work_dir}/blocked.log"
grep -Fq 'LEGACY_AGENT_CONTROL_PLANE_DATA_EXISTS: agent_kill_switches' "${work_dir}/blocked.log"
[[ "$(psql_command "${upgrade_database}" 'SELECT max(version) FROM schema_migrations')" == "97" ]]
[[ "$(psql_command "${upgrade_database}" "SELECT count(*) FROM agent_kill_switches")" == "1" ]]
[[ "$(psql_command "${upgrade_database}" "SELECT to_regprocedure('public.agent_orchestrator_enqueue_run(text,text,uuid,text,text,text,jsonb,jsonb,text[],text[])') IS NOT NULL")" == "t" ]]
[[ "$(psql_command "${upgrade_database}" "SELECT count(*) FROM pg_roles WHERE rolname='agent_orchestrator_runtime'")" == "1" ]]
[[ "$(psql_command "${upgrade_database}" "SELECT concat_ws('|',
  (SELECT count(*) FROM users WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM conversations WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM messages WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM chat_agent_turns WHERE id='98000000-0000-4000-8000-000000000031'),
  (SELECT count(*) FROM chat_agent_events WHERE turn_id='98000000-0000-4000-8000-000000000031'),
  (SELECT count(*) FROM skill_package_versions WHERE name='cleanup-proof'));")" == "${business_counts_before}" ]]

log "removing only the synthetic blocker and completing the upgrade"
psql_command "${upgrade_database}" 'DELETE FROM agent_kill_switches' >/dev/null
run_migrate "${upgrade_database}" up >"${work_dir}/upgrade.log" 2>&1
grep -Fq 'up 098_retire_legacy_agent_control_plane' "${work_dir}/upgrade.log"
grep -Fq 'up 099_chat_agent_event_log_function_repair' "${work_dir}/upgrade.log"
[[ "$(psql_command "${upgrade_database}" 'SELECT max(version) FROM schema_migrations')" == "99" ]]
[[ "$(psql_command "${upgrade_database}" "SELECT concat_ws('|',
  pg_get_functiondef('chat_agent_start_turn(uuid,uuid,uuid,uuid,uuid,uuid,timestamptz)'::regprocedure) LIKE '%ON CONFLICT ON CONSTRAINT chat_agent_events_pkey%',
  pg_get_functiondef('chat_agent_append_event(uuid,uuid,text,integer,jsonb,timestamptz)'::regprocedure) LIKE '%UPDATE messages AS message%')")" == "t|t" ]]
assert_retired_objects_absent "${upgrade_database}"
assert_retained_schema "${upgrade_database}"
[[ "$(psql_command "${upgrade_database}" "SELECT concat_ws('|',
  (SELECT count(*) FROM users WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM conversations WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM messages WHERE id::text LIKE '98000000-%'),
  (SELECT count(*) FROM chat_agent_turns WHERE id='98000000-0000-4000-8000-000000000031'),
  (SELECT count(*) FROM chat_agent_events WHERE turn_id='98000000-0000-4000-8000-000000000031'),
  (SELECT count(*) FROM skill_package_versions WHERE name='cleanup-proof'));")" == "${business_counts_before}" ]]

log "passed (fresh/replay to 099, immutable 096 checksum, idempotent function repair, fail-closed cleanup, retained two-user Chat/Skill data)"
