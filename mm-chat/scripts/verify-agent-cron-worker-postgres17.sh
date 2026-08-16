#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-cron-worker-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
clean_container_name="${container_name}-clean"
database_name="neo_chat_agent_cron_worker_drill"
database_user="postgres"
database_password="agent-cron-worker-drill-$(openssl rand -hex 16)"
runtime_user="agent_cron_worker_app"
runtime_password="agent-cron-worker-runtime-$(openssl rand -hex 16)"
activation_id="activation_21c5000000000001"
plan_fingerprint="sha256:1111111111111111111111111111111111111111111111111111111111111111"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" "${restore_container_name}" \
    "${clean_container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent Cron worker PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Agent Cron worker PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Agent Cron worker PostgreSQL 17 drill: Docker is required" >&2
  exit 1
}
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

start_database() {
  local name="$1"
  docker run -d --rm --name "${name}" \
    -e "POSTGRES_DB=${database_name}" -e "POSTGRES_USER=${database_user}" \
    -e "POSTGRES_PASSWORD=${database_password}" -p 127.0.0.1::5432 \
    "${postgres_image}" >/dev/null
  for _ in $(seq 1 60); do
    docker exec "${name}" pg_isready -U "${database_user}" \
      -d "${database_name}" >/dev/null 2>&1 && return
    sleep 1
  done
  return 1
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
psql_runtime() {
  local command="$1"
  docker exec -e "PGPASSWORD=${runtime_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
    --username="${runtime_user}" --dbname="${database_name}" --command "${command}"
}
expect_runtime_denied() {
  local label="$1" command="$2"
  set +e
  psql_runtime "${command}" >"${work_dir}/${label}.log" 2>&1
  local status=$?
  set -e
  if [[ "${status}" -eq 0 ]] || ! grep -Eiq 'permission denied|must be owner' "${work_dir}/${label}.log"; then
    cat "${work_dir}/${label}.log" >&2
    echo "Agent Cron worker PostgreSQL 17 drill: ${label} was not denied" >&2
    exit 1
  fi
}

log "starting disposable PostgreSQL"
start_database "${container_name}"
database_url="$(database_url_for "${container_name}")"
[[ "$(psql_command "${container_name}" 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }

log "applying and replaying migrations through schema head 096"
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/fresh.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/fresh.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/fresh.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/fresh.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "96" ]]
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "retaining two independent active and due Cron Templates"
(
  cd "${backend_dir}"
  MM_CHAT_AGENT_CRON_RETAIN_FIXTURE=1 MM_CHAT_TEST_DATABASE_URL="${database_url}" \
    go test -count=1 -run '^TestPostgresAgentCronRevisionClaimRestartOverlapAndAuthority$' \
      ./internal/agentcron
)
mapfile -t due_templates < <(psql_command "${container_name}" "
SELECT concat_ws('|',id,user_id,current_revision,current_revision_fingerprint)
FROM agent_cron_templates
WHERE state='active' AND next_trigger_at<=clock_timestamp()
ORDER BY id DESC LIMIT 2;")
if [[ "${#due_templates[@]}" -ne 2 ]]; then
  echo "Agent Cron worker PostgreSQL 17 drill: expected two due Templates" >&2
  exit 1
fi
IFS='|' read -r target_template target_user target_revision target_revision_fingerprint \
  <<<"${due_templates[0]}"
IFS='|' read -r unrelated_template _ <<<"${due_templates[1]}"

log "provisioning one immutable target and an exact function-only LOGIN"
psql_command "${container_name}" "
SELECT activation_id FROM agent_cron_worker_provision_target(
  '${activation_id}','${target_template}','${target_user}'::uuid,${target_revision},
  '${target_revision_fingerprint}','${plan_fingerprint}',
  clock_timestamp()-interval '1 minute',clock_timestamp()+interval '1 hour',
  'g21.5-cron-operator','G21_5_CRON_APPROVED');
CREATE ROLE ${runtime_user} LOGIN INHERIT PASSWORD '${runtime_password}';
GRANT agent_cron_worker TO ${runtime_user};
DO \$\$
DECLARE inherited_roles name[];
BEGIN
  WITH RECURSIVE inherited(role_oid) AS (
    SELECT membership.roleid FROM pg_auth_members membership
    JOIN pg_roles login ON login.oid=membership.member
    WHERE login.rolname='${runtime_user}'
    UNION
    SELECT membership.roleid FROM pg_auth_members membership
    JOIN inherited ON inherited.role_oid=membership.member
  )
  SELECT array_agg(role.rolname ORDER BY role.rolname) INTO inherited_roles
  FROM inherited JOIN pg_roles role ON role.oid=inherited.role_oid;
  IF inherited_roles<>ARRAY['agent_cron_worker']::name[] THEN
    RAISE EXCEPTION 'Cron worker role membership drift: %',inherited_roles;
  END IF;
  IF EXISTS(SELECT 1 FROM pg_roles WHERE rolname='${runtime_user}' AND (
      NOT rolcanlogin OR NOT rolinherit OR rolsuper OR rolcreatedb OR rolcreaterole
      OR rolreplication OR rolbypassrls))
     OR pg_has_role('${runtime_user}','agent_cron_owner','MEMBER')
     OR pg_has_role('${runtime_user}','agent_cron_control','MEMBER')
     OR pg_has_role('${runtime_user}','agent_learning_worker','MEMBER')
     OR has_table_privilege('${runtime_user}','agent_cron_templates','SELECT,INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_cron_worker_targets','SELECT,INSERT,UPDATE,DELETE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_cron_worker_claim_due(text,text,timestamp with time zone,integer,integer)','EXECUTE')
     OR has_function_privilege('${runtime_user}',
       'agent_cron_worker_provision_target(text,text,uuid,bigint,text,text,timestamp with time zone,timestamp with time zone,text,text)','EXECUTE')
     OR has_function_privilege('${runtime_user}',
       'agent_cron_claim_due(text,timestamp with time zone,integer,integer)','EXECUTE') THEN
    RAISE EXCEPTION 'Cron worker function-only authority drifted';
  END IF;
END
\$\$;" >/dev/null

log "proving direct DML and provisioning/control authority are denied"
expect_runtime_denied direct_select "SELECT count(*) FROM agent_cron_templates;"
expect_runtime_denied direct_update "UPDATE agent_cron_templates SET updated_at=clock_timestamp();"
expect_runtime_denied broad_claim \
  "SELECT * FROM agent_cron_claim_due('forged',clock_timestamp(),30,1);"
expect_runtime_denied provision \
  "SELECT agent_cron_worker_provision_target('${activation_id}','${target_template}',
    '${target_user}'::uuid,${target_revision},'${target_revision_fingerprint}',
    '${plan_fingerprint}',clock_timestamp(),clock_timestamp()+interval '1 hour',
    'forged','FORGED');"

log "claiming only the exact activation-bound Template"
forged_count="$(psql_runtime "SELECT count(*) FROM agent_cron_worker_claim_due(
  'activation_21c500000000ffff','cron-worker-forged',clock_timestamp(),30,100);")"
[[ "${forged_count}" == "0" ]]
claimed_template="$(psql_runtime "SELECT template_id FROM agent_cron_worker_claim_due(
  '${activation_id}','cron-worker-exact',clock_timestamp(),30,100);")"
[[ "${claimed_template}" == "${target_template}" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_cron_templates
  WHERE id='${unrelated_template}' AND cursor_claim_owner IS NULL;")" == "1" ]]
[[ "$(psql_runtime "SELECT count(*) FROM agent_cron_worker_claim_due(
  '${activation_id}','cron-worker-replay',clock_timestamp(),30,100);")" == "0" ]]

log "dumping and restoring target, cursor and Cron authority"
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" \
  >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_cron_worker_targets),
  (SELECT count(*) FROM agent_cron_templates),
  (SELECT count(*) FROM agent_cron_triggers),
  (SELECT count(*) FROM agent_cron_audit_events));")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" \
  psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" \
  <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_cron_worker_targets),
  (SELECT count(*) FROM agent_cron_templates),
  (SELECT count(*) FROM agent_cron_triggers),
  (SELECT count(*) FROM agent_cron_audit_events));")"
[[ "${source_counts}" == "${restore_counts}" ]]

run_migrate down >"${work_dir}/peel-096-chat-agent-event-tail.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/peel-096-chat-agent-event-tail.log"
run_migrate down >"${work_dir}/peel-095-tail.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/peel-095-tail.log"
log "proving active, LOGIN-membership and retained-fact down guards"
set +e
run_migrate down >"${work_dir}/active-guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] ||
   ! grep -Fq 'AGENT_WORKER_DOWN_ACTIVE_TARGET' "${work_dir}/active-guard.log"; then
  cat "${work_dir}/active-guard.log" >&2
  exit 1
fi
psql_command "${container_name}" "SELECT agent_cron_worker_disable_target(
  '${activation_id}','g21.5-cron-operator','G21_5_CRON_DISABLED');" >/dev/null
psql_runtime "SELECT * FROM agent_cron_worker_reconcile(
  '${activation_id}',clock_timestamp()+interval '10 minutes',100);" >/dev/null
set +e
run_migrate down >"${work_dir}/membership-guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] ||
   ! grep -Fq 'AGENT_WORKER_DOWN_LOGIN_MEMBERSHIP' "${work_dir}/membership-guard.log"; then
  cat "${work_dir}/membership-guard.log" >&2
  exit 1
fi
psql_command "${container_name}" "REVOKE agent_cron_worker FROM ${runtime_user}; DROP ROLE ${runtime_user};" >/dev/null
set +e
run_migrate down >"${work_dir}/retained-guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] ||
   ! grep -Fq 'AGENT_WORKER_DOWN_RETAINED_ACTIVATION_FACTS' "${work_dir}/retained-guard.log"; then
  cat "${work_dir}/retained-guard.log" >&2
  exit 1
fi

log "proving clean disposable 094 down/up and final head 096"
start_database "${clean_container_name}"
clean_database_url="$(database_url_for "${clean_container_name}")"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" up \
  >"${work_dir}/clean-up.log" 2>&1
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" down \
  >"${work_dir}/clean-down-096.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/clean-down-096.log"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" down \
  >"${work_dir}/clean-down-095.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/clean-down-095.log"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" down \
  >"${work_dir}/clean-down.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/clean-down.log"
MIGRATION_DATABASE_URL="${clean_database_url}" "${work_dir}/migrate" up \
  >"${work_dir}/clean-reup.log" 2>&1
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/clean-reup.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/clean-reup.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/clean-reup.log"
[[ "$(psql_command "${clean_container_name}" 'SELECT max(version) FROM schema_migrations')" == "96" ]]

log "passed (fresh/replay, exact LOGIN, scoped claim, denied widening, dump/restore, guarded and clean down/up)"
