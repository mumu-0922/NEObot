-- Move structure-generation lifecycle mutations out of the API runtime and
-- expose bounded, source-text-free operator gateways. Candidate verification
-- still cannot activate; activation additionally records a validated external
-- gate-report hash and explicit operator identity.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE TABLE knowledge_structure_chunk_profile_descriptors (
  chunk_profile_hash TEXT PRIMARY KEY CHECK (
    chunk_profile_hash ~ '^[0-9a-f]{64}$'
  ),
  schema_version TEXT NOT NULL,
  profile JSONB NOT NULL CHECK (jsonb_typeof(profile) = 'object'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (profile->>'schemaVersion' = schema_version)
);

INSERT INTO knowledge_structure_chunk_profile_descriptors(
  chunk_profile_hash,
  schema_version,
  profile
) VALUES (
  '606d6ac1cca428a05a7dccce0b172aabfba893f02431834cdc75775342db88b1',
  'mm-chat.structure-chunk-profile.v2',
  '{
    "bounds": {
      "child": {"hardMaximum": 650, "target": 400, "targetMaximum": 500, "targetMinimum": 300},
      "overlap": {"maximum": 100, "target": 64},
      "parent": {"hardMaximum": 2000, "targetMaximum": 1600, "targetMinimum": 1200}
    },
    "derivedContext": {"citationAuthority": "original_source_span", "countedInOverlap": false, "maximumTokens": 96},
    "nonIndexable": {"policy": "preserve_source_exclude_retrieval", "signals": ["repeated_text", "page_position", "frequency"]},
    "routes": {
      "code": "logical_lines_then_token",
      "formula": "atomic_then_token",
      "json": "subtree_path_then_token",
      "narrative": "semantic_hint_then_sentence_recursive",
      "slide": "slide_shape_then_token",
      "table": "header_row_group_then_token"
    },
    "schemaVersion": "mm-chat.structure-chunk-profile.v2",
    "semantic": {
      "admission": "long_unstructured_narrative_only",
      "failure": "deterministic_sentence_recursive_fallback",
      "hintAuthority": "content_and_embedding_profile_hash_bound",
      "profileHash": "3c17b8c1ddbed7b0a241dc43bdb24d3615526e94700c0971e585aa25519b409d"
    },
    "tokenizer": {
      "artifactSha256": "223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7",
      "name": "cl100k_base",
      "normalization": "none",
      "profileHash": "bdff1b0c1c8195fc2fd0a1818bac2ca66a9332a53a5cdf3d434132dff02724a0",
      "revision": "openai-public-2022-12-14",
      "specialTokenPolicy": "encode_ordinary",
      "vocabularySha256": "d48a1992b71a810f377931afd97b5b28588e412918a3f2d9e445b019f29dc6e4"
    }
  }'::jsonb
);

CREATE TRIGGER knowledge_structure_chunk_profile_descriptors_immutable
BEFORE UPDATE OR DELETE ON knowledge_structure_chunk_profile_descriptors
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE TABLE knowledge_structure_generation_activation_audits (
  candidate_generation_id UUID PRIMARY KEY
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  previous_generation_id UUID NOT NULL
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  artifact_manifest_hash TEXT NOT NULL CHECK (
    artifact_manifest_hash ~ '^[0-9a-f]{64}$'
  ),
  gate_report_sha256 TEXT NOT NULL CHECK (
    gate_report_sha256 ~ '^[0-9a-f]{64}$'
  ),
  operator_id UUID NOT NULL,
  head_revision_before BIGINT NOT NULL CHECK (head_revision_before >= 1),
  head_revision_after BIGINT NOT NULL CHECK (
    head_revision_after = head_revision_before + 1
  ),
  activated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER knowledge_structure_generation_activation_audits_immutable
BEFORE UPDATE OR DELETE ON knowledge_structure_generation_activation_audits
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE FUNCTION knowledge_structure_generation_operator_status()
RETURNS TABLE(
  head_revision BIGINT,
  corpus_projection_revision BIGINT,
  active_generation_id UUID,
  active_generation_seq BIGINT,
  active_chunk_profile_hash TEXT,
  active_artifact_manifest_hash TEXT,
  candidate_generation_id UUID,
  candidate_generation_seq BIGINT,
  candidate_status TEXT,
  candidate_chunk_profile_hash TEXT,
  candidate_artifact_manifest_hash TEXT,
  candidate_readiness TEXT,
  candidate_document_count BIGINT,
  candidate_parent_count BIGINT,
  candidate_child_count BIGINT,
  pending_job_count BIGINT,
  processing_job_count BIGINT,
  succeeded_job_count BIGINT,
  failed_job_count BIGINT,
  activation_gate_report_sha256 TEXT
)
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT
    head.head_revision,
    head.corpus_projection_revision,
    active.id,
    active.generation_seq,
    active_profile.chunk_profile_hash,
    active.artifact_manifest_hash,
    candidate.id,
    candidate.generation_seq,
    candidate.status,
    candidate_profile.chunk_profile_hash,
    candidate.artifact_manifest_hash,
    candidate_state.readiness,
    COALESCE(candidate_state.document_count, 0),
    COALESCE(candidate_state.parent_count, 0),
    COALESCE(candidate_state.child_count, 0),
    count(job.id) FILTER (WHERE job.status = 'pending'),
    count(job.id) FILTER (WHERE job.status = 'processing'),
    count(job.id) FILTER (WHERE job.status = 'succeeded'),
    count(job.id) FILTER (WHERE job.status = 'failed'),
    activation.gate_report_sha256
  FROM knowledge_corpus_projection_head head
  JOIN knowledge_index_generations active
    ON active.id = head.active_index_generation_id
   AND active.status = 'active'
  JOIN knowledge_index_profiles active_profile
    ON active_profile.id = active.index_profile_id
  LEFT JOIN knowledge_index_generations candidate
    ON candidate.status IN ('building', 'verified')
  LEFT JOIN knowledge_index_profiles candidate_profile
    ON candidate_profile.id = candidate.index_profile_id
  LEFT JOIN knowledge_projection_state candidate_state
    ON candidate_state.index_generation_id = candidate.id
  LEFT JOIN knowledge_processing_jobs job
    ON job.index_generation_id = candidate.id
  LEFT JOIN knowledge_structure_generation_activation_audits activation
    ON activation.candidate_generation_id = active.id
  WHERE head.singleton_id = 1
  GROUP BY
    head.head_revision,
    head.corpus_projection_revision,
    active.id,
    active.generation_seq,
    active_profile.chunk_profile_hash,
    active.artifact_manifest_hash,
    candidate.id,
    candidate.generation_seq,
    candidate.status,
    candidate_profile.chunk_profile_hash,
    candidate.artifact_manifest_hash,
    candidate_state.readiness,
    candidate_state.document_count,
    candidate_state.parent_count,
    candidate_state.child_count,
    activation.gate_report_sha256;
$function$;

CREATE FUNCTION knowledge_list_structure_generation_rebuild_documents(
  p_expected_active_generation_id UUID,
  p_expected_head_revision BIGINT
) RETURNS TABLE(document_id UUID)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_expected_active_generation_id IS NULL
    OR p_expected_head_revision IS NULL
    OR p_expected_head_revision < 1
    OR NOT EXISTS (
      SELECT 1
      FROM knowledge_corpus_projection_head head
      JOIN knowledge_index_generations generation
        ON generation.id = head.active_index_generation_id
       AND generation.status = 'active'
      WHERE head.singleton_id = 1
        AND head.active_index_generation_id = p_expected_active_generation_id
        AND head.head_revision = p_expected_head_revision
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_STRUCTURE_OPERATOR_HEAD_STALE';
  END IF;

  RETURN QUERY
  SELECT document.id
  FROM knowledge_documents document
  JOIN knowledge_document_versions version
    ON version.id = document.current_version_id
   AND version.document_id = document.id
   AND version.status = 'active'
  JOIN files file
    ON file.id = version.file_id
   AND file.upload_status = 'available'
   AND file.deleted_at IS NULL
  WHERE document.status = 'active'
    AND document.deleted_at IS NULL
  ORDER BY document.id;
END
$function$;

CREATE FUNCTION knowledge_begin_registered_structure_generation_rebuild(
  p_index_profile_id UUID,
  p_search_profile_id UUID,
  p_generation_id UUID,
  p_chunk_profile_hash TEXT,
  p_base_profile_hash TEXT,
  p_parser_manifest_hash TEXT,
  p_search_profile_hash TEXT,
  p_build_snapshot_hash TEXT,
  p_allocations JSONB
) RETURNS TABLE(
  candidate_generation_id UUID,
  allocated_document_count BIGINT,
  active_generation_id UUID
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_structure_chunk_profile_descriptors descriptor
    WHERE descriptor.chunk_profile_hash = p_chunk_profile_hash
      AND descriptor.schema_version = 'mm-chat.structure-chunk-profile.v2'
  )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_STRUCTURE_OPERATOR_PROFILE_UNREGISTERED';
  END IF;

  RETURN QUERY
  SELECT rebuild.candidate_generation_id,
    rebuild.allocated_document_count,
    rebuild.active_generation_id
  FROM knowledge_begin_structure_generation_rebuild(
    p_index_profile_id,
    p_search_profile_id,
    p_generation_id,
    p_chunk_profile_hash,
    p_base_profile_hash,
    p_parser_manifest_hash,
    p_search_profile_hash,
    p_build_snapshot_hash,
    p_allocations
  ) rebuild;
END
$function$;

CREATE FUNCTION knowledge_activate_structure_generation_candidate(
  p_index_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_manifest_hash TEXT,
  p_gate_report_sha256 TEXT,
  p_operator_id UUID
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  previous_generation_id UUID;
  promoted BOOLEAN;
  resulting_head_revision BIGINT;
BEGIN
  IF p_gate_report_sha256 IS NULL
    OR p_gate_report_sha256 !~ '^[0-9a-f]{64}$'
    OR p_operator_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_STRUCTURE_ACTIVATION_AUDIT_INVALID';
  END IF;

  SELECT head.active_index_generation_id
    INTO previous_generation_id
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id = 1
    AND head.head_revision = p_expected_head_revision;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_STRUCTURE_ACTIVATION_HEAD_STALE';
  END IF;

  SELECT knowledge_promote_index_generation(
    p_index_generation_id,
    p_expected_head_revision,
    p_manifest_hash
  ) INTO promoted;
  IF promoted IS DISTINCT FROM true THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_STRUCTURE_ACTIVATION_FAILED';
  END IF;

  SELECT head.head_revision INTO resulting_head_revision
  FROM knowledge_corpus_projection_head head
  WHERE head.singleton_id = 1
    AND head.active_index_generation_id = p_index_generation_id;

  INSERT INTO knowledge_structure_generation_activation_audits(
    candidate_generation_id,
    previous_generation_id,
    artifact_manifest_hash,
    gate_report_sha256,
    operator_id,
    head_revision_before,
    head_revision_after
  ) VALUES (
    p_index_generation_id,
    previous_generation_id,
    p_manifest_hash,
    p_gate_report_sha256,
    p_operator_id,
    p_expected_head_revision,
    resulting_head_revision
  );
  RETURN true;
END
$function$;

ALTER TABLE knowledge_structure_chunk_profile_descriptors
  OWNER TO rag_projection_owner;
ALTER TABLE knowledge_structure_generation_activation_audits
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_structure_generation_operator_status()
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_list_structure_generation_rebuild_documents(UUID, BIGINT)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_begin_registered_structure_generation_rebuild(
  UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_activate_structure_generation_candidate(
  UUID, BIGINT, TEXT, TEXT, UUID
) OWNER TO rag_projection_owner;

REVOKE ALL ON knowledge_structure_chunk_profile_descriptors FROM PUBLIC;
REVOKE ALL ON knowledge_structure_generation_activation_audits FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_structure_generation_operator_status()
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_list_structure_generation_rebuild_documents(
  UUID, BIGINT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_begin_registered_structure_generation_rebuild(
  UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_activate_structure_generation_candidate(
  UUID, BIGINT, TEXT, TEXT, UUID
) FROM PUBLIC;

REVOKE EXECUTE ON FUNCTION knowledge_begin_structure_generation_rebuild(
  UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
) FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION knowledge_verify_structure_generation(UUID, BIGINT, TEXT)
  FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION knowledge_fail_structure_generation(UUID, BIGINT, TEXT, TEXT)
  FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION knowledge_promote_index_generation(UUID, BIGINT, TEXT)
  FROM go_api_runtime;
REVOKE EXECUTE ON FUNCTION knowledge_rollback_index_generation(
  UUID, UUID, BIGINT, TEXT, TEXT
) FROM go_api_runtime;

GRANT EXECUTE ON FUNCTION knowledge_structure_generation_operator_status(),
  knowledge_list_structure_generation_rebuild_documents(UUID, BIGINT),
  knowledge_begin_registered_structure_generation_rebuild(
    UUID, UUID, UUID, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB
  ),
  knowledge_verify_structure_generation(UUID, BIGINT, TEXT),
  knowledge_fail_structure_generation(UUID, BIGINT, TEXT, TEXT),
  knowledge_activate_structure_generation_candidate(UUID, BIGINT, TEXT, TEXT, UUID),
  knowledge_rollback_index_generation(UUID, UUID, BIGINT, TEXT, TEXT)
TO rag_replay_operator;
