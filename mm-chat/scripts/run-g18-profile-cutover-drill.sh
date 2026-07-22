#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cutover_dir="$(cd -- "${script_dir}/../ops/g18-profile-cutover" && pwd)"
hybrid_dir="$(cd -- "${script_dir}/../ops/g18-hybrid-shadow" && pwd)"
vector_dir="$(cd -- "${script_dir}/../ops/g18-pgvector-shadow" && pwd)"
restore_dir="$(cd -- "${script_dir}/../ops/g18-postgres17" && pwd)"
postgres_dir="$(cd -- "${script_dir}/../postgres" && pwd)"
compose_file="${restore_dir}/compose.yml"
project="mmchat-g18-cutover-${UID}-$$"
report_dir="$(mktemp -d /tmp/mm-chat-g18-profile-cutover.XXXXXX)"
pg17_image_ref="mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5"
compose=(docker compose -p "${project}" -f "${compose_file}")

log() {
  printf '[g18-cutover] %s\n' "$*"
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

psql_command() {
  local command=$1
  "${compose[@]}" exec -T pg17 \
    psql -X --set=ON_ERROR_STOP=1 \
      --username=postgres --dbname=postgres --command="${command}"
}

run_migrate() {
  local output=$1
  "${compose[@]}" run --rm --no-deps \
    -e 'MIGRATION_DATABASE_URL=postgres://postgres@pg17:5432/postgres?sslmode=disable' \
    migrate17 2>&1 | tee "${output}"
}

command -v docker >/dev/null
docker info >/dev/null

log "project=${project} (PG17-only; internal network; synthetic data)"
log "building the pinned PostgreSQL image and migration CLI"
docker build --pull=false --tag "${pg17_image_ref}" "${postgres_dir}"
"${compose[@]}" build --pull=false migrate17

log "starting disposable PostgreSQL 17"
"${compose[@]}" up -d pg17
wait_for_ready

log "applying the PG16-compatible production migrations through 037"
run_migrate "${report_dir}/migrate-before-cutover.log"
grep -Fq 'up 037_rag_retrieval_profile_pointer' \
  "${report_dir}/migrate-before-cutover.log"

log "loading the synthetic authority and reviewed G18 projections"
psql_file "${restore_dir}/10-synthetic-fixture.sql" \
  | tee "${report_dir}/seed-base.txt"
psql_file "${restore_dir}/20-verify-restore.sql" \
  --set=expected_major=17 --set=expect_extensions=true \
  | tee "${report_dir}/verify-base.txt"
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
psql_file "${hybrid_dir}/00-shadow-schema.up.sql" \
  | tee "${report_dir}/hybrid-schema-up.txt"
psql_file "${hybrid_dir}/10-shadow-fixture.sql" \
  | tee "${report_dir}/seed-hybrid.txt"

log "installing the candidate PG17 profile router and proving activation"
psql_file "${cutover_dir}/00-profile-router.up.sql" \
  | tee "${report_dir}/router-up.txt"
psql_file "${cutover_dir}/10-active-projection-maintenance.up.sql" \
  | tee "${report_dir}/maintenance-up.txt"
psql_file "${cutover_dir}/20-verify-activation.sql" \
  | tee "${report_dir}/verify-activation.txt"

log "publishing two heads concurrently through active profile maintenance"
psql_file "${cutover_dir}/40-concurrent-publish-fixture.sql" \
  | tee "${report_dir}/seed-concurrent-publish.txt"
psql_command "INSERT INTO knowledge_document_projection_heads(
  index_generation_id, document_id, active_materialization_id,
  last_corpus_projection_revision
) VALUES (
  '18180000-0000-0000-0000-000000000007',
  '18400000-0000-0000-0000-000000000002',
  '18400000-0000-0000-0000-000000000004',
  3
);" >"${report_dir}/publish-alpha.txt" 2>&1 &
alpha_pid=$!
psql_command "INSERT INTO knowledge_document_projection_heads(
  index_generation_id, document_id, active_materialization_id,
  last_corpus_projection_revision
) VALUES (
  '18180000-0000-0000-0000-000000000007',
  '18400000-0000-0000-0000-000000000012',
  '18400000-0000-0000-0000-000000000014',
  4
);" >"${report_dir}/publish-beta.txt" 2>&1 &
beta_pid=$!
wait "${alpha_pid}"
wait "${beta_pid}"
psql_file "${cutover_dir}/45-verify-concurrent-publish.sql" \
  | tee "${report_dir}/verify-concurrent-publish.txt"

log "restarting PostgreSQL and proving the durable active profile"
"${compose[@]}" restart pg17 >/dev/null
wait_for_ready
psql_file "${cutover_dir}/25-verify-restart.sql" \
  | tee "${report_dir}/verify-restart.txt"

log "proving router rollback refuses an active PG17 profile"
set +e
psql_file "${cutover_dir}/10-active-projection-maintenance.down.sql" \
  >"${report_dir}/maintenance-rollback-guard.txt" 2>&1
maintenance_guard_status=$?
set -e
if ((maintenance_guard_status == 0)); then
  printf 'active-profile maintenance rollback unexpectedly succeeded\n' >&2
  exit 1
fi
grep -Fq 'RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY' \
  "${report_dir}/maintenance-rollback-guard.txt"

set +e
psql_file "${cutover_dir}/00-profile-router.down.sql" \
  >"${report_dir}/rollback-guard.txt" 2>&1
guard_status=$?
set -e
if ((guard_status == 0)); then
  printf 'active-profile router rollback unexpectedly succeeded\n' >&2
  exit 1
fi
grep -Fq 'RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY' \
  "${report_dir}/rollback-guard.txt"

log "switching to legacy and rolling back the candidate layers"
psql_file "${cutover_dir}/27-switch-legacy.sql" \
  | tee "${report_dir}/switch-legacy.txt"
psql_file "${cutover_dir}/10-active-projection-maintenance.down.sql" \
  | tee "${report_dir}/maintenance-down.txt"
psql_file "${cutover_dir}/00-profile-router.down.sql" \
  | tee "${report_dir}/router-down.txt"
psql_file "${hybrid_dir}/00-shadow-schema.down.sql" \
  | tee "${report_dir}/hybrid-schema-down.txt"
psql_file "${vector_dir}/00-shadow-schema.down.sql" \
  | tee "${report_dir}/vector-schema-down.txt"
psql_file "${cutover_dir}/30-verify-rollback.sql" \
  | tee "${report_dir}/verify-rollback.txt"

log "proving the embedded production manifest remains 1-37"
run_migrate "${report_dir}/migrate-after-cutover.log"
grep -Fq 'no migrations changed' "${report_dir}/migrate-after-cutover.log"

log "decisive output"
grep -h 'PASS G18.5B.1\|PASS G18.5B.2a\|PASS PG17' \
  "${report_dir}"/*.txt
