-- Replace direct Candidate failure with an exact, confirmed operator gateway.
-- The underlying failure transition remains the migration 032 CAS boundary;
-- this migration adds bounded operator identity/reason and immutable audit.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE TABLE knowledge_structure_generation_abandonment_audits (
  candidate_generation_id UUID PRIMARY KEY
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  artifact_manifest_hash TEXT NOT NULL CHECK (
    artifact_manifest_hash ~ '^[0-9a-f]{64}$'
  ),
  failure_code TEXT NOT NULL CHECK (failure_code = 'OPERATOR_ABANDONED'),
  operator_id UUID NOT NULL,
  reason TEXT NOT NULL CHECK (
    reason = btrim(reason)
    AND octet_length(reason) BETWEEN 1 AND 1024
  ),
  head_revision BIGINT NOT NULL CHECK (head_revision >= 1),
  abandoned_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER knowledge_structure_generation_abandonment_audits_immutable
BEFORE UPDATE OR DELETE ON knowledge_structure_generation_abandonment_audits
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE FUNCTION knowledge_abandon_structure_generation_candidate(
  p_index_generation_id UUID,
  p_expected_head_revision BIGINT,
  p_expected_manifest_hash TEXT,
  p_operator_id UUID,
  p_reason TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  failed BOOLEAN;
  audit_matches BOOLEAN;
  normalized_reason TEXT := btrim(p_reason);
BEGIN
  IF p_index_generation_id IS NULL
    OR p_expected_head_revision IS NULL
    OR p_expected_head_revision < 1
    OR p_expected_manifest_hash IS NULL
    OR p_expected_manifest_hash !~ '^[0-9a-f]{64}$'
    OR p_operator_id IS NULL
    OR p_reason IS NULL
    OR normalized_reason = ''
    OR octet_length(normalized_reason) > 1024
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_STRUCTURE_ABANDON_ARGUMENT_INVALID';
  END IF;

  SELECT knowledge_fail_structure_generation(
    p_index_generation_id,
    p_expected_head_revision,
    p_expected_manifest_hash,
    'OPERATOR_ABANDONED'
  ) INTO failed;
  IF failed IS DISTINCT FROM true THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_STRUCTURE_ABANDON_FAILED';
  END IF;

  INSERT INTO knowledge_structure_generation_abandonment_audits(
    candidate_generation_id,
    artifact_manifest_hash,
    failure_code,
    operator_id,
    reason,
    head_revision
  ) VALUES (
    p_index_generation_id,
    p_expected_manifest_hash,
    'OPERATOR_ABANDONED',
    p_operator_id,
    normalized_reason,
    p_expected_head_revision
  )
  ON CONFLICT (candidate_generation_id) DO NOTHING;

  SELECT EXISTS (
    SELECT 1
    FROM knowledge_structure_generation_abandonment_audits audit
    WHERE audit.candidate_generation_id = p_index_generation_id
      AND audit.artifact_manifest_hash = p_expected_manifest_hash
      AND audit.failure_code = 'OPERATOR_ABANDONED'
      AND audit.operator_id = p_operator_id
      AND audit.reason = normalized_reason
      AND audit.head_revision = p_expected_head_revision
  ) INTO audit_matches;
  IF audit_matches IS DISTINCT FROM true THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_STRUCTURE_ABANDON_REPLAY_MISMATCH';
  END IF;
  RETURN true;
END
$function$;

ALTER TABLE knowledge_structure_generation_abandonment_audits
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_abandon_structure_generation_candidate(
  UUID, BIGINT, TEXT, UUID, TEXT
) OWNER TO rag_projection_owner;

REVOKE ALL ON knowledge_structure_generation_abandonment_audits FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_abandon_structure_generation_candidate(
  UUID, BIGINT, TEXT, UUID, TEXT
) FROM PUBLIC;
REVOKE EXECUTE ON FUNCTION knowledge_fail_structure_generation(
  UUID, BIGINT, TEXT, TEXT
) FROM rag_replay_operator;
REVOKE EXECUTE ON FUNCTION knowledge_abandon_structure_generation_candidate(
  UUID, BIGINT, TEXT, UUID, TEXT
) FROM go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_abandon_structure_generation_candidate(
  UUID, BIGINT, TEXT, UUID, TEXT
) TO rag_replay_operator;
