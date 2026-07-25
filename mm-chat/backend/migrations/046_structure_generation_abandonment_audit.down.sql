SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

REVOKE EXECUTE ON FUNCTION knowledge_abandon_structure_generation_candidate(
  UUID, BIGINT, TEXT, UUID, TEXT
) FROM rag_replay_operator;

DROP FUNCTION knowledge_abandon_structure_generation_candidate(
  UUID, BIGINT, TEXT, UUID, TEXT
);
DROP TABLE knowledge_structure_generation_abandonment_audits;

GRANT EXECUTE ON FUNCTION knowledge_fail_structure_generation(
  UUID, BIGINT, TEXT, TEXT
) TO rag_replay_operator;
