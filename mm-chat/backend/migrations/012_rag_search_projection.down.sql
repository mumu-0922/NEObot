-- Roll back G7.4 only while no search projection state exists.  Search rows
-- bind child chunks to provider embeddings and must be purged by an explicit
-- operator flow before schema downgrade.

DO $rag_search_projection_down$
BEGIN
  IF EXISTS (SELECT 1 FROM knowledge_child_search_projections)
    OR EXISTS (SELECT 1 FROM knowledge_search_profiles)
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_DOWN_SEARCH_PROJECTION_STATE_EXISTS';
  END IF;
END
$rag_search_projection_down$;

DROP FUNCTION knowledge_assert_materialization_search_complete(
  UUID, BIGINT, TEXT, INTEGER
);
DROP INDEX idx_knowledge_child_search_projection_ready;
DROP INDEX idx_knowledge_child_search_projection_exact;
DROP INDEX idx_knowledge_child_search_projection_lexical;
DROP INDEX idx_knowledge_child_search_projection_generation;
DROP TABLE knowledge_child_search_projections;
DROP TRIGGER knowledge_search_profiles_immutable ON knowledge_search_profiles;
DROP TABLE knowledge_search_profiles;
