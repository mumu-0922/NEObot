DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM mcp_servers
    WHERE transport = 'stdio'
  ) THEN
    RAISE EXCEPTION
      'cannot roll back 076_mcp_private_runner_artifacts while stdio MCP servers exist';
  END IF;
END
$$;

ALTER TABLE mcp_servers
  DROP CONSTRAINT mcp_servers_stdio_endpoint_check,
  DROP CONSTRAINT mcp_servers_transport_check;

ALTER TABLE mcp_servers
  ADD CONSTRAINT mcp_servers_transport_check
    CHECK (transport IN ('streamable_http'));
