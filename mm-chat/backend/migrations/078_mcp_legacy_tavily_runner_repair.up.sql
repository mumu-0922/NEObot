UPDATE mcp_servers AS server
SET endpoint_url = 'runner://marketplace-tavily-ai-tavily-mcp-0.2.19',
    transport = 'stdio',
    auth_type = 'env',
    auth_config = jsonb_set(
      jsonb_set(
        jsonb_set(
          server.auth_config,
          '{metadata,runnerArtifactId}',
          to_jsonb('marketplace-tavily-ai-tavily-mcp-0.2.19'::text),
          true
        ),
        '{metadata,marketplace,deploymentHash}',
        to_jsonb('4eb21572ab57f5f7ac2490758eb8c2714564f8d390fa8f6ccce22a348891c5bf'::text),
        true
      ),
      '{metadata,legacyInstallRepair}',
      to_jsonb('078'::text),
      true
    ),
    status = 'needs_auth',
    tool_snapshot = '[]'::jsonb,
    tool_snapshot_hash = NULL,
    validated_at = NULL,
    last_error_code = 'credential_required',
    updated_at = now()
WHERE server.deleted_at IS NULL
  AND server.transport = 'streamable_http'
  AND server.endpoint_url IN (
    'https://mcp.tavily.com/mcp',
    'https://mcp.tavily.com/mcp/'
  )
  AND server.auth_type = 'none'
  AND server.status = 'unavailable'
  AND server.last_error_code = 'connect_failed'
  AND server.auth_config #>> '{metadata,marketplace,provider}' = 'lobehub'
  AND server.auth_config #>> '{metadata,marketplace,identifier}' = 'tavily-ai-tavily-mcp'
  AND server.auth_config #>> '{metadata,marketplace,version}' = '0.2.19'
  AND server.auth_config #>> '{metadata,marketplace,deploymentHash}' =
    '70d1cd77529cffedb9624f4b3f62383e4aa43ba2bfba451c84d4b13e4a4e7d98'
  AND NOT EXISTS (
    SELECT 1
    FROM mcp_credentials AS credential
    WHERE credential.user_id = server.user_id
      AND credential.server_source = 'private'
      AND credential.server_ref = server.id::text
  );
