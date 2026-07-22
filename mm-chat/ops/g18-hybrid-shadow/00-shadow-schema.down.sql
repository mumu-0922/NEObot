\set ON_ERROR_STOP on

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

DROP FUNCTION IF EXISTS knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
);
DROP FUNCTION IF EXISTS knowledge_backfill_bm25_shadow(UUID, UUID);
DROP TABLE IF EXISTS knowledge_child_bm25_shadow_projections;
DROP FUNCTION IF EXISTS knowledge_validate_bm25_shadow_insert();
DROP VIEW IF EXISTS knowledge_bm25_shadow_sources;
DROP VIEW IF EXISTS knowledge_bm25_shadow_build_sources;
DROP FUNCTION IF EXISTS knowledge_build_bm25_shadow_text(TEXT, TEXT[]);
DROP FUNCTION IF EXISTS knowledge_bm25_shadow_query_terms(TEXT);
DROP FUNCTION IF EXISTS knowledge_normalize_bm25_shadow_terms(TEXT[]);

SELECT 'PASS G18.4 BM25 hybrid shadow rollback' AS result;
