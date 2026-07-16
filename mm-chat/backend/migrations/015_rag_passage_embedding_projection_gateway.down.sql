DROP FUNCTION IF EXISTS knowledge_stage_passage_embedding(
  UUID, UUID, UUID, UUID, UUID, REAL[], TEXT
);
DROP FUNCTION IF EXISTS knowledge_fetch_passage_embedding_candidates(
  UUID, UUID, UUID, UUID
);
