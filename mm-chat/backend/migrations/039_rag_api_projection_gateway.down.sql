SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

REVOKE ALL ON FUNCTION knowledge_resolve_purge_projection_binding(
  UUID, UUID
) FROM go_api_runtime;
REVOKE ALL ON FUNCTION knowledge_is_document_version_actively_projected(
  UUID, UUID
) FROM go_api_runtime;
REVOKE ALL ON FUNCTION knowledge_allocate_parse_materialization(
  UUID, UUID, UUID
) FROM go_api_runtime;

DROP FUNCTION knowledge_resolve_purge_projection_binding(UUID, UUID);
DROP FUNCTION knowledge_is_document_version_actively_projected(UUID, UUID);
DROP FUNCTION knowledge_allocate_parse_materialization(UUID, UUID, UUID);
