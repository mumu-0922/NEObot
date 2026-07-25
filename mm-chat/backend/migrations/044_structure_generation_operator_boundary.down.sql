SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

REVOKE EXECUTE ON FUNCTION knowledge_structure_generation_operator_status(),
  knowledge_list_structure_generation_rebuild_documents(UUID, BIGINT),
  knowledge_begin_registered_structure_generation_rebuild(
    UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
  ),
  knowledge_verify_structure_generation(UUID, BIGINT, TEXT),
  knowledge_fail_structure_generation(UUID, BIGINT, TEXT, TEXT),
  knowledge_activate_structure_generation_candidate(UUID, BIGINT, TEXT, TEXT, UUID),
  knowledge_rollback_index_generation(UUID, UUID, BIGINT, TEXT, TEXT)
FROM rag_replay_operator;

DROP FUNCTION knowledge_activate_structure_generation_candidate(
  UUID, BIGINT, TEXT, TEXT, UUID
);
DROP FUNCTION knowledge_begin_registered_structure_generation_rebuild(
  UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
);
DROP FUNCTION knowledge_list_structure_generation_rebuild_documents(UUID, BIGINT);
DROP FUNCTION knowledge_structure_generation_operator_status();

DROP TRIGGER knowledge_structure_generation_activation_audits_immutable
  ON knowledge_structure_generation_activation_audits;
DROP TABLE knowledge_structure_generation_activation_audits;
DROP TRIGGER knowledge_structure_chunk_profile_descriptors_immutable
  ON knowledge_structure_chunk_profile_descriptors;
DROP TABLE knowledge_structure_chunk_profile_descriptors;

GRANT EXECUTE ON FUNCTION knowledge_begin_structure_generation_rebuild(
  UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_verify_structure_generation(UUID, BIGINT, TEXT)
  TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_fail_structure_generation(UUID, BIGINT, TEXT, TEXT)
  TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_promote_index_generation(UUID, BIGINT, TEXT)
  TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_rollback_index_generation(
  UUID, UUID, BIGINT, TEXT, TEXT
) TO go_api_runtime;
