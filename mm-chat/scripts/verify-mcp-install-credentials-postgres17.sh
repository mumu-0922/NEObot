#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
source "${script_dir}/migration-drill-tail.sh"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-mcp-credentials-pg17-$RANDOM-$$"
database_password="mcp-credentials-$(openssl rand -hex 16)"
work_dir="$(mktemp -d)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  rm -rf -- "${work_dir}"
}
trap cleanup EXIT INT TERM

docker run -d --rm --name "${container_name}" \
  -e POSTGRES_DB=neo_chat_mcp_credentials \
  -e POSTGRES_USER=postgres \
  -e "POSTGRES_PASSWORD=${database_password}" \
  -p 127.0.0.1::5432 "${postgres_image}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${container_name}" pg_isready -U postgres -d neo_chat_mcp_credentials >/dev/null 2>&1 && break
  sleep 1
done
port="$(docker port "${container_name}" 5432/tcp | awk -F: 'NR == 1 {print $NF}')"
database_url="postgres://postgres:${database_password}@127.0.0.1:${port}/neo_chat_mcp_credentials?sslmode=disable"
psql_command() {
  docker exec "${container_name}" psql --set=ON_ERROR_STOP=1 --no-psqlrc \
    --tuples-only --no-align --username=postgres \
    --dbname=neo_chat_mcp_credentials --command "$1"
}

(cd "${backend_dir}" && go build -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
psql_command "$(migration_drill_deferred_tail_sql "${backend_dir}" \
  098_retire_legacy_agent_control_plane \
  099_chat_agent_event_log_function_repair)" >/dev/null
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" up >"${work_dir}/up.log" 2>&1
grep -Fq "up 077_mcp_marketplace_install_credentials" "${work_dir}/up.log"
grep -Fq "up 078_mcp_legacy_tavily_runner_repair" "${work_dir}/up.log"
grep -Fq "up 079_mcp_tavily_credential_revalidation" "${work_dir}/up.log"
grep -Fq "up 080_mcp_legacy_deepwiki_icon" "${work_dir}/up.log"
grep -Fq "up 081_mcp_legacy_context7_artifact_rebind" "${work_dir}/up.log"
grep -Fq "up 082_assistant_library" "${work_dir}/up.log"
grep -Fq "up 083_skill_supply_chain" "${work_dir}/up.log"
grep -Fq "up 084_agent_orchestrator_foundation" "${work_dir}/up.log"
grep -Fq "up 085_agent_runner_foundation" "${work_dir}/up.log"
grep -Fq "up 086_agent_broker_foundation" "${work_dir}/up.log"
grep -Fq "up 087_agent_child_delegation" "${work_dir}/up.log"
grep -Fq "up 088_agent_cron_foundation" "${work_dir}/up.log"
grep -Fq "up 089_agent_draft_learning" "${work_dir}/up.log"
grep -Fq "up 090_agent_product_shadow" "${work_dir}/up.log"
grep -Fq "up 091_agent_artifact_publication" "${work_dir}/up.log"
grep -Fq "up 092_agent_project_mutation_canary" "${work_dir}/up.log"
grep -Fq "up 093_agent_child_canary_reap_transport" "${work_dir}/up.log"
grep -Fq "up 094_agent_cron_learning_activation" "${work_dir}/up.log"
grep -Fq "up 095_agent_product_canary_activation" "${work_dir}/up.log"
grep -Fq "up 096_chat_agent_event_log" "${work_dir}/up.log"
grep -Fq "up 097_chat_agent_goals" "${work_dir}/up.log"
psql_command "DELETE FROM schema_migrations WHERE version IN (98,99)" >/dev/null
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-097-tail-1.log" 2>&1
grep -Fq "down 097_chat_agent_goals" "${work_dir}/peel-097-tail-1.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-096-tail-1.log" 2>&1
grep -Fq "down 096_chat_agent_event_log" "${work_dir}/peel-096-tail-1.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-095-tail-1.log" 2>&1
grep -Fq "down 095_agent_product_canary_activation" "${work_dir}/peel-095-tail-1.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-094-tail-1.log" 2>&1
grep -Fq "down 094_agent_cron_learning_activation" "${work_dir}/peel-094-tail-1.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-093-tail-1.log" 2>&1
grep -Fq "down 093_agent_child_canary_reap_transport" "${work_dir}/peel-093-tail-1.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-092-tail-1.log" 2>&1
grep -Fq "down 092_agent_project_mutation_canary" "${work_dir}/peel-092-tail-1.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/peel-091-tail-1.log" 2>&1
grep -Fq "down 091_agent_artifact_publication" "${work_dir}/peel-091-tail-1.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-090.log" 2>&1
grep -Fq "down 090_agent_product_shadow" "${work_dir}/down-090.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-089.log" 2>&1
grep -Fq "down 089_agent_draft_learning" "${work_dir}/down-089.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-088.log" 2>&1
grep -Fq "down 088_agent_cron_foundation" "${work_dir}/down-088.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-087.log" 2>&1
grep -Fq "down 087_agent_child_delegation" "${work_dir}/down-087.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-086.log" 2>&1
grep -Fq "down 086_agent_broker_foundation" "${work_dir}/down-086.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-085.log" 2>&1
grep -Fq "down 085_agent_runner_foundation" "${work_dir}/down-085.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-084.log" 2>&1
grep -Fq "down 084_agent_orchestrator_foundation" "${work_dir}/down-084.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-083.log" 2>&1
grep -Fq "down 083_skill_supply_chain" "${work_dir}/down-083.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-082.log" 2>&1
grep -Fq "down 082_assistant_library" "${work_dir}/down-082.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-081.log" 2>&1
grep -Fq "down 081_mcp_legacy_context7_artifact_rebind" "${work_dir}/down-081.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-080.log" 2>&1
grep -Fq "down 080_mcp_legacy_deepwiki_icon" "${work_dir}/down-080.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-079.log" 2>&1
grep -Fq "down 079_mcp_tavily_credential_revalidation" "${work_dir}/down-079.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-078.log" 2>&1
grep -Fq "down 078_mcp_legacy_tavily_runner_repair" "${work_dir}/down-078.log"
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" down >"${work_dir}/down-077.log" 2>&1
grep -Fq "down 077_mcp_marketplace_install_credentials" "${work_dir}/down-077.log"

docker exec -i "${container_name}" psql -U postgres -d neo_chat_mcp_credentials \
  -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
INSERT INTO users (id, email, display_name)
VALUES ('78000000-0000-4000-8000-000000000001', 'mcp-tavily-repair@example.test', 'MCP Tavily repair');
INSERT INTO mcp_servers (
  id, user_id, name, endpoint_url, transport, auth_type, auth_config,
  status, last_error_code
) VALUES (
  '78000000-0000-4000-8000-000000000002',
  '78000000-0000-4000-8000-000000000001',
  'Legacy Tavily',
  'https://mcp.tavily.com/mcp',
  'streamable_http',
  'none',
  '{"metadata":{"marketplace":{"provider":"lobehub","identifier":"tavily-ai-tavily-mcp","version":"0.2.19","deploymentHash":"70d1cd77529cffedb9624f4b3f62383e4aa43ba2bfba451c84d4b13e4a4e7d98"}}}'::jsonb,
  'unavailable',
  'connect_failed'
), (
  '78000000-0000-4000-8000-000000000003',
  '78000000-0000-4000-8000-000000000001',
  'Legacy DeepWiki',
  'https://mcp.deepwiki.com/mcp',
  'streamable_http',
  'none',
  '{"metadata":{}}'::jsonb,
  'ready',
  NULL
), (
  '78000000-0000-4000-8000-000000000004',
  '78000000-0000-4000-8000-000000000001',
  'Legacy Context7',
  'runner://marketplace-upstash-context7-2.2.0',
  'stdio',
  'none',
  '{"metadata":{"runnerArtifactId":"marketplace-upstash-context7-2.2.0","marketplace":{"provider":"lobehub","identifier":"upstash-context7","version":"2.2.0","deploymentHash":"20a578fff586f03151f2f2f6aa97ea331d3985f8e93ce068f0f59e984cc4964d"}}}'::jsonb,
  'ready',
  NULL
);
SQL

MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" up >"${work_dir}/reup.log" 2>&1
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
repaired="$(
  docker exec "${container_name}" psql -U postgres -d neo_chat_mcp_credentials -Atc \
    "SELECT concat_ws('|', transport, auth_type, status, last_error_code, auth_config #>> '{metadata,runnerArtifactId}') FROM mcp_servers WHERE id = '78000000-0000-4000-8000-000000000002'"
)"
if [[ "${repaired}" != "stdio|env|needs_auth|credential_required|marketplace-tavily-ai-tavily-mcp-0.2.19" ]]; then
  echo "MCP credential migration drill: unexpected Tavily repair state: ${repaired}" >&2
  exit 1
fi

deepwiki="$(
  docker exec "${container_name}" psql -U postgres -d neo_chat_mcp_credentials -Atc \
    "SELECT concat_ws('|', auth_config #>> '{metadata,icon}', auth_config #>> '{metadata,legacyIconRepair}') FROM mcp_servers WHERE id = '78000000-0000-4000-8000-000000000003'"
)"
if [[ "${deepwiki}" != "https://deepwiki.com/favicon.ico|080" ]]; then
  echo "MCP credential migration drill: unexpected DeepWiki icon repair: ${deepwiki}" >&2
  exit 1
fi

context7="$(
  docker exec "${container_name}" psql -U postgres -d neo_chat_mcp_credentials -Atc \
    "SELECT concat_ws('|', auth_config #>> '{metadata,marketplace,deploymentHash}', auth_config #>> '{metadata,legacyArtifactRepair}') FROM mcp_servers WHERE id = '78000000-0000-4000-8000-000000000004'"
)"
if [[ "${context7}" != "22b235834a14b617480cc92dd0f6f6c7587cb399880c666773135971767bc6e2|081" ]]; then
  echo "MCP credential migration drill: unexpected Context7 artifact repair: ${context7}" >&2
  exit 1
fi

printf 'MCP credential migration drill: passed (historical 097 boundary, replay to head 099, and exact 078-081 repairs)\n'
