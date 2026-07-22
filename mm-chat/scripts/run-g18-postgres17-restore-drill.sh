#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
drill_dir="$(cd -- "${script_dir}/../ops/g18-postgres17" && pwd)"
postgres_dir="$(cd -- "${script_dir}/../postgres" && pwd)"
compose_file="${drill_dir}/compose.yml"
project="mmchat-g18-pg17-${UID}-$$"
report_dir="$(mktemp -d /tmp/mm-chat-g18-postgres17.XXXXXX)"
dump_file="${report_dir}/pg16-source.dump"
pg17_image_ref="mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5"
compose=(docker compose -p "${project}" -f "${compose_file}")

log() {
  printf '[g18-pg17] %s\n' "$*"
}

cleanup() {
  local status=$?
  local leaked=false
  trap - EXIT INT TERM
  set +e
  if (( status != 0 )); then
    "${compose[@]}" logs --no-color >"${report_dir}/compose.log" 2>&1
  fi
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1
  if [[ -n "$(docker ps -aq --filter "label=com.docker.compose.project=${project}")" ]] \
    || [[ -n "$(docker volume ls -q --filter "label=com.docker.compose.project=${project}")" ]]; then
    leaked=true
    status=1
  fi
  if (( status == 0 )) && [[ "${leaked}" == false ]]; then
    log "PASS reports=${report_dir} disposable_databases=removed"
  else
    log "FAIL status=${status} reports=${report_dir} leaked=${leaked}" >&2
  fi
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'required command not found: %s\n' "$1" >&2
    exit 127
  }
}

wait_for_ready() {
  local service=$1
  local database=$2
  local deadline=$((SECONDS + 180))
  until "${compose[@]}" exec -T "${service}" \
      pg_isready --username=postgres --dbname="${database}" >/dev/null 2>&1; do
    (( SECONDS < deadline )) || {
      printf 'timed out waiting for %s/%s\n' "${service}" "${database}" >&2
      return 1
    }
    sleep 1
  done
}

psql_file() {
  local service=$1
  local database=$2
  local file=$3
  shift 3
  "${compose[@]}" exec -T "${service}" \
    psql -X --set=ON_ERROR_STOP=1 "$@" \
      --username=postgres --dbname="${database}" <"${file}"
}

run_migrate() {
  local service=$1
  local database_url=$2
  local output=$3
  "${compose[@]}" run --rm --no-deps \
    -e "MIGRATION_DATABASE_URL=${database_url}" "${service}" \
    2>&1 | tee "${output}"
}

require_command docker
require_command sha256sum
docker info >/dev/null

log "project=${project} (isolated; no host ports; no project env)"
log "building digest-pinned PostgreSQL 17 extension image and migration CLI"
docker build --pull=false --tag "${pg17_image_ref}" "${postgres_dir}"
"${compose[@]}" build --pull=false migrate16

pg17_image_id="$(docker image inspect --format '{{.Id}}' "${pg17_image_ref}")"
[[ -n "${pg17_image_id}" ]] || {
  printf 'could not resolve built PostgreSQL 17 image id\n' >&2
  exit 1
}

log "proving PostgreSQL 16 PGDATA is rejected before startup"
guard_dir="${report_dir}/fake-pg16-data"
mkdir -p "${guard_dir}"
printf '16\n' >"${guard_dir}/PG_VERSION"
set +e
docker run --rm --network none \
  --mount "type=bind,src=${guard_dir},dst=/var/lib/postgresql/data" \
  "${pg17_image_id}" >"${report_dir}/pgdata-guard.log" 2>&1
guard_status=$?
set -e
[[ "${guard_status}" -eq 78 ]] || {
  printf 'PGDATA guard exited %s, expected 78\n' "${guard_status}" >&2
  exit 1
}
grep -Fq 'never mount PostgreSQL 16 PGDATA here' \
  "${report_dir}/pgdata-guard.log"
[[ "$(tr -d '[:space:]' <"${guard_dir}/PG_VERSION")" == 16 ]]

log "starting disposable PostgreSQL 16 source and PostgreSQL 17 target"
"${compose[@]}" up -d pg16 pg17
wait_for_ready pg16 mm_chat_g18_source
wait_for_ready pg17 postgres

log "applying all current migrations to PostgreSQL 16"
run_migrate migrate16 \
  'postgres://postgres@pg16:5432/mm_chat_g18_source?sslmode=disable' \
  "${report_dir}/migrate-pg16-source.log"

log "loading and verifying synthetic authority/projection state"
psql_file pg16 mm_chat_g18_source "${drill_dir}/10-synthetic-fixture.sql" \
  | tee "${report_dir}/seed-pg16.txt"
psql_file pg16 mm_chat_g18_source "${drill_dir}/20-verify-restore.sql" \
  --set=expected_major=16 --set=expect_extensions=false \
  | tee "${report_dir}/verify-pg16-source.txt"

log "creating the preserved PostgreSQL 16 logical backup"
"${compose[@]}" exec -T pg16 \
  pg_dump --format=custom --username=postgres --dbname=mm_chat_g18_source \
  >"${dump_file}"
[[ -s "${dump_file}" ]]
(cd "${report_dir}" && sha256sum "$(basename "${dump_file}")" \
  >"$(basename "${dump_file}").sha256")
(cd "${report_dir}" && sha256sum -c "$(basename "${dump_file}").sha256")

log "verifying pg_textsearch/pgvector availability and query mechanics"
psql_file pg17 postgres "${drill_dir}/00-extension-smoke.sql" \
  | tee "${report_dir}/extension-smoke.txt"

log "bootstrapping current schema once on PostgreSQL 17 to provision roles"
run_migrate migrate17 \
  'postgres://postgres@pg17:5432/postgres?sslmode=disable' \
  "${report_dir}/migrate-pg17-bootstrap.log"

log "restoring the PostgreSQL 16 backup into a fresh PostgreSQL 17 database"
"${compose[@]}" exec -T pg17 \
  createdb --username=postgres --template=template0 mm_chat_g18_restored
cat "${dump_file}" | "${compose[@]}" exec -T pg17 \
  pg_restore --exit-on-error --username=postgres --dbname=mm_chat_g18_restored
"${compose[@]}" exec -T pg17 \
  psql -X --set=ON_ERROR_STOP=1 --username=postgres \
    --dbname=mm_chat_g18_restored \
    --command="CREATE EXTENSION vector VERSION '0.8.5'; CREATE EXTENSION pg_textsearch VERSION '1.3.1';"

run_migrate migrate17 \
  'postgres://postgres@pg17:5432/mm_chat_g18_restored?sslmode=disable' \
  "${report_dir}/migrate-pg17-restored.log"
grep -Fq 'no migrations changed' "${report_dir}/migrate-pg17-restored.log"
psql_file pg17 mm_chat_g18_restored "${drill_dir}/20-verify-restore.sql" \
  --set=expected_major=17 --set=expect_extensions=true \
  | tee "${report_dir}/verify-pg17-restored.txt"

log "restoring the same preserved backup into a fresh PostgreSQL 16 rollback database"
"${compose[@]}" exec -T pg16 \
  createdb --username=postgres --template=template0 mm_chat_g18_rollback
cat "${dump_file}" | "${compose[@]}" exec -T pg16 \
  pg_restore --exit-on-error --username=postgres --dbname=mm_chat_g18_rollback
run_migrate migrate16 \
  'postgres://postgres@pg16:5432/mm_chat_g18_rollback?sslmode=disable' \
  "${report_dir}/migrate-pg16-rollback.log"
grep -Fq 'no migrations changed' "${report_dir}/migrate-pg16-rollback.log"
psql_file pg16 mm_chat_g18_rollback "${drill_dir}/20-verify-restore.sql" \
  --set=expected_major=16 --set=expect_extensions=false \
  | tee "${report_dir}/verify-pg16-rollback.txt"

log "rechecking source after both restores"
psql_file pg16 mm_chat_g18_source "${drill_dir}/20-verify-restore.sql" \
  --set=expected_major=16 --set=expect_extensions=false \
  | tee "${report_dir}/verify-pg16-source-final.txt"

log "decisive output"
grep -h 'PASS' "${report_dir}"/*.txt
