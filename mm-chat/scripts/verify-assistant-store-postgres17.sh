#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-assistant-pg17-$RANDOM-$$"
database_name="neo_chat_assistant_drill"
database_user="postgres"
database_password="assistant-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
trap cleanup EXIT INT TERM

log() { printf 'Assistant Store PostgreSQL 17 drill: %s\n' "$*"; }

if ! command -v docker >/dev/null 2>&1 || ! docker version >/dev/null 2>&1; then
  echo "Assistant Store PostgreSQL 17 drill: Docker is required" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "Assistant Store PostgreSQL 17 drill: openssl is required" >&2
  exit 1
fi
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  log "building the project PostgreSQL image"
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

log "starting disposable database"
docker run -d --rm \
  --name "${container_name}" \
  -e "POSTGRES_DB=${database_name}" \
  -e "POSTGRES_USER=${database_user}" \
  -e "POSTGRES_PASSWORD=${database_password}" \
  -p 127.0.0.1::5432 \
  "${postgres_image}" >/dev/null

for _ in $(seq 1 60); do
  if docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null 2>&1; then
  echo "Assistant Store PostgreSQL 17 drill: database did not become ready" >&2
  exit 1
fi

host_port="$(docker port "${container_name}" 5432/tcp | awk -F: 'NR == 1 {print $NF}')"
database_url="postgres://${database_user}:${database_password}@127.0.0.1:${host_port}/${database_name}?sslmode=disable"

psql_command() {
  docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
    --username="${database_user}" --dbname="${database_name}" --command "$1"
}

server_major="$(psql_command "SHOW server_version_num" | cut -c1-2)"
[[ "${server_major}" == "17" ]] || { echo "expected PostgreSQL 17" >&2; exit 1; }

log "building and applying 001 -> 088"
(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/mm-chat-migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/mm-chat-migrate" "$@"; }
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 082_assistant_library" "${work_dir}/fresh.log"
grep -Fq "up 083_skill_supply_chain" "${work_dir}/fresh.log"
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/fresh.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/fresh.log"
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/fresh.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/fresh.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/fresh.log"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "checking schema, ownership indexes, JSON metadata, and runtime grants"
psql_command "
DO \$\$
BEGIN
  IF to_regclass('public.assistant_library_entries') IS NULL
     OR to_regclass('public.assistant_market_admissions') IS NULL THEN
    RAISE EXCEPTION 'Assistant Store tables are missing';
  END IF;
  IF to_regclass('public.idx_assistant_library_user_store_unique') IS NULL THEN
    RAISE EXCEPTION 'partial Store uniqueness index is missing';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'assistant_library_entries'
      AND column_name = 'required_tools' AND udt_name = 'jsonb'
  ) THEN
    RAISE EXCEPTION 'required_tools JSONB is missing';
  END IF;
  IF NOT has_table_privilege(
    'go_api_runtime', 'assistant_library_entries', 'SELECT,INSERT,UPDATE,DELETE'
  ) THEN
    RAISE EXCEPTION 'Assistant library runtime grants are missing';
  END IF;
END
\$\$;
" >/dev/null

log "running repository ownership and CAS lifecycle"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  go test -count=1 -run '^TestAssistantPostgresRepositoryAuthorityAndCAS$' ./internal/agents)

log "rolling back the clean 088 through 083 tails before the Assistant 082 replay"
run_migrate down >"${work_dir}/down-088.log" 2>&1
grep -Fq "down 088_agent_cron_foundation" "${work_dir}/down-088.log"
run_migrate down >"${work_dir}/down-087.log" 2>&1
grep -Fq "down 087_agent_child_delegation" "${work_dir}/down-087.log"
run_migrate down >"${work_dir}/down-086.log" 2>&1
grep -Fq "down 086_agent_broker_foundation" "${work_dir}/down-086.log"
run_migrate down >"${work_dir}/down-085.log" 2>&1
grep -Fq "down 085_agent_runner_foundation" "${work_dir}/down-085.log"
run_migrate down >"${work_dir}/down-084.log" 2>&1
grep -Fq "down 084_agent_orchestrator_foundation" "${work_dir}/down-084.log"
run_migrate down >"${work_dir}/down-083.log" 2>&1
grep -Fq "down 083_skill_supply_chain" "${work_dir}/down-083.log"

log "proving clean 081 -> 082 -> 081 -> 088 replay"
run_migrate down >"${work_dir}/down.log" 2>&1
grep -Fq "down 082_assistant_library" "${work_dir}/down.log"
psql_command "
DO \$\$
BEGIN
  IF to_regclass('public.assistant_library_entries') IS NOT NULL
     OR to_regclass('public.assistant_market_admissions') IS NOT NULL THEN
    RAISE EXCEPTION '082 down retained Assistant Store tables';
  END IF;
END
\$\$;
" >/dev/null
run_migrate up >"${work_dir}/reup.log" 2>&1
grep -Fq "up 082_assistant_library" "${work_dir}/reup.log"
grep -Fq "up 083_skill_supply_chain" "${work_dir}/reup.log"
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/reup.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/reup.log"
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/reup.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/reup.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/reup.log"
run_migrate up >"${work_dir}/final-replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/final-replay.log"

log "passed (fresh through 088, schema/grants, repository ownership/CAS, clean 082 down/up with 083-088 tail replay)"
