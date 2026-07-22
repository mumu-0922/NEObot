\set ON_ERROR_STOP on

DO $rollback_guard$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM knowledge_retrieval_profile_head
    WHERE singleton_id = 1
      AND active_profile <> 'legacy'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY';
  END IF;
END
$rollback_guard$;

DROP TRIGGER knowledge_document_projection_head_pg17_retrieval
  ON knowledge_document_projection_heads;
REVOKE EXECUTE ON FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID)
  FROM rag_replay_operator;
DROP FUNCTION knowledge_maintain_pg17_retrieval_on_head();
DROP FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID);

SELECT 'PASS G18.5B.2a active projection maintenance rollback' AS result;
