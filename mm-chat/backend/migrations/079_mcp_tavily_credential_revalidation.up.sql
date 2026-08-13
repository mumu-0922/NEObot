UPDATE mcp_servers AS server
SET status = 'needs_auth',
    validated_at = NULL,
    last_error_code = 'credential_revalidation_required',
    updated_at = now()
WHERE server.deleted_at IS NULL
  AND server.transport = 'stdio'
  AND server.endpoint_url = 'runner://marketplace-tavily-ai-tavily-mcp-0.2.19'
  AND server.auth_type = 'env'
  AND server.auth_config #>> '{metadata,runnerArtifactId}' =
    'marketplace-tavily-ai-tavily-mcp-0.2.19'
  AND server.status = 'ready';
