#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
pointer_dir="$(cd -- "${script_dir}/../ops/g18-profile-pointer" && pwd)"
restore_dir="$(cd -- "${script_dir}/../ops/g18-postgres17" && pwd)"
compose_file="${restore_dir}/compose.yml"
project="mmchat-g18-profile-${UID}-$$"
report_dir="$(mktemp -d /tmp/mm-chat-g18-profile-pointer.XXXXXX)"
compose=(docker compose -p "${project}" -f "${compose_file}")

log() {
  printf '[g18-profile] %s\n' "$*"
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
  until "${compose[@]}" exec -T pg16 \
      pg_isready --username=postgres \
        --dbname=mm_chat_g18_source >/dev/null 2>&1; do
    ((SECONDS < deadline)) || {
      printf 'timed out waiting for pg16 source\n' >&2
      return 1
    }
    sleep 1
  done
}

psql_file() {
  local file=$1
  shift
  "${compose[@]}" exec -T pg16 \
    psql -X --set=ON_ERROR_STOP=1 "$@" \
      --username=postgres --dbname=mm_chat_g18_source <"${file}"
}

psql_command() {
  local command=$1
  "${compose[@]}" exec -T pg16 \
    psql -X --set=ON_ERROR_STOP=1 \
      --username=postgres --dbname=mm_chat_g18_source \
      --command="${command}"
}

run_migrate() {
  local direction=$1
  local output=$2
  "${compose[@]}" run --rm --no-deps migrate16 \
    /usr/local/bin/mm-chat-migrate "${direction}" 2>&1 | tee "${output}"
}

command -v docker >/dev/null
docker info >/dev/null

log "project=${project} (PG16-compatible; internal network; synthetic data)"
log "building the current migration binary"
"${compose[@]}" build --pull=false migrate16

log "starting disposable PostgreSQL 16"
"${compose[@]}" up -d pg16
wait_for_ready

log "applying migrations through the legacy-default profile pointer"
run_migrate up "${report_dir}/migrate-up.txt"
grep -Fq 'up 037_rag_retrieval_profile_pointer' \
  "${report_dir}/migrate-up.txt"

log "loading synthetic authority/search rows and proving exact legacy parity"
psql_file "${restore_dir}/10-synthetic-fixture.sql" \
  | tee "${report_dir}/seed.txt"
psql_file "${restore_dir}/20-verify-restore.sql" \
  --set=expected_major=16 --set=expect_extensions=false \
  | tee "${report_dir}/verify-base.txt"
psql_file "${pointer_dir}/20-verify-pointer.sql" \
  | tee "${report_dir}/verify-pointer.txt"

log "proving rollback refuses a non-legacy active pointer"
psql_command "UPDATE knowledge_retrieval_profile_head
  SET active_profile = 'pg17_bm25_pgvector_v1'
  WHERE singleton_id = 1;" >/dev/null
set +e
"${compose[@]}" run --rm --no-deps migrate16 \
  /usr/local/bin/mm-chat-migrate down \
  >"${report_dir}/rollback-guard.txt" 2>&1
guard_status=$?
set -e
if ((guard_status == 0)); then
  printf 'non-legacy rollback unexpectedly succeeded\n' >&2
  exit 1
fi
grep -Fq 'RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY' \
  "${report_dir}/rollback-guard.txt"
psql_file "${pointer_dir}/25-verify-rollback-guard.sql" \
  | tee "${report_dir}/verify-rollback-guard.txt"

log "switching back through the controlled compare-and-swap function"
psql_command "SET ROLE rag_replay_operator;
  SELECT * FROM knowledge_set_retrieval_profile(
    'pg17_bm25_pgvector_v1',
    'legacy',
    1,
    'G18.5A controlled rollback before migration down'
  );
  RESET ROLE;" | tee "${report_dir}/switch-back.txt"

log "rolling back migration 037 while retaining the legacy reader"
run_migrate down "${report_dir}/migrate-down.txt"
grep -Fq 'down 037_rag_retrieval_profile_pointer' \
  "${report_dir}/migrate-down.txt"
psql_file "${pointer_dir}/30-verify-rollback.sql" \
  | tee "${report_dir}/verify-rollback.txt"

log "reapplying migration 037 and proving restart state is legacy"
run_migrate up "${report_dir}/migrate-reapply.txt"
grep -Fq 'up 037_rag_retrieval_profile_pointer' \
  "${report_dir}/migrate-reapply.txt"
psql_file "${pointer_dir}/20-verify-pointer.sql" \
  | tee "${report_dir}/verify-reapply.txt"
run_migrate up "${report_dir}/migrate-final.txt"
grep -Fq 'no migrations changed' "${report_dir}/migrate-final.txt"

log "decisive output"
grep -h 'PASS G18.5A\|PASS PG16' "${report_dir}"/*.txt
