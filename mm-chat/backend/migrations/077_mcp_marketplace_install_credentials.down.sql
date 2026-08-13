DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM mcp_servers WHERE auth_type = 'env')
     OR EXISTS (SELECT 1 FROM mcp_credentials WHERE kind = 'env') THEN
    RAISE EXCEPTION
      'refusing to remove MCP environment credentials while dependent rows exist';
  END IF;
END $$;

ALTER TABLE mcp_credentials
  DROP CONSTRAINT mcp_credentials_kind_check;

ALTER TABLE mcp_credentials
  ADD CONSTRAINT mcp_credentials_kind_check
    CHECK (kind IN ('header', 'oauth'));

ALTER TABLE mcp_servers
  DROP CONSTRAINT mcp_servers_auth_type_check;

ALTER TABLE mcp_servers
  ADD CONSTRAINT mcp_servers_auth_type_check
    CHECK (auth_type IN ('none', 'header', 'oauth'));
