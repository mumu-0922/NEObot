ALTER TABLE mcp_servers
  DROP CONSTRAINT mcp_servers_transport_check;

ALTER TABLE mcp_servers
  ADD CONSTRAINT mcp_servers_transport_check
    CHECK (transport IN ('streamable_http', 'stdio')),
  ADD CONSTRAINT mcp_servers_stdio_endpoint_check CHECK (
    transport <> 'stdio'
    OR endpoint_url ~ '^runner://[a-z0-9][a-z0-9._-]{0,127}$'
  );
