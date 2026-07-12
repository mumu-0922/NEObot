#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bakeoff_dir="$(cd "${script_dir}/../ops/bakeoff/postgres" && pwd)"
compose_file="${bakeoff_dir}/compose.yml"
project="mmchat-p15-pdb-${UID}-$$"
report_dir="$(mktemp -d "/tmp/mm-chat-phase15-pg-bakeoff.XXXXXX")"
resource_report="${report_dir}/resources.tsv"
compose=(docker compose -p "${project}" -f "${compose_file}")
cleaned=false

log() {
  printf '[phase15-pg] %s\n' "$*"
}

cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if [[ "${cleaned}" != true ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
    cleaned=true
  fi
  if (( status == 0 )); then
    log "PASS reports=${report_dir}"
  else
    log "FAIL status=${status} reports=${report_dir}" >&2
    "${compose[@]}" logs --no-color postgres >"${report_dir}/postgres.log" 2>&1 || true
  fi
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'required command not found: %s\n' "$1" >&2
    exit 1
  }
}

psql_file() {
  local database=$1
  local file=$2
  "${compose[@]}" exec -T postgres \
    psql -X --set=ON_ERROR_STOP=1 --username=postgres \
    --dbname="${database}" <"${file}"
}

psql_command() {
  local database=$1
  shift
  "${compose[@]}" exec -T postgres \
    psql -X --set=ON_ERROR_STOP=1 --username=postgres \
    --dbname="${database}" "$@"
}

wait_for_bootstrap_then_ready() {
  local deadline=$((SECONDS + 180))
  log "waiting for ParadeDB bootstrap restart marker"
  until "${compose[@]}" logs --no-color postgres 2>&1 |
      grep -Fq 'PostgreSQL init process complete; ready for start up'; do
    (( SECONDS < deadline )) || {
      printf 'timed out waiting for ParadeDB bootstrap restart\n' >&2
      return 1
    }
    sleep 1
  done

  log "bootstrap restart observed; now waiting for pg_isready"
  deadline=$((SECONDS + 120))
  until "${compose[@]}" exec -T postgres \
      pg_isready --username=postgres --dbname=postgres >/dev/null 2>&1; do
    (( SECONDS < deadline )) || {
      printf 'timed out waiting for PostgreSQL readiness\n' >&2
      return 1
    }
    sleep 1
  done
}

wait_for_ready() {
  local database=${1:-postgres}
  local deadline=$((SECONDS + 120))
  until "${compose[@]}" exec -T postgres \
      pg_isready --username=postgres --dbname="${database}" >/dev/null 2>&1; do
    (( SECONDS < deadline )) || {
      printf 'timed out waiting for PostgreSQL database %s\n' "${database}" >&2
      return 1
    }
    sleep 1
  done
}

sample_resources() {
  local phase=$1
  local container_id
  container_id="$("${compose[@]}" ps -q postgres)"
  {
    printf '%s\t' "${phase}"
    docker stats --no-stream --format \
      '{{.CPUPerc}}\t{{.MemUsage}}\t{{.MemPerc}}\t{{.BlockIO}}\t{{.PIDs}}' \
      "${container_id}"
  } >>"${resource_report}"
}

require_command docker
docker info >/dev/null

printf 'phase\tcpu\tmemory\tmemory_percent\tblock_io\tpids\n' \
  >"${resource_report}"

log "project=${project} (isolated, no published ports)"
"${compose[@]}" up -d postgres
wait_for_bootstrap_then_ready

container_id="$("${compose[@]}" ps -q postgres)"
read -r memory_limit cpu_limit < <(
  docker inspect --format '{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}}' \
    "${container_id}"
)
[[ "${memory_limit}" == 1073741824 ]] || {
  printf 'expected 1GiB limit, got %s bytes\n' "${memory_limit}" >&2
  exit 1
}
[[ "${cpu_limit}" == 2000000000 ]] || {
  printf 'expected 2 CPU limit, got %s NanoCPUs\n' "${cpu_limit}" >&2
  exit 1
}
if docker port "${container_id}" 2>/dev/null | grep -q .; then
  printf 'bake-off container unexpectedly publishes a host port\n' >&2
  exit 1
fi
docker inspect --format '{{range .Config.Env}}{{println .}}{{end}}' \
  "${container_id}" | grep -Fxq 'PDB_TUNE=false'
sample_resources startup

log "loading schema and deterministic fixtures"
psql_file postgres "${bakeoff_dir}/00-schema.sql"
psql_file postgres "${bakeoff_dir}/10-fixtures.sql"
sample_resources indexed

log "running lexical, vector, ACL, recall, RRF, and rollback tests"
psql_file postgres "${bakeoff_dir}/20-tests.sql" |
  tee "${report_dir}/sql-tests.txt"
sample_resources tested

log "exporting only an explicitly authorized immutable version"
psql_command postgres --csv --tuples-only --command "
  COPY (
    SELECT chunk_id, tenant_id, document_version_id, content::text
    FROM phase15_bakeoff.lexical_jieba
    WHERE tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'::uuid
      AND document_version_id = ANY (
        ARRAY['aaaaaaaa-0000-0000-0000-000000000001'::uuid]
      )
    ORDER BY chunk_id
  ) TO STDOUT WITH (FORMAT csv, HEADER true)
" >"${report_dir}/authorized-export.csv"
[[ "$(wc -l <"${report_dir}/authorized-export.csv")" -eq 5 ]]
if grep -Fq 'bbbbbbbb-' "${report_dir}/authorized-export.csv"; then
  printf 'unauthorized tenant leaked into CSV export\n' >&2
  exit 1
fi

log "dumping and restoring into a database created from template0"
dump_file="${report_dir}/postgres.dump"
"${compose[@]}" exec -T postgres \
  pg_dump --username=postgres --dbname=postgres --format=custom \
  --schema=phase15_bakeoff --no-owner --no-acl >"${dump_file}"
[[ -s "${dump_file}" ]]
psql_command postgres --command \
  "CREATE DATABASE bakeoff_restored TEMPLATE template0;"
psql_command bakeoff_restored --command \
  "CREATE EXTENSION pg_search VERSION '0.24.2'; CREATE EXTENSION vector VERSION '0.8.2';"
cat "${dump_file}" | "${compose[@]}" exec -T postgres \
  pg_restore --username=postgres --dbname=bakeoff_restored \
  --no-owner --no-acl --exit-on-error
psql_file bakeoff_restored "${bakeoff_dir}/30-verify-recovery.sql" |
  tee "${report_dir}/restore-verify.txt"
sample_resources restored

log "testing graceful restart"
"${compose[@]}" restart postgres >/dev/null
wait_for_ready postgres
psql_file postgres "${bakeoff_dir}/30-verify-recovery.sql" |
  tee "${report_dir}/restart-verify.txt"
sample_resources restarted

log "testing SIGKILL crash recovery of the isolated container"
container_id="$("${compose[@]}" ps -q postgres)"
docker kill --signal=KILL "${container_id}" >/dev/null
"${compose[@]}" up -d postgres >/dev/null
wait_for_ready postgres
"${compose[@]}" logs --no-color postgres \
  >"${report_dir}/crash-recovery.log"
grep -Fq 'database system was interrupted' \
  "${report_dir}/crash-recovery.log" || {
  printf 'PostgreSQL did not report crash recovery after SIGKILL\n' >&2
  exit 1
}
psql_file postgres "${bakeoff_dir}/30-verify-recovery.sql" |
  tee "${report_dir}/crash-verify.txt"
sample_resources crash-recovered

psql_command postgres --tuples-only --no-align --command "
  SELECT 'postgres_size_bytes=' || pg_database_size(current_database())
  UNION ALL
  SELECT 'restored_size_bytes=' || pg_database_size('bakeoff_restored');
" >"${report_dir}/database-sizes.txt"

log "decisive output"
grep -h 'PASS' "${report_dir}"/*-verify.txt \
  "${report_dir}/sql-tests.txt" || true
cat "${resource_report}"
