#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
cutover_sql="${script_dir}/cutover-legacy-skills.sql"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-legacy-cutover-pg17-$RANDOM-$$"
database_name="neo_chat_legacy_cutover_drill"
database_user="postgres"
database_password="legacy-cutover-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker stop "${container_name}" >/dev/null 2>&1 || true
  find "${work_dir}" -depth -mindepth 1 -delete
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM
log() { printf 'Agent legacy cutover PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker openssl sha256sum; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || { echo "Docker is required" >&2; exit 1; }
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

docker run -d --rm --name "${container_name}" \
  -e "POSTGRES_DB=${database_name}" \
  -e "POSTGRES_USER=${database_user}" \
  -e "POSTGRES_PASSWORD=${database_password}" \
  -p 127.0.0.1::5432 "${postgres_image}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" \
    >/dev/null 2>&1 && break
  sleep 1
done
docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null

refresh_database_url() {
  local database_port
  database_port="$(docker port "${container_name}" 5432/tcp | sed -n '1s/.*://p')"
  database_url="postgres://${database_user}:${database_password}@127.0.0.1:${database_port}/${database_name}?sslmode=disable"
}
refresh_database_url
psql_command() {
  docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
      --username="${database_user}" --dbname="${database_name}" --command "$1"
}
run_cutover() {
  docker exec -i -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --username="${database_user}" \
      --dbname="${database_name}" "$@" <"${cutover_sql}"
}

log "applying schema head 001 -> 093"
[[ "$(psql_command 'SHOW server_version_num' | cut -c1-2)" == "17" ]]
(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" up >"${work_dir}/migrate.log" 2>&1
grep -Fq "up 090_agent_product_shadow" "${work_dir}/migrate.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/migrate.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/migrate.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/migrate.log"
[[ "$(psql_command 'SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1')" == "93" ]]

log "seeding retired selection fixtures and taking a full backup"
user_id="11111111-1111-4111-8111-111111111111"
psql_command "
INSERT INTO users(id,display_name) VALUES('${user_id}','cutover');
INSERT INTO conversations(id,user_id,title,status,metadata,created_at,updated_at)
VALUES
  ('21111111-1111-4111-8111-111111111111','${user_id}','one','active',
   '{\"activeSkills\":[\"legacy-a\"],\"useReasoning\":true,\"nested\":{\"kept\":1}}',
   TIMESTAMPTZ '2026-01-01 00:00:00+00',TIMESTAMPTZ '2026-01-01 00:00:01+00'),
  ('31111111-1111-4111-8111-111111111111','${user_id}','two','archived',
   '{\"activeSkills\":[],\"useSearch\":true}',
   TIMESTAMPTZ '2026-01-02 00:00:00+00',TIMESTAMPTZ '2026-01-02 00:00:01+00'),
  ('41111111-1111-4111-8111-111111111111','${user_id}','three','active',
   '{\"knowledgeCollectionIds\":[\"kept\"]}',
   TIMESTAMPTZ '2026-01-03 00:00:00+00',TIMESTAMPTZ '2026-01-03 00:00:01+00');" >/dev/null
unrelated_before="$(psql_command "SELECT md5(string_agg((to_jsonb(c) || jsonb_build_object('metadata', c.metadata - 'activeSkills'))::text, '' ORDER BY c.id)) FROM conversations c")"
docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
  pg_dump --no-owner --no-privileges -U "${database_user}" -d "${database_name}" \
  >"${work_dir}/pre-cutover.sql"
[[ -s "${work_dir}/pre-cutover.sql" ]]
backup_fingerprint="sha256:$(sha256sum "${work_dir}/pre-cutover.sql" | awk '{print $1}')"

log "proving default dry-run and rejected guards are non-destructive"
run_cutover >"${work_dir}/dry-run.log"
grep -Fq "dry-run" "${work_dir}/dry-run.log"
[[ "$(psql_command "SELECT count(*) FROM conversations WHERE metadata ? 'activeSkills'")" == "2" ]]

set +e
run_cutover --variable=cutover_apply=true --variable=expected_count=1 \
  --variable=backup_fingerprint="${backup_fingerprint}" >"${work_dir}/count-reject.log" 2>&1
count_status=$?
run_cutover --variable=cutover_apply=true --variable=expected_count=2 \
  --variable=backup_fingerprint=invalid >"${work_dir}/fingerprint-reject.log" 2>&1
fingerprint_status=$?
set -e
[[ "${count_status}" -ne 0 ]]
[[ "${fingerprint_status}" -ne 0 ]]
grep -Fq "LEGACY_SKILL_EXPECTED_COUNT_MISMATCH" "${work_dir}/count-reject.log"
grep -Fq "LEGACY_SKILL_BACKUP_FINGERPRINT_REQUIRED" "${work_dir}/fingerprint-reject.log"
[[ "$(psql_command "SELECT count(*) FROM conversations WHERE metadata ? 'activeSkills'")" == "2" ]]

log "applying the exact one-key cutover and proving idempotence"
run_cutover --variable=cutover_apply=true --variable=expected_count=2 \
  --variable=backup_fingerprint="${backup_fingerprint}" >"${work_dir}/apply.log"
grep -Fq "removed_legacy_skill_selections" "${work_dir}/apply.log"
[[ "$(psql_command "SELECT count(*) FROM conversations WHERE metadata ? 'activeSkills'")" == "0" ]]
unrelated_after="$(psql_command "SELECT md5(string_agg((to_jsonb(c) || jsonb_build_object('metadata', c.metadata - 'activeSkills'))::text, '' ORDER BY c.id)) FROM conversations c")"
[[ "${unrelated_before}" == "${unrelated_after}" ]]
[[ "$(psql_command "SELECT metadata->>'useReasoning' FROM conversations WHERE title='one'")" == "true" ]]
[[ "$(psql_command "SELECT metadata->'nested'->>'kept' FROM conversations WHERE title='one'")" == "1" ]]
[[ "$(psql_command "SELECT metadata->>'useSearch' FROM conversations WHERE title='two'")" == "true" ]]
[[ "$(psql_command "SELECT metadata->'knowledgeCollectionIds'->>0 FROM conversations WHERE title='three'")" == "kept" ]]

docker restart "${container_name}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" \
    >/dev/null 2>&1 && break
  sleep 1
done
refresh_database_url
[[ "$(psql_command "SELECT count(*) FROM conversations WHERE metadata ? 'activeSkills'")" == "0" ]]

run_cutover --variable=cutover_apply=true --variable=expected_count=0 \
  --variable=backup_fingerprint="${backup_fingerprint}" >"${work_dir}/replay.log"
grep -Fq "removed_legacy_skill_selections" "${work_dir}/replay.log"
[[ "$(psql_command 'SELECT count(*) FROM conversations')" == "3" ]]
[[ "$(psql_command 'SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1')" == "93" ]]

if ! MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-093-tail-1.log" 2>&1; then
  cat "${work_dir}/peel-093-tail-1.log" >&2
  exit 1
fi
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/peel-093-tail-1.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-092.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/peel-092.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" up >"${work_dir}/reup-092.log" 2>&1
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/reup-092.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/reup-092.log"
[[ "$(psql_command 'SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1')" == "93" ]]

log "passed (dry-run, backup/count fences, exact JSONB deletion, unrelated-byte equivalence, restart/replay, schema head 093)"
