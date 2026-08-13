UPDATE mcp_servers AS server
SET auth_config = jsonb_set(
      server.auth_config,
      '{metadata}',
      CASE
        WHEN jsonb_typeof(server.auth_config -> 'metadata') = 'object'
          THEN server.auth_config -> 'metadata'
        ELSE '{}'::jsonb
      END || jsonb_build_object(
        'icon', 'https://deepwiki.com/favicon.ico',
        'legacyIconRepair', '080'
      ),
      true
    ),
    updated_at = now()
WHERE server.deleted_at IS NULL
  AND server.transport = 'streamable_http'
  AND server.endpoint_url IN (
    'https://mcp.deepwiki.com/mcp',
    'https://mcp.deepwiki.com/mcp/'
  )
  AND coalesce(server.auth_config #>> '{metadata,icon}', '') = '';
