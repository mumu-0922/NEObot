-- Retire the former checked-in Browser (Playwright) manifest authority.
-- Historical Run snapshots and Tool call/result evidence remain immutable.

WITH deleted AS (
  DELETE FROM mcp_conversation_servers
  WHERE server_source = 'manifest'
    AND server_ref = 'playwright-browser-0.0.79'
  RETURNING conversation_id
)
UPDATE mcp_conversation_selections AS selection
SET revision = selection.revision + 1,
    updated_at = now()
WHERE selection.conversation_id IN (
  SELECT DISTINCT conversation_id FROM deleted
);

WITH deleted AS (
  DELETE FROM mcp_workspace_servers
  WHERE server_source = 'manifest'
    AND server_ref = 'playwright-browser-0.0.79'
  RETURNING workspace_id
)
UPDATE mcp_workspace_selections AS selection
SET revision = selection.revision + 1,
    updated_at = now()
WHERE selection.workspace_id IN (
  SELECT DISTINCT workspace_id FROM deleted
);

DELETE FROM mcp_credentials
WHERE server_source = 'manifest'
  AND server_ref = 'playwright-browser-0.0.79';

DELETE FROM mcp_oauth_states
WHERE server_source = 'manifest'
  AND server_ref = 'playwright-browser-0.0.79';

DELETE FROM mcp_server_grants
WHERE server_source = 'manifest'
  AND server_ref = 'playwright-browser-0.0.79';
