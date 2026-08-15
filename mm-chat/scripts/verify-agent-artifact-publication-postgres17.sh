#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-artifact-pg17-$RANDOM-$$"
restore_container_name="${container_name}-restore"
database_name="neo_chat_agent_artifact_drill"
database_user="postgres"
database_password="agent-artifact-drill-$(openssl rand -hex 16)"
runtime_user="agent_broker_canary_app"
runtime_password="artifact-runtime-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" "${restore_container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent Artifact PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Agent Artifact PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Agent Artifact PostgreSQL 17 drill: Docker is required" >&2
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

log "applying and replaying migrations through schema head 094"
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 090_agent_product_shadow" "${work_dir}/fresh.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/fresh.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/fresh.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/fresh.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/fresh.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "94" ]]
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "provisioning the independent exact-membership LOGIN"
psql_command "${container_name}" "
CREATE ROLE ${runtime_user} LOGIN INHERIT PASSWORD '${runtime_password}';
GRANT agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_artifact_control
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
    'agent_artifact_control','agent_effect_control','agent_orchestrator_runtime','agent_runner_control'
  ]::name[] THEN
    RAISE EXCEPTION 'Broker canary role membership drift: %',inherited_roles;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='${runtime_user}' AND (
      NOT rolcanlogin OR NOT rolinherit OR rolsuper OR rolcreatedb OR rolcreaterole
      OR rolreplication OR rolbypassrls)) THEN
    RAISE EXCEPTION 'Broker canary LOGIN attributes drifted';
  END IF;
  IF has_table_privilege('${runtime_user}','agent_artifacts','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_effect_intents','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_attempts','INSERT,UPDATE,DELETE')
     OR pg_has_role('${runtime_user}','agent_product_owner','MEMBER')
     OR pg_has_role('${runtime_user}','agent_effect_owner','MEMBER')
     OR pg_has_role('${runtime_user}','agent_orchestrator_owner','MEMBER')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_artifact_authorize(text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)','EXECUTE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_artifact_attach(text,text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)','EXECUTE')
     OR has_function_privilege('agent_runner_control',
       'agent_artifact_attach(text,text,uuid,text,text,bigint,text,text,text,text,text,bigint,text,text)','EXECUTE') THEN
    RAISE EXCEPTION 'Artifact function-only authority drifted';
  END IF;
END
\$\$;" >/dev/null

log "proving authorize/attach replay, collision, lease and Grant fences"
(
  cd "${backend_dir}"
  MM_CHAT_TEST_DATABASE_URL="${database_url}" \
    MM_CHAT_AGENT_ARTIFACT_DATABASE_URL="${runtime_url}" \
    MM_CHAT_AGENT_ARTIFACT_RETAIN_FIXTURE=1 \
    go test -count=1 -race -run '^TestPostgresArtifactPublicationAuthorizeAttachReplayAndFences$' \
      ./internal/agentbroker
)

log "dumping and restoring retained Artifact authority"
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" \
  >"${work_dir}/authority.sql"
source_counts="$(psql_command "${container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_artifacts),(SELECT count(*) FROM agent_effect_intents),
  (SELECT count(*) FROM agent_effect_grant_revocations));")"
start_database "${restore_container_name}"
docker exec -i -e "PGPASSWORD=${database_password}" "${restore_container_name}" \
  psql --set=ON_ERROR_STOP=1 --no-psqlrc -U "${database_user}" -d "${database_name}" \
  <"${work_dir}/authority.sql" >/dev/null
restore_counts="$(psql_command "${restore_container_name}" "SELECT concat_ws(',',
  (SELECT count(*) FROM agent_artifacts),(SELECT count(*) FROM agent_effect_intents),
  (SELECT count(*) FROM agent_effect_grant_revocations));")"
[[ "${source_counts}" == "${restore_counts}" ]]

log "proving role-aware 091 down/up preserves Artifact rows"
psql_command "${container_name}" "
REVOKE agent_orchestrator_runtime,agent_runner_control,agent_effect_control,agent_artifact_control
  FROM ${runtime_user};
DROP ROLE ${runtime_user};" >/dev/null
run_migrate down >"${work_dir}/peel-094-tail-1.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/peel-094-tail-1.log"
run_migrate down >"${work_dir}/peel-093-tail-1.log" 2>&1
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/peel-093-tail-1.log"
run_migrate down >"${work_dir}/peel-092.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/peel-092.log"
run_migrate down >"${work_dir}/down-091.log" 2>&1
grep -Fq "down 091_agent_artifact_publication" "${work_dir}/down-091.log"
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM agent_artifacts")" == "1" ]]
[[ "$(psql_command "${container_name}" "SELECT count(*) FROM pg_roles WHERE rolname='agent_artifact_control'")" == "0" ]]
run_migrate up >"${work_dir}/reup-091.log" 2>&1
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/reup-091.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/reup-091.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/reup-091.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/reup-091.log"

log "proving clean down/up and final head 094"
psql_command "${container_name}" "
TRUNCATE agent_effect_grant_revocations;
DELETE FROM agent_runs WHERE id='run_2121212121212121';
DELETE FROM agent_run_snapshots WHERE id NOT IN (SELECT snapshot_id FROM agent_runs);" >/dev/null
run_migrate down >"${work_dir}/peel-094-tail-2.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/peel-094-tail-2.log"
run_migrate down >"${work_dir}/peel-093-tail-2.log" 2>&1
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/peel-093-tail-2.log"
run_migrate down >"${work_dir}/clean-peel-092-2.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/clean-peel-092-2.log"
run_migrate down >"${work_dir}/clean-down-091.log" 2>&1
grep -Fq "down 091_agent_artifact_publication" "${work_dir}/clean-down-091.log"
run_migrate up >"${work_dir}/final-up.log" 2>&1
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/final-up.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/final-up.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/final-up.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/final-up.log"
[[ "$(psql_command "${container_name}" 'SELECT max(version) FROM schema_migrations')" == "94" ]]
log "passed (fresh/replay, exact role, authorize/attach, collision/fences, dump/restore, down/up)"
