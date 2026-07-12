-- Conservative rollback: all preconditions run before the first destructive DDL.
DO $preconditions$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_proc function
    JOIN pg_namespace namespace ON namespace.oid = function.pronamespace
    WHERE namespace.nspname = current_schema()
      AND function.prosecdef
      AND function.proname LIKE 'knowledge_%'
      AND NOT COALESCE(
        function.proconfig @> ARRAY[
          'search_path=' || quote_ident(current_schema()) ||
            ', pg_catalog, pg_temp'
        ],
        false
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_DOWN_UNSAFE_FUNCTION_SEARCH_PATH';
  END IF;

  IF EXISTS (
    SELECT 1 FROM knowledge_outbox WHERE status = 'processing'
  ) OR EXISTS (
    SELECT 1 FROM knowledge_processing_jobs WHERE status = 'processing'
  ) OR EXISTS (
    SELECT 1 FROM knowledge_collection_purges WHERE status = 'processing'
  ) OR EXISTS (
    SELECT 1 FROM knowledge_collection_purge_items WHERE status = 'processing'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_DOWN_ACTIVE_LEASE';
  END IF;

  IF EXISTS (
    SELECT 1 FROM knowledge_processing_jobs WHERE NOT legacy_projection_unbound
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_DOWN_BOUND_JOB_EXISTS';
  END IF;

  IF EXISTS (
    SELECT id::TEXT FROM processor_governance_profiles
    EXCEPT SELECT entity_key FROM phase15_rag_projection_migration_baseline
      WHERE entity_kind = 'profile'
  ) OR EXISTS (
    SELECT processor || chr(31) || endpoint_id || chr(31) || model_id
    FROM processor_governance_heads
    EXCEPT SELECT entity_key FROM phase15_rag_projection_migration_baseline
      WHERE entity_kind = 'head'
  ) OR EXISTS (
    SELECT id::TEXT FROM processing_consents
    EXCEPT SELECT entity_key FROM phase15_rag_projection_migration_baseline
      WHERE entity_kind = 'consent'
  ) OR EXISTS (
    SELECT id::TEXT FROM knowledge_processing_jobs
    EXCEPT SELECT entity_key FROM phase15_rag_projection_migration_baseline
      WHERE entity_kind = 'job'
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_DOWN_POST_010_AUTHORITY_ROW_EXISTS';
  END IF;

  IF EXISTS (
    SELECT collection_id, processor
    FROM processing_consents
    WHERE scope = 'collection' AND superseded_at IS NULL
    GROUP BY collection_id, processor HAVING count(*) > 1
  ) OR EXISTS (
    SELECT user_id, processor
    FROM processing_consents
    WHERE scope = 'query' AND superseded_at IS NULL
    GROUP BY user_id, processor HAVING count(*) > 1
  ) OR EXISTS (
    SELECT collection_id, processor, consent_revision
    FROM processing_consents WHERE scope = 'collection'
    GROUP BY collection_id, processor, consent_revision HAVING count(*) > 1
  ) OR EXISTS (
    SELECT user_id, processor, consent_revision
    FROM processing_consents WHERE scope = 'query'
    GROUP BY user_id, processor, consent_revision HAVING count(*) > 1
  ) THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_DOWN_CONSENT_NAMESPACE_CONFLICT';
  END IF;

  IF EXISTS (SELECT 1 FROM knowledge_index_profiles)
    OR EXISTS (SELECT 1 FROM knowledge_index_generations)
    OR EXISTS (SELECT 1 FROM knowledge_projection_state)
    OR EXISTS (SELECT 1 FROM knowledge_document_materializations)
    OR EXISTS (SELECT 1 FROM knowledge_parser_artifact_sets)
    OR EXISTS (SELECT 1 FROM knowledge_parser_artifacts)
    OR EXISTS (SELECT 1 FROM knowledge_blocks)
    OR EXISTS (SELECT 1 FROM knowledge_parent_chunks)
    OR EXISTS (SELECT 1 FROM knowledge_child_chunks)
    OR EXISTS (SELECT 1 FROM knowledge_chunk_block_spans)
    OR EXISTS (SELECT 1 FROM knowledge_outbox_applied_events)
    OR EXISTS (SELECT 1 FROM knowledge_outbox_replays)
    OR EXISTS (SELECT 1 FROM knowledge_processing_job_replays)
    OR EXISTS (SELECT 1 FROM knowledge_collection_purges)
    OR EXISTS (SELECT 1 FROM knowledge_collection_purge_items)
    OR EXISTS (
      SELECT 1 FROM knowledge_corpus_projection_head
      WHERE singleton_id <> 1 OR active_index_generation_id IS NOT NULL
        OR corpus_projection_revision <> 1 OR head_revision <> 1
    )
  THEN
    RAISE EXCEPTION USING ERRCODE = 'P0001', MESSAGE = 'RAG_DOWN_PROJECTION_STATE_EXISTS';
  END IF;
END
$preconditions$;

REVOKE EXECUTE ON FUNCTION knowledge_claim_outbox(TEXT, UUID, UUID, INTEGER)
  FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_apply_and_ack_outbox(
  TEXT, UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT
) FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_retry_outbox(UUID, UUID, UUID, TEXT, INTEGER)
  FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_fail_outbox(UUID, UUID, UUID, TEXT)
  FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_claim_processing_job(UUID, UUID, INTEGER, TEXT[])
  FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_heartbeat_processing_job(UUID, UUID, UUID, INTEGER)
  FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_finish_processing_job(
  UUID, UUID, UUID, TEXT, TEXT, INTEGER
) FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_replay_outbox(UUID, TEXT, UUID, TEXT)
  FROM rag_replay_operator;
REVOKE EXECUTE ON FUNCTION knowledge_replay_processing_job(UUID, TEXT, UUID, UUID, TEXT)
  FROM rag_replay_operator;
REVOKE EXECUTE ON FUNCTION knowledge_claim_collection_purge(UUID, UUID, INTEGER)
  FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_enumerate_collection_purge(
  UUID, UUID, UUID, INTEGER, INTEGER
) FROM rag_worker_executor;
REVOKE EXECUTE ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) FROM go_evidence_hydrator;
REVOKE EXECUTE ON FUNCTION knowledge_rag_worker_readiness()
  FROM rag_worker_executor, rag_api_reader;

DROP FUNCTION knowledge_rag_worker_readiness();
DROP FUNCTION knowledge_reauthorize_and_hydrate_evidence(UUID, UUID, UUID, JSONB);
DROP FUNCTION knowledge_promote_index_generation(UUID, BIGINT, TEXT);
DROP FUNCTION knowledge_purge_materialization(UUID, UUID, UUID, BIGINT);
DROP FUNCTION knowledge_publish_materialization(
  UUID, UUID, UUID, UUID, BIGINT, BIGINT, TEXT
);
DROP FUNCTION knowledge_complete_collection_purge(UUID);
DROP FUNCTION knowledge_enumerate_collection_purge(
  UUID, UUID, UUID, INTEGER, INTEGER
);
DROP FUNCTION knowledge_claim_collection_purge(UUID, UUID, INTEGER);
DROP FUNCTION knowledge_finish_collection_purge_item(
  UUID, UUID, UUID, BOOLEAN, TEXT
);
DROP FUNCTION knowledge_claim_collection_purge_item(UUID, UUID, INTEGER);
DROP FUNCTION knowledge_replay_processing_job(UUID, TEXT, UUID, UUID, TEXT);
DROP FUNCTION knowledge_replay_outbox(UUID, TEXT, UUID, TEXT);
DROP FUNCTION knowledge_finish_processing_job(
  UUID, UUID, UUID, TEXT, TEXT, INTEGER
);
DROP FUNCTION knowledge_heartbeat_processing_job(UUID, UUID, UUID, INTEGER);
DROP FUNCTION knowledge_claim_processing_job(UUID, UUID, INTEGER, TEXT[]);
DROP FUNCTION knowledge_fail_outbox(UUID, UUID, UUID, TEXT);
DROP FUNCTION knowledge_retry_outbox(UUID, UUID, UUID, TEXT, INTEGER);
DROP FUNCTION knowledge_apply_and_ack_outbox(
  TEXT, UUID, UUID, UUID, TEXT, UUID, TEXT, TEXT
);
DROP FUNCTION knowledge_claim_outbox(TEXT, UUID, UUID, INTEGER);

REVOKE SELECT, INSERT, UPDATE ON
  knowledge_outbox,
  knowledge_processing_jobs
FROM rag_projection_owner;
REVOKE SELECT ON
  sessions,
  conversations,
  team_memberships,
  teams,
  knowledge_collections,
  knowledge_documents,
  knowledge_document_versions,
  knowledge_blocks
FROM rag_projection_owner;

DO $privileges$
DECLARE
  schema_name TEXT := current_schema();
BEGIN
  EXECUTE format(
    'REVOKE USAGE ON SCHEMA %I FROM rag_projection_owner, rag_worker_executor, rag_replay_operator, rag_api_reader, go_evidence_hydrator',
    schema_name
  );
END
$privileges$;

ALTER TABLE knowledge_processing_jobs
  DROP CONSTRAINT knowledge_processing_jobs_materialization_fk,
  DROP CONSTRAINT knowledge_processing_jobs_generation_fk;

DROP INDEX idx_knowledge_collection_purge_items_claim;
DROP TABLE knowledge_collection_purge_items;
DROP INDEX idx_knowledge_collection_purges_claim;
DROP TABLE knowledge_collection_purges;
DROP TABLE knowledge_processing_job_replays;
DROP TABLE knowledge_outbox_replays;
DROP INDEX idx_knowledge_outbox_applied_events_cursor;
DROP TABLE knowledge_outbox_applied_events;
DROP TRIGGER knowledge_chunk_block_spans_validate ON knowledge_chunk_block_spans;
DROP TABLE knowledge_chunk_block_spans;
DROP FUNCTION knowledge_validate_chunk_block_span();
DROP TRIGGER knowledge_child_chunks_immutable ON knowledge_child_chunks;
DROP TABLE knowledge_child_chunks;
DROP TRIGGER knowledge_parent_chunks_immutable ON knowledge_parent_chunks;
DROP TABLE knowledge_parent_chunks;
DROP TRIGGER knowledge_blocks_immutable ON knowledge_blocks;
DROP TABLE knowledge_blocks;
DROP TRIGGER knowledge_parser_artifacts_immutable ON knowledge_parser_artifacts;
DROP TABLE knowledge_parser_artifacts;
ALTER TABLE knowledge_document_materializations
  DROP CONSTRAINT knowledge_document_materializations_artifact_set_fk;
DROP TABLE knowledge_parser_artifact_sets;
DROP TABLE knowledge_document_projection_heads;
DROP TABLE knowledge_document_materializations;
DROP TABLE knowledge_projection_state;
DROP TABLE knowledge_corpus_projection_head;
DROP INDEX idx_knowledge_index_generations_one_candidate;
DROP INDEX idx_knowledge_index_generations_one_active;
DROP TABLE knowledge_index_generations;
DROP TRIGGER knowledge_index_profiles_immutable ON knowledge_index_profiles;
DROP TABLE knowledge_index_profiles;
DROP FUNCTION knowledge_reject_immutable_projection_mutation();

ALTER TABLE knowledge_processing_jobs
  DROP CONSTRAINT knowledge_processing_jobs_lease_token_shape_check,
  DROP CONSTRAINT knowledge_processing_jobs_projection_binding_check,
  DROP CONSTRAINT knowledge_processing_jobs_collection_consent_model_fk,
  DROP CONSTRAINT knowledge_processing_jobs_governance_head_model_fk,
  DROP CONSTRAINT knowledge_processing_jobs_governance_profile_model_fk,
  DROP CONSTRAINT knowledge_processing_jobs_model_not_blank,
  DROP CONSTRAINT knowledge_processing_jobs_authority_shape_check;

ALTER TABLE knowledge_processing_jobs
  ADD CONSTRAINT knowledge_processing_jobs_authority_shape_check CHECK (
    (
      stage IN ('parse', 'passage_embedding')
      AND operation IN ('initial', 'replace', 'reprocess')
      AND processor IS NOT NULL AND endpoint_id IS NOT NULL
      AND governance_profile_id IS NOT NULL
      AND governance_revision IS NOT NULL
      AND governance_head_revision IS NOT NULL
      AND collection_consent_id IS NOT NULL
      AND collection_consent_revision IS NOT NULL
    ) OR (
      stage = 'purge' AND operation = 'purge'
      AND processor IS NULL AND endpoint_id IS NULL
      AND governance_profile_id IS NULL AND governance_revision IS NULL
      AND governance_head_revision IS NULL
      AND collection_consent_id IS NULL
      AND collection_consent_revision IS NULL
    )
  ),
  ADD CONSTRAINT knowledge_processing_jobs_state_shape_check CHECK (
    (status = 'pending' AND lease_owner IS NULL AND lease_expires_at IS NULL
      AND completed_at IS NULL AND error_code IS NULL
      AND attempt_count < max_attempts)
    OR (status = 'processing' AND lease_owner IS NOT NULL
      AND lease_expires_at IS NOT NULL AND completed_at IS NULL
      AND error_code IS NULL AND attempt_count BETWEEN 1 AND max_attempts)
    OR (status = 'succeeded' AND lease_owner IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NULL AND attempt_count BETWEEN 1 AND max_attempts)
    OR (status = 'failed' AND lease_owner IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL
      AND error_code IS NOT NULL AND attempt_count BETWEEN 1 AND max_attempts)
    OR (status = 'cancelled' AND lease_owner IS NULL
      AND lease_expires_at IS NULL AND completed_at IS NOT NULL)
  );
ALTER TABLE knowledge_processing_jobs
  DROP COLUMN legacy_projection_unbound,
  DROP COLUMN model_id,
  DROP COLUMN materialization_id,
  DROP COLUMN index_generation_id,
  DROP COLUMN lease_token;

ALTER TABLE knowledge_outbox
  DROP CONSTRAINT knowledge_outbox_lease_shape_check,
  DROP CONSTRAINT knowledge_outbox_error_code_check,
  DROP CONSTRAINT knowledge_outbox_attempt_bound_check,
  DROP CONSTRAINT knowledge_outbox_id_event_unique,
  DROP COLUMN failed_at,
  DROP COLUMN error_code,
  DROP COLUMN lock_expires_at,
  DROP COLUMN lock_token,
  DROP COLUMN lock_owner,
  DROP COLUMN max_attempts,
  ADD CONSTRAINT knowledge_outbox_status_timestamps_check CHECK (
    (status = 'pending' AND locked_at IS NULL AND published_at IS NULL)
    OR (status = 'processing' AND locked_at IS NOT NULL AND published_at IS NULL)
    OR (status = 'published' AND published_at IS NOT NULL)
    OR (status = 'failed' AND published_at IS NULL)
  );

DROP INDEX idx_processing_consents_query_endpoint_model_revision;
DROP INDEX idx_processing_consents_collection_endpoint_model_revision;
DROP INDEX idx_processing_consents_current_query_endpoint_model;
DROP INDEX idx_processing_consents_current_collection_endpoint_model;

ALTER TABLE processing_consents
  DROP CONSTRAINT processing_consents_collection_job_model_binding_unique,
  DROP CONSTRAINT processing_consents_governance_head_model_fk,
  DROP CONSTRAINT processing_consents_governance_profile_model_fk,
  DROP CONSTRAINT processing_consents_model_not_blank;
ALTER TABLE processor_governance_heads
  DROP CONSTRAINT processor_governance_heads_active_profile_model_fk,
  DROP CONSTRAINT processor_governance_heads_pkey,
  DROP CONSTRAINT processor_governance_heads_model_not_blank;
ALTER TABLE processor_governance_profiles
  DROP CONSTRAINT processor_governance_profiles_id_model_hash_unique,
  DROP CONSTRAINT processor_governance_profiles_model_active_binding_unique,
  DROP CONSTRAINT processor_governance_profiles_model_binding_unique,
  DROP CONSTRAINT processor_governance_profiles_model_revision_unique,
  DROP CONSTRAINT processor_governance_profiles_contract_hash_check,
  DROP CONSTRAINT processor_governance_profiles_model_not_blank;

ALTER TABLE processor_governance_profiles
  ADD CONSTRAINT processor_governance_profiles_processor_revision_unique
    UNIQUE (processor, endpoint_id, governance_revision),
  ADD CONSTRAINT processor_governance_profiles_binding_unique
    UNIQUE (processor, endpoint_id, id, governance_revision),
  ADD CONSTRAINT processor_governance_profiles_active_binding_unique
    UNIQUE (processor, endpoint_id, id, governance_revision, status);
ALTER TABLE processor_governance_heads
  ADD CONSTRAINT processor_governance_heads_pkey PRIMARY KEY (processor, endpoint_id),
  ADD CONSTRAINT processor_governance_heads_active_profile_fk
    FOREIGN KEY (
      processor, endpoint_id, active_profile_id,
      active_governance_revision, active_profile_status
    ) REFERENCES processor_governance_profiles(
      processor, endpoint_id, id, governance_revision, status
    ) ON DELETE RESTRICT;
ALTER TABLE processing_consents
  ADD CONSTRAINT processing_consents_governance_profile_fk
    FOREIGN KEY (
      processor, endpoint_id, governance_profile_id, governance_revision
    ) REFERENCES processor_governance_profiles(
      processor, endpoint_id, id, governance_revision
    ) ON DELETE RESTRICT,
  ADD CONSTRAINT processing_consents_governance_head_fk
    FOREIGN KEY (processor, endpoint_id)
    REFERENCES processor_governance_heads(processor, endpoint_id)
    ON DELETE RESTRICT,
  ADD CONSTRAINT processing_consents_collection_job_binding_unique UNIQUE (
    collection_id, id, processor, endpoint_id, governance_profile_id,
    governance_revision, governance_head_revision, consent_revision
  );

ALTER TABLE knowledge_processing_jobs
  ADD CONSTRAINT knowledge_processing_jobs_governance_profile_fk
    FOREIGN KEY (
      processor, endpoint_id, governance_profile_id, governance_revision
    ) REFERENCES processor_governance_profiles(
      processor, endpoint_id, id, governance_revision
    ) ON DELETE RESTRICT,
  ADD CONSTRAINT knowledge_processing_jobs_governance_head_fk
    FOREIGN KEY (processor, endpoint_id)
    REFERENCES processor_governance_heads(processor, endpoint_id)
    ON DELETE RESTRICT,
  ADD CONSTRAINT knowledge_processing_jobs_collection_consent_fk
    FOREIGN KEY (
      collection_id, collection_consent_id, processor, endpoint_id,
      governance_profile_id, governance_revision,
      governance_head_revision, collection_consent_revision
    ) REFERENCES processing_consents(
      collection_id, id, processor, endpoint_id,
      governance_profile_id, governance_revision,
      governance_head_revision, consent_revision
    ) ON DELETE RESTRICT;

ALTER TABLE processing_consents DROP COLUMN model_id;
ALTER TABLE processor_governance_heads DROP COLUMN model_id;
ALTER TABLE processor_governance_profiles
  DROP COLUMN profile_contract_hash,
  DROP COLUMN model_id;

CREATE UNIQUE INDEX idx_processing_consents_current_collection_processor
  ON processing_consents(collection_id, processor)
  WHERE scope = 'collection' AND superseded_at IS NULL;
CREATE UNIQUE INDEX idx_processing_consents_current_query_processor
  ON processing_consents(user_id, processor)
  WHERE scope = 'query' AND superseded_at IS NULL;
CREATE UNIQUE INDEX idx_processing_consents_collection_revision
  ON processing_consents(collection_id, processor, consent_revision)
  WHERE scope = 'collection';
CREATE UNIQUE INDEX idx_processing_consents_query_revision
  ON processing_consents(user_id, processor, consent_revision)
  WHERE scope = 'query';

DROP TABLE phase15_rag_projection_migration_baseline;
