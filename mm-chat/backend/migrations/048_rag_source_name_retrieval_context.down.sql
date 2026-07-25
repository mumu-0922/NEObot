SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC, rag_worker_executor, go_api_runtime;
REVOKE ALL ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC, rag_replay_operator;
REVOKE ALL ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) FROM PUBLIC, go_evidence_hydrator, go_api_runtime;
REVOKE ALL ON FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) FROM PUBLIC, rag_replay_operator;

DROP FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
);
DROP FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
);
DROP FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
);
DROP FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
);

ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates_v47_base(
  UUID[], TEXT, REAL[], INTEGER
) RENAME TO knowledge_fetch_profiled_query_evidence_candidates;
ALTER FUNCTION knowledge_fetch_generation_evaluation_candidates_v47_base(
  UUID, UUID[], TEXT, REAL[], INTEGER
) RENAME TO knowledge_fetch_generation_evaluation_candidates;
ALTER FUNCTION knowledge_reauthorize_and_hydrate_evidence_v47_base(
  UUID, UUID, UUID, JSONB
) RENAME TO knowledge_reauthorize_and_hydrate_evidence;
ALTER FUNCTION knowledge_hydrate_generation_evaluation_evidence_v47_base(
  UUID, UUID[], JSONB
) RENAME TO knowledge_hydrate_generation_evaluation_evidence;

GRANT EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor, go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_fetch_generation_evaluation_candidates(
  UUID, UUID[], TEXT, REAL[], INTEGER
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) TO go_evidence_hydrator, go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_hydrate_generation_evaluation_evidence(
  UUID, UUID[], JSONB
) TO rag_replay_operator;

DROP FUNCTION knowledge_fetch_source_name_evidence_candidates(
  UUID, UUID[], TEXT, INTEGER
);
DROP FUNCTION knowledge_source_name_key(TEXT);
