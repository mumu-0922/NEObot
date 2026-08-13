UPDATE mcp_servers AS server
SET auth_config = jsonb_set(
      server.auth_config,
      '{metadata}',
      (server.auth_config -> 'metadata') - 'icon' - 'legacyIconRepair',
      true
    ),
    updated_at = now()
WHERE server.deleted_at IS NULL
  AND server.transport = 'streamable_http'
  AND server.endpoint_url IN (
    'https://mcp.deepwiki.com/mcp',
    'https://mcp.deepwiki.com/mcp/'
  )
  AND server.auth_config #>> '{metadata,icon}' =
    'https://deepwiki.com/favicon.ico'
  AND server.auth_config #>> '{metadata,legacyIconRepair}' = '080';
