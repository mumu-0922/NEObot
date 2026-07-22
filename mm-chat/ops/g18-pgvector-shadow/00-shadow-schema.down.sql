\set ON_ERROR_STOP on

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

DROP FUNCTION IF EXISTS knowledge_backfill_pgvector_shadow(UUID, UUID);
DROP TABLE IF EXISTS knowledge_child_vector_shadow_projections;
DROP FUNCTION IF EXISTS knowledge_validate_pgvector_shadow_insert();
DROP VIEW IF EXISTS knowledge_pgvector_shadow_sources;

SELECT 'PASS G18.3 pgvector shadow rollback' AS result;
