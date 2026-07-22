-- G18.5A PG16-compatible retrieval profile pointer.
--
-- The Go reader moves behind this router while the pointer remains `legacy`.
-- Migration 038, applied only after a fresh PG17 restore, will add the
-- BM25/pgvector implementation and replace the guarded unavailable branch.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE TABLE knowledge_retrieval_profile_head (
  singleton_id SMALLINT PRIMARY KEY DEFAULT 1 CHECK (singleton_id = 1),
  active_profile TEXT NOT NULL CHECK (
    active_profile IN ('legacy', 'pg17_bm25_pgvector_v1')
  ),
  revision BIGINT NOT NULL CHECK (revision >= 1),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO knowledge_retrieval_profile_head (
  singleton_id, active_profile, revision
) VALUES (1, 'legacy', 1);

CREATE TABLE knowledge_retrieval_profile_transitions (
  id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  from_profile TEXT NOT NULL CHECK (
    from_profile IN ('legacy', 'pg17_bm25_pgvector_v1')
  ),
  to_profile TEXT NOT NULL CHECK (
    to_profile IN ('legacy', 'pg17_bm25_pgvector_v1')
  ),
  revision BIGINT NOT NULL UNIQUE CHECK (revision >= 2),
  reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 512),
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (from_profile <> to_profile)
);

CREATE TRIGGER knowledge_retrieval_profile_transitions_immutable
BEFORE UPDATE OR DELETE ON knowledge_retrieval_profile_transitions
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE FUNCTION knowledge_set_retrieval_profile(
  p_expected_profile TEXT,
  p_target_profile TEXT,
  p_expected_revision BIGINT,
  p_reason TEXT
) RETURNS TABLE(
  active_profile TEXT,
  revision BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  head knowledge_retrieval_profile_head%ROWTYPE;
  normalized_reason TEXT;
BEGIN
  normalized_reason := btrim(p_reason);
  IF p_expected_profile IS NULL
    OR p_expected_profile NOT IN ('legacy', 'pg17_bm25_pgvector_v1')
    OR p_target_profile IS NULL
    OR p_target_profile NOT IN ('legacy', 'pg17_bm25_pgvector_v1')
    OR p_expected_revision IS NULL
    OR p_expected_revision < 1
    OR normalized_reason IS NULL
    OR octet_length(normalized_reason) NOT BETWEEN 1 AND 512
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ARGUMENT_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 5);
  SELECT profile.* INTO head
  FROM knowledge_retrieval_profile_head profile
  WHERE profile.singleton_id = 1
  FOR UPDATE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;
  IF head.active_profile <> p_expected_profile
    OR head.revision <> p_expected_revision
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '40001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_CONFLICT';
  END IF;

  IF p_target_profile = 'pg17_bm25_pgvector_v1' THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
  END IF;
  IF p_target_profile = head.active_profile THEN
    RETURN QUERY SELECT head.active_profile, head.revision;
    RETURN;
  END IF;

  UPDATE knowledge_retrieval_profile_head profile
  SET active_profile = p_target_profile,
      revision = head.revision + 1,
      updated_at = clock_timestamp()
  WHERE profile.singleton_id = 1
  RETURNING profile.* INTO head;
  INSERT INTO knowledge_retrieval_profile_transitions (
    from_profile, to_profile, revision, reason
  ) VALUES (
    p_expected_profile,
    head.active_profile,
    head.revision,
    normalized_reason
  );

  RETURN QUERY SELECT head.active_profile, head.revision;
END
$function$;

CREATE FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding REAL[],
  p_limit INTEGER
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  rank_score REAL
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  selected_profile TEXT;
BEGIN
  SELECT profile.active_profile INTO selected_profile
  FROM knowledge_retrieval_profile_head profile
  WHERE profile.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;

  IF selected_profile = 'legacy' THEN
    RETURN QUERY
    SELECT candidate.*
    FROM knowledge_fetch_hybrid_query_evidence_candidates(
      p_collection_ids,
      p_query_text,
      p_query_embedding,
      p_limit
    ) candidate;
    RETURN;
  END IF;

  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
END
$function$;

ALTER TABLE knowledge_retrieval_profile_head OWNER TO rag_projection_owner;
ALTER TABLE knowledge_retrieval_profile_transitions
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON knowledge_retrieval_profile_head FROM PUBLIC;
REVOKE ALL ON knowledge_retrieval_profile_transitions FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor, go_api_runtime;
