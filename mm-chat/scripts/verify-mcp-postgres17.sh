#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-mcp-pg17-$RANDOM-$$"
database_name="neo_chat_mcp_drill"
database_user="postgres"
database_password="mcp-drill-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
trap cleanup EXIT INT TERM

log() {
  printf 'MCP PostgreSQL 17 drill: %s\n' "$*"
}

if ! command -v docker >/dev/null 2>&1 || ! docker version >/dev/null 2>&1; then
  echo "MCP PostgreSQL 17 drill: Docker is required" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1; then
  echo "MCP PostgreSQL 17 drill: openssl is required" >&2
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
  echo "MCP PostgreSQL 17 drill: database did not become ready" >&2
  exit 1
fi

host_port="$(docker port "${container_name}" 5432/tcp | awk -F: 'NR == 1 {print $NF}')"
if [[ ! "${host_port}" =~ ^[0-9]+$ ]]; then
  echo "MCP PostgreSQL 17 drill: failed to resolve the disposable port" >&2
  exit 1
fi
database_url="postgres://${database_user}:${database_password}@127.0.0.1:${host_port}/${database_name}?sslmode=disable"

psql_command() {
  docker exec -e "PGPASSWORD=${database_password}" "${container_name}" \
    psql --set=ON_ERROR_STOP=1 --no-psqlrc --tuples-only --no-align \
    --username="${database_user}" \
    --dbname="${database_name}" --command "$1"
}

log "checking PostgreSQL major"
server_major="$(psql_command "SHOW server_version_num" | cut -c1-2)"
if [[ "${server_major}" != "17" ]]; then
  echo "MCP PostgreSQL 17 drill: expected PostgreSQL 17, got ${server_major}" >&2
  exit 1
fi

log "building the current migration command"
(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/mm-chat-migrate" ./cmd/migrate)

run_migrate() {
  MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/mm-chat-migrate" "$@"
}

log "applying a fresh 001 -> 074 chain"
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 074_mcp_tools_foundation" "${work_dir}/fresh.log"

log "proving replay is a no-op"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"

log "rolling back only 074 and seeding retired metadata"
run_migrate down >"${work_dir}/down.log" 2>&1
grep -Fq "down 074_mcp_tools_foundation" "${work_dir}/down.log"
psql_command "
DO \$\$
BEGIN
  IF EXISTS (
    SELECT 1 FROM pg_catalog.pg_tables
    WHERE schemaname = 'public' AND tablename LIKE 'mcp_%'
  ) THEN
    RAISE EXCEPTION 'MCP tables remained after 074 down';
  END IF;
  IF to_regclass('public.plugin_registry') IS NULL THEN
    RAISE EXCEPTION 'plugin_registry must survive 074 down';
  END IF;
END
\$\$;
INSERT INTO users (id, email, display_name)
VALUES ('74000000-0000-4000-8000-000000000001', 'mcp-migration@example.test', 'MCP migration');
INSERT INTO conversations (id, user_id, title, metadata)
VALUES (
  '74000000-0000-4000-8000-000000000002',
  '74000000-0000-4000-8000-000000000001',
  'MCP migration',
  '{\"activePlugins\":[\"weather\"],\"safeField\":\"retained\"}'::jsonb
);
" >/dev/null

log "reapplying 074 and verifying schema, metadata, and retention"
run_migrate up >"${work_dir}/reup.log" 2>&1
grep -Fq "up 074_mcp_tools_foundation" "${work_dir}/reup.log"
psql_command "
DO \$\$
DECLARE
  mcp_table_count integer;
  metadata_value jsonb;
BEGIN
  SELECT count(*) INTO mcp_table_count
  FROM pg_catalog.pg_tables
  WHERE schemaname = 'public' AND tablename LIKE 'mcp_%';
  IF mcp_table_count <> 12 THEN
    RAISE EXCEPTION 'expected 12 MCP tables, got %', mcp_table_count;
  END IF;
  SELECT metadata INTO metadata_value
  FROM conversations
  WHERE id = '74000000-0000-4000-8000-000000000002';
  IF metadata_value ? 'activePlugins' OR metadata_value->>'safeField' <> 'retained' THEN
    RAISE EXCEPTION 'retired metadata cleanup was not selective: %', metadata_value;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = 'public' AND table_name = 'mcp_tool_calls'
      AND column_name = 'retained_until' AND is_nullable = 'NO'
  ) THEN
    RAISE EXCEPTION 'retained_until contract is missing';
  END IF;
  IF to_regclass('public.idx_mcp_tool_calls_retention') IS NULL THEN
    RAISE EXCEPTION 'MCP retention index is missing';
  END IF;
END
\$\$;
" >/dev/null

log "running PostgreSQL repository lifecycle coverage"
(cd "${backend_dir}" && MM_CHAT_TEST_DATABASE_URL="${database_url}" \
  go test -count=1 -run '^TestPostgresRepositoryLifecycleAndRetention$' ./internal/mcpclient)

log "proving a second replay remains a no-op"
run_migrate up >"${work_dir}/final-replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/final-replay.log"

log "passed (fresh, replay, down/up, metadata, retention, repository lifecycle)"
