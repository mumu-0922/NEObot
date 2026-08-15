#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-agent-root-canary-pg17-$RANDOM-$$"
database_name="neo_chat_agent_root_canary_drill"
database_user="postgres"
database_password="agent-root-canary-drill-$(openssl rand -hex 16)"
runtime_user="agent_root_canary_app"
runtime_password="root-canary-runtime-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent Root canary PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Agent Root canary PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Agent Root canary PostgreSQL 17 drill: Docker is required" >&2
  exit 1
}
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

log "starting disposable database"
docker run -d --rm --name "${container_name}" \
  -e "POSTGRES_DB=${database_name}" \
  -e "POSTGRES_USER=${database_user}" \
  -e "POSTGRES_PASSWORD=${database_password}" \
  -p 127.0.0.1::5432 "${postgres_image}" >/dev/null
for _ in $(seq 1 60); do
  if docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null
port="$(docker port "${container_name}" 5432/tcp | awk -F: 'NR==1{print $NF}')"
admin_url="postgres://${database_user}:${database_password}@127.0.0.1:${port}/${database_name}?sslmode=disable"
runtime_url="postgres://${runtime_user}:${runtime_password}@127.0.0.1:${port}/${database_name}?sslmode=disable"
psql_command() {
  docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
      --username="${database_user}" --dbname="${database_name}" --command "$1"
}
[[ "$(psql_command 'SHOW server_version_num' | cut -c1-2)" == "17" ]]

log "applying migrations through schema head 095"
(cd "${backend_dir}" && go build -buildvcs=false -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
MIGRATION_DATABASE_URL="${admin_url}" "${work_dir}/migrate" up >"${work_dir}/migrate.log" 2>&1
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/migrate.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/migrate.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/migrate.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/migrate.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/migrate.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/migrate.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/migrate.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/migrate.log"
[[ "$(psql_command "SELECT max(version) FROM schema_migrations")" == "95" ]]

log "provisioning the independent exact-membership LOGIN"
psql_command "
CREATE ROLE ${runtime_user} LOGIN INHERIT PASSWORD '${runtime_password}';
GRANT agent_orchestrator_runtime,agent_runner_control TO ${runtime_user};
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
  IF inherited_roles <> ARRAY['agent_orchestrator_runtime','agent_runner_control']::name[] THEN
    RAISE EXCEPTION 'Root canary role membership drift: %',inherited_roles;
  END IF;
  IF EXISTS (
    SELECT 1 FROM pg_roles WHERE rolname='${runtime_user}' AND (
      NOT rolcanlogin OR NOT rolinherit OR rolsuper OR rolcreatedb OR rolcreaterole
      OR rolreplication OR rolbypassrls
    )
  ) THEN
    RAISE EXCEPTION 'Root canary LOGIN attributes drifted';
  END IF;
  IF has_table_privilege('${runtime_user}','agent_runs','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_attempts','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_runner_requests','INSERT,UPDATE,DELETE')
     OR has_table_privilege('${runtime_user}','agent_runner_sandboxes','INSERT,UPDATE,DELETE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_orchestrator_transition(text,uuid,text,text,text,bigint,text,text,text,text,text,text,text,text,jsonb)','EXECUTE')
     OR NOT has_function_privilege('${runtime_user}',
       'agent_runner_update_sandbox(text,text,bigint,text,text,text)','EXECUTE') THEN
    RAISE EXCEPTION 'Root canary least privilege drifted';
  END IF;
END
\$\$;
" >/dev/null

log "proving atomic Sandbox/Attempt/Step/Run cancellation and rollback"
(
  cd "${backend_dir}"
  MM_CHAT_TEST_DATABASE_URL="${admin_url}" \
    MM_CHAT_AGENT_ROOT_CANARY_DATABASE_URL="${runtime_url}" \
    go test -count=1 -race \
      -run '^TestPostgresTerminalRepositoryCommitsWholeCanceledChainOrRollsBack$' \
      ./internal/agentrootcanary
)

log "peeling and reapplying the empty migrations 094, 093 and 092 tail"
MIGRATION_DATABASE_URL="${admin_url}" "${work_dir}/migrate" down >"${work_dir}/peel-095-tail-1.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/peel-095-tail-1.log"
MIGRATION_DATABASE_URL="${admin_url}" "${work_dir}/migrate" down >"${work_dir}/peel-094-tail-1.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/peel-094-tail-1.log"
MIGRATION_DATABASE_URL="${admin_url}" "${work_dir}/migrate" down >"${work_dir}/peel-093-tail-1.log" 2>&1
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/peel-093-tail-1.log"
MIGRATION_DATABASE_URL="${admin_url}" "${work_dir}/migrate" down >"${work_dir}/peel-092.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/peel-092.log"
MIGRATION_DATABASE_URL="${admin_url}" "${work_dir}/migrate" up >"${work_dir}/reup-092.log" 2>&1
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/reup-092.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/reup-092.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/reup-092.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/reup-092.log"
[[ "$(psql_command "SELECT max(version) FROM schema_migrations")" == "95" ]]

log "passed (PostgreSQL 17, exact role inheritance, rollback, atomic terminal chain; no Artifact role use)"
