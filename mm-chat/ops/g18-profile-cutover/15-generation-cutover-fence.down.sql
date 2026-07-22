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

DROP TRIGGER knowledge_corpus_head_pg17_retrieval_fence
  ON knowledge_corpus_projection_head;
REVOKE EXECUTE ON FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  FROM rag_replay_operator;
DROP FUNCTION knowledge_fence_pg17_generation_cutover();
DROP FUNCTION knowledge_assert_pg17_generation_ready(UUID);

SELECT 'PASS G18.5B.2b generation cutover fence rollback' AS result;
