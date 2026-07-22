-- The token-fenced Go source-object endpoint executes this existing hardened
-- metadata gateway through the dedicated Go API database login. Grant only the
-- function call; projection and file tables remain inaccessible directly.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

GRANT EXECUTE ON FUNCTION knowledge_fetch_parse_source_metadata(
  UUID, UUID, UUID, UUID, UUID
) TO go_api_runtime;
