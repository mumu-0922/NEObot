#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
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

(cd "${project_dir}/backend" && go build -trimpath -o "${work_dir}/migrate" ./cmd/migrate)
MIGRATION_DATABASE_URL="${database_url}" "${work_dir}/migrate" up >"${work_dir}/up.log" 2>&1
grep -Fq "up 077_mcp_marketplace_install_credentials" "${work_dir}/up.log"
grep -Fq "up 078_mcp_legacy_tavily_runner_repair" "${work_dir}/up.log"
grep -Fq "up 079_mcp_tavily_credential_revalidation" "${work_dir}/up.log"
grep -Fq "up 080_mcp_legacy_deepwiki_icon" "${work_dir}/up.log"
grep -Fq "up 081_mcp_legacy_context7_artifact_rebind" "${work_dir}/up.log"
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

printf 'MCP credential migration drill: passed (077-081 up/down/up and exact repairs)\n'
