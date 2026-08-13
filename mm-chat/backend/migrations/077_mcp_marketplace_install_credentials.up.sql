ALTER TABLE mcp_servers
  DROP CONSTRAINT mcp_servers_auth_type_check;

ALTER TABLE mcp_servers
  ADD CONSTRAINT mcp_servers_auth_type_check
    CHECK (auth_type IN ('none', 'header', 'oauth', 'env'));

ALTER TABLE mcp_credentials
  DROP CONSTRAINT mcp_credentials_kind_check;

ALTER TABLE mcp_credentials
  ADD CONSTRAINT mcp_credentials_kind_check
    CHECK (kind IN ('header', 'oauth', 'env'));
