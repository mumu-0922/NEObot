#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-skill-supply-pg17-$RANDOM-$$"
database_name="neo_chat_skill_supply_drill"
database_user="postgres"
database_password="skill-supply-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
trap cleanup EXIT INT TERM

log() { printf 'Skill supply PostgreSQL 17 drill: %s\n' "$*"; }

if ! command -v docker >/dev/null 2>&1 || ! docker version >/dev/null 2>&1; then
  echo "Skill supply PostgreSQL 17 drill: Docker is required" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "Skill supply PostgreSQL 17 drill: openssl is required" >&2
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
  echo "Skill supply PostgreSQL 17 drill: database did not become ready" >&2
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

log "building and applying 001 -> 096"
(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/mm-chat-migrate" ./cmd/migrate)
run_migrate() { MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/mm-chat-migrate" "$@"; }
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 083_skill_supply_chain" "${work_dir}/fresh.log"
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/fresh.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/fresh.log"
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/fresh.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/fresh.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/fresh.log"
grep -Fq "up 089_agent_draft_learning" "${work_dir}/fresh.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/fresh.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/fresh.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/fresh.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/fresh.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/fresh.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/fresh.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/fresh.log"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "checking schema, composite authority, trigger, and least-privilege grants"
psql_command "
DO \$\$
BEGIN
  IF to_regclass('public.skill_package_versions') IS NULL
     OR to_regclass('public.skill_package_candidates') IS NULL
     OR to_regclass('public.skill_installations') IS NULL THEN
    RAISE EXCEPTION 'Skill supply tables are missing';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger
    WHERE tgname = 'trg_skill_installation_admission' AND NOT tgisinternal
  ) THEN
    RAISE EXCEPTION 'Skill admission trigger is missing';
  END IF;
  IF NOT has_table_privilege('go_api_runtime', 'skill_package_versions', 'SELECT,INSERT')
     OR NOT has_table_privilege('go_api_runtime', 'skill_package_candidates', 'SELECT,INSERT')
     OR NOT has_column_privilege('go_api_runtime', 'skill_package_candidates', 'status', 'UPDATE')
     OR has_column_privilege('go_api_runtime', 'skill_package_candidates', 'source_ref', 'UPDATE')
     OR NOT has_table_privilege('go_api_runtime', 'skill_installations', 'SELECT,INSERT,DELETE') THEN
    RAISE EXCEPTION 'Skill runtime grants violate the contract';
  END IF;
END
\$\$;
" >/dev/null

log "running source-drift, review-CAS, ownership, install and uninstall lifecycle"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  go test -count=1 -run '^TestSkillPostgresRepositoryAuthorityDriftOwnershipAndCAS$' ./internal/skillsupply)

log "rolling back the empty 094-084 tails before the 083 guard"
run_migrate down >"${work_dir}/peel-096-chat-agent-event-tail.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/peel-096-chat-agent-event-tail.log"
run_migrate down >"${work_dir}/peel-095-tail-1.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/peel-095-tail-1.log"
run_migrate down >"${work_dir}/peel-094-tail-1.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/peel-094-tail-1.log"
run_migrate down >"${work_dir}/peel-093-tail-1.log" 2>&1
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/peel-093-tail-1.log"
run_migrate down >"${work_dir}/peel-092-tail-1.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/peel-092-tail-1.log"
run_migrate down >"${work_dir}/peel-091-tail-1.log" 2>&1
grep -Fq "down 091_agent_artifact_publication" "${work_dir}/peel-091-tail-1.log"
run_migrate down >"${work_dir}/down-090.log" 2>&1
grep -Fq "down 090_agent_product_shadow" "${work_dir}/down-090.log"
run_migrate down >"${work_dir}/down-089.log" 2>&1
grep -Fq "down 089_agent_draft_learning" "${work_dir}/down-089.log"
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

log "proving non-empty guarded down"
set +e
run_migrate down >"${work_dir}/guarded-down.log" 2>&1
guard_status=$?
set -e
if [[ "${guard_status}" -eq 0 ]] || ! grep -Fq "SKILL_SUPPLY_CHAIN_DOWN_DATA_EXISTS" "${work_dir}/guarded-down.log"; then
  cat "${work_dir}/guarded-down.log" >&2
  echo "Skill supply PostgreSQL 17 drill: non-empty down did not fail closed" >&2
  exit 1
fi

log "proving clean 082 -> 083 -> 082 -> 095 replay"
psql_command "TRUNCATE TABLE skill_installations, skill_package_candidates, skill_package_versions;" >/dev/null
run_migrate down >"${work_dir}/down.log" 2>&1
grep -Fq "down 083_skill_supply_chain" "${work_dir}/down.log"
psql_command "
DO \$\$
BEGIN
  IF to_regclass('public.skill_package_versions') IS NOT NULL
     OR to_regclass('public.skill_package_candidates') IS NOT NULL
     OR to_regclass('public.skill_installations') IS NOT NULL THEN
    RAISE EXCEPTION '083 down retained Skill supply tables';
  END IF;
END
\$\$;
" >/dev/null
run_migrate up >"${work_dir}/reup.log" 2>&1
grep -Fq "up 083_skill_supply_chain" "${work_dir}/reup.log"
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/reup.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/reup.log"
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/reup.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/reup.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/reup.log"
grep -Fq "up 089_agent_draft_learning" "${work_dir}/reup.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/reup.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/reup.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/reup.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/reup.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/reup.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/reup.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/reup.log"
run_migrate up >"${work_dir}/final-replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/final-replay.log"

log "passed (fresh/replay, schema/grants, drift/CAS/ownership lifecycle, guarded down, clean down/up)"
