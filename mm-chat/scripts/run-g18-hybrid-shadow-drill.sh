#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
hybrid_dir="$(cd -- "${script_dir}/../ops/g18-hybrid-shadow" && pwd)"
vector_dir="$(cd -- "${script_dir}/../ops/g18-pgvector-shadow" && pwd)"
restore_dir="$(cd -- "${script_dir}/../ops/g18-postgres17" && pwd)"
postgres_dir="$(cd -- "${script_dir}/../postgres" && pwd)"
compose_file="${restore_dir}/compose.yml"
project="mmchat-g18-hybrid-${UID}-$$"
report_dir="$(mktemp -d /tmp/mm-chat-g18-hybrid-shadow.XXXXXX)"
pg17_image_ref="mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5"
compose=(docker compose -p "${project}" -f "${compose_file}")

log() {
  printf '[g18-hybrid] %s\n' "$*"
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

log "loading and verifying the G18 authority/projection fixture"
psql_file "${restore_dir}/10-synthetic-fixture.sql" \
  | tee "${report_dir}/seed-base.txt"
psql_file "${restore_dir}/20-verify-restore.sql" \
  --set=expected_major=17 --set=expect_extensions=true \
  | tee "${report_dir}/verify-base.txt"

log "creating and filling the G18.3 pgvector shadow prerequisite"
psql_file "${vector_dir}/00-shadow-schema.up.sql" \
  | tee "${report_dir}/vector-schema-up.txt"
psql_file "${vector_dir}/10-shadow-fixture.sql" \
  | tee "${report_dir}/seed-vector.txt"
"${compose[@]}" exec -T pg17 psql -X --set=ON_ERROR_STOP=1 \
  --username=postgres --dbname=postgres \
  --command="SELECT * FROM knowledge_backfill_pgvector_shadow(
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );" | tee "${report_dir}/backfill-vector.txt"

log "creating the BM25/hybrid shadow schema and synthetic lexical fixture"
psql_file "${hybrid_dir}/00-shadow-schema.up.sql" \
  | tee "${report_dir}/hybrid-schema-up.txt"
psql_file "${hybrid_dir}/10-shadow-fixture.sql" \
  | tee "${report_dir}/seed-hybrid.txt"

log "proving BM25, Dense, RRF, Golden, deletion, and redacted diagnostics"
psql_file "${hybrid_dir}/20-verify-shadow.sql" \
  | tee "${report_dir}/verify-hybrid.txt"
grep -Fq 'idx_knowledge_child_bm25_shadow_text' \
  "${report_dir}/verify-hybrid.txt"
grep -Fq 'idx_knowledge_child_vector_shadow_hnsw' \
  "${report_dir}/verify-hybrid.txt"

log "rolling back G18.4 while retaining G18.3 and the legacy reader"
psql_file "${hybrid_dir}/00-shadow-schema.down.sql" \
  | tee "${report_dir}/hybrid-schema-down.txt"
psql_file "${hybrid_dir}/30-verify-rollback.sql" \
  --set=expect_vector_shadow=true \
  | tee "${report_dir}/verify-hybrid-rollback.txt"

log "rolling back the G18.3 prerequisite and retaining legacy REAL[] data"
psql_file "${vector_dir}/00-shadow-schema.down.sql" \
  | tee "${report_dir}/vector-schema-down.txt"
psql_file "${hybrid_dir}/30-verify-rollback.sql" \
  --set=expect_vector_shadow=false \
  | tee "${report_dir}/verify-final-rollback.txt"

log "proving the production migration manifest stayed unchanged"
run_migrate "${report_dir}/migrate-after-shadow.log"
grep -Fq 'no migrations changed' "${report_dir}/migrate-after-shadow.log"

log "decisive output"
grep -h 'PASS G18.4\|PASS PG17' "${report_dir}"/*.txt
