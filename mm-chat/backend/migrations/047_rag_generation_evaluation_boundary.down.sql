REVOKE ALL ON FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) FROM rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) FROM rag_replay_operator;

DROP FUNCTION IF EXISTS knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
);
DROP FUNCTION IF EXISTS knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
);
