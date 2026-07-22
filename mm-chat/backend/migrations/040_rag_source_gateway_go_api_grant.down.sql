SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

REVOKE EXECUTE ON FUNCTION knowledge_fetch_parse_source_metadata(
  UUID, UUID, UUID, UUID, UUID
) FROM go_api_runtime;
