DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM mcp_servers AS server
    JOIN mcp_credentials AS credential
      ON credential.user_id = server.user_id
     AND credential.server_source = 'private'
     AND credential.server_ref = server.id::text
    WHERE server.auth_config #>> '{metadata,legacyInstallRepair}' = '078'
  ) THEN
    RAISE EXCEPTION
      'refusing to roll back repaired Tavily Runner servers while credentials exist';
  END IF;
END $$;

UPDATE mcp_servers AS server
SET endpoint_url = 'https://mcp.tavily.com/mcp',
    transport = 'streamable_http',
    auth_type = 'none',
    auth_config = jsonb_set(
      (server.auth_config #- '{metadata,runnerArtifactId}')
        #- '{metadata,legacyInstallRepair}',
      '{metadata,marketplace,deploymentHash}',
      to_jsonb('70d1cd77529cffedb9624f4b3f62383e4aa43ba2bfba451c84d4b13e4a4e7d98'::text),
      true
    ),
    status = 'unavailable',
    tool_snapshot = '[]'::jsonb,
    tool_snapshot_hash = NULL,
    validated_at = NULL,
    last_error_code = 'connect_failed',
    updated_at = now()
WHERE server.transport = 'stdio'
  AND server.endpoint_url = 'runner://marketplace-tavily-ai-tavily-mcp-0.2.19'
  AND server.auth_type = 'env'
  AND server.auth_config #>> '{metadata,legacyInstallRepair}' = '078';
