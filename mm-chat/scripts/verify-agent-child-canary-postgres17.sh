#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-child-canary-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
database_name="neo_chat_agent_child_canary_drill"
database_user="postgres"
database_password="agent-child-canary-drill-$(openssl rand -hex 16)"
runtime_user="agent_child_canary_app"
runtime_password="child-runtime-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" "${restore_container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent Child canary PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Agent Child canary PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Agent Child canary PostgreSQL 17 drill: Docker is required" >&2
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
    docker exec "${name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1 && return
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
runtime_url_for() {
  local name="$1" port
  port="$(docker port "${name}" 5432/tcp | sed -n '1s/.*://p')"
  printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable' \
    "${runtime_user}" "${runtime_password}" "${port}" "${database_name}"
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
runtime_url="$(runtime_url_for "${container_name}")"
[[ "$(psql_command "${container_name}" 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" "$@"; }

log "applying and replaying migrations through schema head 095"
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/fresh.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/fresh.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/fresh.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/fresh.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/fresh.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/fresh.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "95" ]]
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "provisioning the tenth exact-membership LOGIN"
psql_command "${container_name}" "
CREATE ROLE ${runtime_user} LOGIN INHERIT PASSWORD '${runtime_password}';
GRANT agent_orchestrator_runtime,agent_runner_control,agent_delegation_control
  TO ${runtime_user};
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
  IF inherited_roles <> ARRAY[
    'agent_delegation_control','agent_orchestrator_runtime','agent_runner_control'
  ]::name[] THEN
    RAISE EXCEPTION 'Child canary role membership drift: %',inherited_roles;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='${runtime_user}' AND (
      NOT rolcanlogin OR NOT rolinherit OR rolsuper OR rolcreatedb OR rolcreaterole
      OR rolreplication OR rolbypassrls)) THEN
    RAISE EXCEPTION 'Child canary LOGIN attributes drifted';
  END IF;
  IF pg_has_role('${runtime_user}','agent_delegation_owner','MEMBER')
     OR pg_has_role('${runtime_user}','agent_orchestrator_owner','MEMBER')
     OR pg_has_role('${runtime_user}','agent_runner_owner','MEMBER')
     OR has_table_privilege('${runtime_user}','agent_delegation_reaps','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_delegation_authorities','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_attempts','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_runner_requests','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_runner_sandboxes','INSERT,UPDATE,DELETE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_delegation_reap_inventory(integer)','EXECUTE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_delegation_complete_reap(text,boolean,text)','EXECUTE')
     OR has_function_privilege('go_api_runtime',
       'agent_delegation_reap_inventory(integer)','EXECUTE')
     OR has_function_privilege('agent_effect_control',
       'agent_delegation_reap_inventory(integer)','EXECUTE') THEN
    RAISE EXCEPTION 'Child canary function-only authority drifted';
  END IF;
END
\$\$;" >/dev/null

log "proving exact Parent/Child lineage, late-launch fence and atomic reap"
(
  cd "${backend_dir}"
  MM_CHAT_AGENT_CHILD_CANARY_DATABASE_URL="${database_url}" \
    MM_CHAT_AGENT_CHILD_CANARY_RUNTIME_DATABASE_URL="${runtime_url}" \
    go test -count=1 -race -run '^TestPostgresAgentChildReapTransport$' \
      ./internal/agentdelegation
)

log "running G20.5 depth/subset/stale/restart regression at head 095"
(
  cd "${backend_dir}"
  MM_CHAT_TEST_DATABASE_URL="${database_url}" \
    go test -count=1 -race -run '^TestPostgresAgentDelegationDepthBudgetLaunchCascadeAndSettlement$' \
      ./internal/agentdelegation
)

log "checking exact durable Child inventory"
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_delegation_lineage
  WHERE idempotency_key='g21.4-child-canary-child-postgres'")" == "1" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_delegation_authorities
  WHERE depth=1 AND registry_json->'tools'='[]'::jsonb")" -ge "1" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_delegation_reaps
  WHERE state='reaped' AND completed_at IS NOT NULL")" -ge "1" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_runner_sandboxes
  WHERE state='terminal' AND observed_terminal='canceled'")" == "1" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_delegation_reap_inventory(100)")" == "0" ]]

log "dumping and restoring lineage, launch authority and Runner projection"
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" \
  >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_delegation_authorities),
  (SELECT count(*) FROM agent_delegation_lineage),
  (SELECT count(*) FROM agent_delegation_reaps),
  (SELECT count(*) FROM agent_runner_requests),
  (SELECT count(*) FROM agent_runner_sandboxes));")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" \
  psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" \
  <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_delegation_authorities),
  (SELECT count(*) FROM agent_delegation_lineage),
  (SELECT count(*) FROM agent_delegation_reaps),
  (SELECT count(*) FROM agent_runner_requests),
  (SELECT count(*) FROM agent_runner_sandboxes));")"
[[ "${source_counts}" == "${restore_counts}" ]]

run_migrate down >"${work_dir}/peel-095-tail.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/peel-095-tail.log"
run_migrate down >"${work_dir}/peel-094-tail.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/peel-094-tail.log"
log "proving dirty migration 093 cannot be peeled"
psql_command "${container_name}" "
UPDATE agent_delegation_reaps SET state='failed',completed_at=NULL,
  error_code='ROLLBACK_GUARD' WHERE reap_id=(
    SELECT reap_id FROM agent_delegation_reaps ORDER BY created_at LIMIT 1
  );" >/dev/null
set +e
run_migrate down >"${work_dir}/guard.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] ||
   ! grep -Fq 'AGENT_CHILD_REAP_TRANSPORT_DOWN_REQUIRES_CLEAN' "${work_dir}/guard.log"; then
  cat "${work_dir}/guard.log" >&2
  exit 1
fi

log "proving clean 092 behavior and reapplying migration 093"
psql_command "${container_name}" "
UPDATE agent_delegation_reaps SET state='reaped',completed_at=clock_timestamp(),
  error_code=NULL WHERE state IN ('pending','failed');" >/dev/null
run_migrate down >"${work_dir}/down-093.log" 2>&1
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/down-093.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "92" ]]
psql_command "${container_name}" "
DO \$\$
BEGIN
  IF to_regprocedure('agent_delegation_reap_inventory(integer)') IS NOT NULL
     OR position('AGENT_REAP_AUTHORITY_ACTIVE' in pg_get_functiondef(
       'agent_delegation_complete_reap(text,boolean,text)'::regprocedure))<>0
     OR has_table_privilege('agent_delegation_owner','agent_runner_sandboxes','UPDATE')
     OR has_table_privilege('agent_delegation_owner','agent_runner_requests','SELECT') THEN
    RAISE EXCEPTION 'migration 087 reap behavior was not restored';
  END IF;
END
\$\$;" >/dev/null
run_migrate up >"${work_dir}/reup-093.log" 2>&1
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/reup-093.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/reup-093.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/reup-093.log"
run_migrate up >"${work_dir}/final.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/final.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "95" ]]
log "passed (fresh/replay, tenth LOGIN, exact depth-one lineage, late-launch/retry/mismatch/replay fences, atomic Runner reap, dump/restore, guarded clean down/up)"
