#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
source "${script_dir}/migration-drill-tail.sh"
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

log "applying 001 -> 097 with the 098/099/100 tail deferred"
psql_command "$(migration_drill_deferred_tail_sql "${backend_dir}" \
  098_retire_legacy_agent_control_plane \
  099_chat_agent_event_log_function_repair \
  100_chat_agent_approvals \
  101_chat_agent_transcript_blocks)" >/dev/null
run_migrate up >"${work_dir}/fresh.log" 2>&1
grep -Fq "up 074_mcp_tools_foundation" "${work_dir}/fresh.log"
grep -Fq "up 075_mcp_runtime_role_grants" "${work_dir}/fresh.log"
grep -Fq "up 076_mcp_private_runner_artifacts" "${work_dir}/fresh.log"
grep -Fq "up 077_mcp_marketplace_install_credentials" "${work_dir}/fresh.log"
grep -Fq "up 078_mcp_legacy_tavily_runner_repair" "${work_dir}/fresh.log"
grep -Fq "up 079_mcp_tavily_credential_revalidation" "${work_dir}/fresh.log"
grep -Fq "up 080_mcp_legacy_deepwiki_icon" "${work_dir}/fresh.log"
grep -Fq "up 081_mcp_legacy_context7_artifact_rebind" "${work_dir}/fresh.log"
grep -Fq "up 082_assistant_library" "${work_dir}/fresh.log"
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
grep -Fq "up 097_chat_agent_goals" "${work_dir}/fresh.log"

log "proving replay is a no-op"
run_migrate up >"${work_dir}/replay.log" 2>&1
grep -Fq "no migrations changed" "${work_dir}/replay.log"
psql_command "DELETE FROM schema_migrations WHERE version IN (98,99,100)" >/dev/null

log "rolling back the clean 097 through 077 tails before the 076 guard drill"
run_migrate down >"${work_dir}/peel-097-chat-agent-goal-tail.log" 2>&1
grep -Fq "down 097_chat_agent_goals" "${work_dir}/peel-097-chat-agent-goal-tail.log"
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
run_migrate down >"${work_dir}/down-083.log" 2>&1
grep -Fq "down 083_skill_supply_chain" "${work_dir}/down-083.log"
run_migrate down >"${work_dir}/down-082.log" 2>&1
grep -Fq "down 082_assistant_library" "${work_dir}/down-082.log"
run_migrate down >"${work_dir}/down-081.log" 2>&1
grep -Fq "down 081_mcp_legacy_context7_artifact_rebind" "${work_dir}/down-081.log"
run_migrate down >"${work_dir}/down-080.log" 2>&1
grep -Fq "down 080_mcp_legacy_deepwiki_icon" "${work_dir}/down-080.log"
run_migrate down >"${work_dir}/down-079.log" 2>&1
grep -Fq "down 079_mcp_tavily_credential_revalidation" "${work_dir}/down-079.log"
run_migrate down >"${work_dir}/down-078.log" 2>&1
grep -Fq "down 078_mcp_legacy_tavily_runner_repair" "${work_dir}/down-078.log"
run_migrate down >"${work_dir}/down-077.log" 2>&1
grep -Fq "down 077_mcp_marketplace_install_credentials" "${work_dir}/down-077.log"

log "proving 076 accepts approved Runner references and guards destructive down"
psql_command "
INSERT INTO users (id, email, display_name)
VALUES ('76000000-0000-4000-8000-000000000001', 'mcp-runner-migration@example.test', 'MCP Runner migration');
INSERT INTO mcp_servers (
  id, user_id, name, endpoint_url, transport, auth_type
) VALUES (
  '76000000-0000-4000-8000-000000000002',
  '76000000-0000-4000-8000-000000000001',
  'Context7',
  'runner://marketplace-upstash-context7-2.2.0',
  'stdio',
  'none'
);
DO \$\$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'mcp_servers'::regclass
      AND conname = 'mcp_servers_stdio_endpoint_check'
  ) THEN
    RAISE EXCEPTION '076 stdio endpoint constraint is missing';
  END IF;
END
\$\$;
" >/dev/null
if run_migrate down >"${work_dir}/guarded-down-076.log" 2>&1; then
  echo "MCP PostgreSQL 17 drill: 076 down unexpectedly accepted a stdio row" >&2
  exit 1
fi
grep -Fq "cannot roll back 076_mcp_private_runner_artifacts while stdio MCP servers exist" \
  "${work_dir}/guarded-down-076.log"
psql_command "
DO \$\$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM schema_migrations WHERE version = 76
  ) OR NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conrelid = 'mcp_servers'::regclass
      AND conname = 'mcp_servers_stdio_endpoint_check'
  ) THEN
    RAISE EXCEPTION 'failed 076 down changed migration or constraint state';
  END IF;
END
\$\$;
" >/dev/null

log "removing the fixture and rolling back 076"
psql_command "
DELETE FROM mcp_servers WHERE id = '76000000-0000-4000-8000-000000000002';
DELETE FROM users WHERE id = '76000000-0000-4000-8000-000000000001';
" >/dev/null
run_migrate down >"${work_dir}/down-076.log" 2>&1
grep -Fq "down 076_mcp_private_runner_artifacts" "${work_dir}/down-076.log"

log "rolling back 075 and proving runtime grants are removed"
run_migrate down >"${work_dir}/down-075.log" 2>&1
grep -Fq "down 075_mcp_runtime_role_grants" "${work_dir}/down-075.log"
psql_command "
DO \$\$
BEGIN
  IF has_table_privilege('go_api_runtime', 'mcp_artifact_cleanup_queue', 'SELECT') THEN
    RAISE EXCEPTION '075 down retained MCP runtime table privileges';
  END IF;
END
\$\$;
" >/dev/null

log "rolling back 074 and seeding retired metadata"
run_migrate down >"${work_dir}/down-074.log" 2>&1
grep -Fq "down 074_mcp_tools_foundation" "${work_dir}/down-074.log"
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

log "reapplying 074 -> 100 and verifying schema, metadata, retention, grants, and stdio persistence"
run_migrate up >"${work_dir}/reup.log" 2>&1
grep -Fq "up 074_mcp_tools_foundation" "${work_dir}/reup.log"
grep -Fq "up 075_mcp_runtime_role_grants" "${work_dir}/reup.log"
grep -Fq "up 076_mcp_private_runner_artifacts" "${work_dir}/reup.log"
grep -Fq "up 077_mcp_marketplace_install_credentials" "${work_dir}/reup.log"
grep -Fq "up 078_mcp_legacy_tavily_runner_repair" "${work_dir}/reup.log"
grep -Fq "up 079_mcp_tavily_credential_revalidation" "${work_dir}/reup.log"
grep -Fq "up 080_mcp_legacy_deepwiki_icon" "${work_dir}/reup.log"
grep -Fq "up 081_mcp_legacy_context7_artifact_rebind" "${work_dir}/reup.log"
grep -Fq "up 082_assistant_library" "${work_dir}/reup.log"
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
grep -Fq "up 097_chat_agent_goals" "${work_dir}/reup.log"
grep -Fq "up 098_retire_legacy_agent_control_plane" "${work_dir}/reup.log"
grep -Fq "up 099_chat_agent_event_log_function_repair" "${work_dir}/reup.log"
grep -Fq "up 100_chat_agent_approvals" "${work_dir}/reup.log"
grep -Fq "up 101_chat_agent_transcript_blocks" "${work_dir}/reup.log"
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
  IF NOT has_table_privilege(
    'go_api_runtime', 'mcp_artifact_cleanup_queue', 'SELECT,DELETE'
  ) THEN
    RAISE EXCEPTION 'MCP cleanup runtime privileges are missing';
  END IF;
  IF NOT has_table_privilege(
    'go_api_runtime', 'mcp_servers', 'SELECT,INSERT,UPDATE,DELETE'
  ) THEN
    RAISE EXCEPTION 'MCP repository runtime privileges are missing';
  END IF;
  IF has_function_privilege(
    'memory_worker_runtime', 'mcp_enqueue_account_artifacts()', 'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'MCP account cleanup trigger remains public executable';
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

log "passed (historical 097 boundary, replay to head 101, guarded 076 down/up, metadata, retention, runtime grants, stdio repository lifecycle)"
