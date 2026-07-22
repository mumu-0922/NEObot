#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
shadow_dir="$(cd -- "${script_dir}/../ops/g18-pgvector-shadow" && pwd)"
restore_dir="$(cd -- "${script_dir}/../ops/g18-postgres17" && pwd)"
postgres_dir="$(cd -- "${script_dir}/../postgres" && pwd)"
compose_file="${restore_dir}/compose.yml"
project="mmchat-g18-vector-${UID}-$$"
report_dir="$(mktemp -d /tmp/mm-chat-g18-pgvector-shadow.XXXXXX)"
pg17_image_ref="mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5"
compose=(docker compose -p "${project}" -f "${compose_file}")

log() {
  printf '[g18-vector] %s\n' "$*"
}

cleanup() {
  local status=$?
  local leaked=false
  trap - EXIT INT TERM
  set +e
  if ((status != 0)); then
    "${compose[@]}" logs --no-color >"${report_dir}/compose.log" 2>&1
  fi
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1
  if [[ -n "$(docker ps -aq --filter "label=com.docker.compose.project=${project}")" ]] \
    || [[ -n "$(docker volume ls -q --filter "label=com.docker.compose.project=${project}")" ]]; then
    leaked=true
    status=1
  fi
  if ((status == 0)) && [[ "${leaked}" == false ]]; then
    log "PASS reports=${report_dir} disposable_database=removed"
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
  local deadline=$((SECONDS + 180))
  until "${compose[@]}" exec -T pg17 \
      pg_isready --username=postgres --dbname=postgres >/dev/null 2>&1; do
    ((SECONDS < deadline)) || {
      printf 'timed out waiting for pg17/postgres\n' >&2
      return 1
    }
    sleep 1
  done
}

psql_file() {
  local file=$1
  shift
  "${compose[@]}" exec -T pg17 \
    psql -X --set=ON_ERROR_STOP=1 "$@" \
      --username=postgres --dbname=postgres <"${file}"
}

run_migrate() {
  local output=$1
  "${compose[@]}" run --rm --no-deps \
    -e 'MIGRATION_DATABASE_URL=postgres://postgres@pg17:5432/postgres?sslmode=disable' \
    migrate17 2>&1 | tee "${output}"
}

require_command docker
docker info >/dev/null

log "project=${project} (PG17-only; internal network; synthetic data)"
log "building pinned database image and current migration CLI"
docker build --pull=false --tag "${pg17_image_ref}" "${postgres_dir}"
"${compose[@]}" build --pull=false migrate17

log "starting disposable PostgreSQL 17"
"${compose[@]}" up -d pg17
wait_for_ready

log "applying the current 37 production migrations"
run_migrate "${report_dir}/migrate-before-shadow.log"
grep -Fq 'up 037_rag_retrieval_profile_pointer' \
  "${report_dir}/migrate-before-shadow.log"

log "loading the G18.2 authority/projection base fixture"
psql_file "${restore_dir}/10-synthetic-fixture.sql" \
  | tee "${report_dir}/seed-base.txt"
psql_file "${restore_dir}/20-verify-restore.sql" \
  --set=expected_major=17 --set=expect_extensions=true \
  | tee "${report_dir}/verify-base.txt"

log "creating the pgvector shadow schema"
psql_file "${shadow_dir}/00-shadow-schema.up.sql" \
  | tee "${report_dir}/schema-up.txt"

log "loading compatible Jina REAL[] vectors without provider calls"
psql_file "${shadow_dir}/10-shadow-fixture.sql" \
  | tee "${report_dir}/seed-shadow.txt"

log "proving backfill, exact/HNSW parity, ACL, deletion, and rejection gates"
psql_file "${shadow_dir}/20-verify-shadow.sql" \
  | tee "${report_dir}/verify-shadow.txt"
grep -Fq 'Index Scan using idx_knowledge_child_vector_shadow_hnsw' \
  "${report_dir}/verify-shadow.txt"

log "rolling back only the shadow schema"
psql_file "${shadow_dir}/00-shadow-schema.down.sql" \
  | tee "${report_dir}/schema-down.txt"
psql_file "${shadow_dir}/30-verify-rollback.sql" \
  | tee "${report_dir}/verify-rollback.txt"

log "proving the production migration manifest stayed unchanged"
run_migrate "${report_dir}/migrate-after-shadow.log"
grep -Fq 'no migrations changed' "${report_dir}/migrate-after-shadow.log"

log "decisive output"
grep -h 'PASS G18.3\|PASS PG17' "${report_dir}"/*.txt
