-- G7.4 extension-independent Postgres search projection for Jina 1024 lanes.
-- The schema deliberately avoids CREATE EXTENSION so the existing single-server
-- Postgres image remains bootable; pgvector/BM25 promotion can add physical
-- accelerator indexes in a later reversible migration.

CREATE TABLE knowledge_search_profiles (
  id UUID PRIMARY KEY,
  index_profile_id UUID NOT NULL
    REFERENCES knowledge_index_profiles(id) ON DELETE RESTRICT,
  provider_profile_id TEXT NOT NULL CHECK (
    provider_profile_id = 'mineru_jina_postgres_v1'
  ),
  embedding_processor TEXT NOT NULL CHECK (embedding_processor = 'jina'),
  embedding_model_id TEXT NOT NULL CHECK (embedding_model_id = 'jina-embeddings-v4'),
  embedding_dimensions INTEGER NOT NULL CHECK (embedding_dimensions = 1024),
  rerank_processor TEXT NOT NULL CHECK (rerank_processor = 'jina'),
  rerank_model_id TEXT NOT NULL CHECK (rerank_model_id = 'jina-reranker-v3'),
  lexical_config JSONB NOT NULL CHECK (jsonb_typeof(lexical_config) = 'object'),
  exact_config JSONB NOT NULL CHECK (jsonb_typeof(exact_config) = 'object'),
  profile_hash TEXT NOT NULL UNIQUE CHECK (profile_hash ~ '^[0-9a-f]{64}$'),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (index_profile_id, provider_profile_id, embedding_model_id)
);
CREATE TRIGGER knowledge_search_profiles_immutable
BEFORE UPDATE OR DELETE ON knowledge_search_profiles
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE TABLE knowledge_child_search_projections (
  child_chunk_id UUID PRIMARY KEY,
  parent_chunk_id UUID NOT NULL,
  materialization_id UUID NOT NULL,
  index_generation_id UUID NOT NULL,
  collection_id UUID NOT NULL,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  search_profile_id UUID NOT NULL
    REFERENCES knowledge_search_profiles(id) ON DELETE RESTRICT,
  embedding_model_id TEXT NOT NULL CHECK (embedding_model_id = 'jina-embeddings-v4'),
  embedding_dimensions INTEGER NOT NULL CHECK (embedding_dimensions = 1024),
  embedding_vector REAL[],
  embedding_vector_sha256 TEXT CHECK (
    embedding_vector_sha256 IS NULL OR embedding_vector_sha256 ~ '^[0-9a-f]{64}$'
  ),
  lexical_text TEXT NOT NULL CHECK (
    length(lexical_text) > 0 AND octet_length(lexical_text) <= 65536
  ),
  lexical_tsv TSVECTOR GENERATED ALWAYS AS (
    to_tsvector('simple'::regconfig, lexical_text)
  ) STORED,
  exact_terms TEXT[] NOT NULL DEFAULT '{}',
  source_span_hash TEXT NOT NULL CHECK (source_span_hash ~ '^[0-9a-f]{64}$'),
  chunk_profile_hash TEXT NOT NULL CHECK (chunk_profile_hash ~ '^[0-9a-f]{64}$'),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  locator_summary JSONB NOT NULL CHECK (jsonb_typeof(locator_summary) = 'object'),
  status TEXT NOT NULL DEFAULT 'staging' CHECK (
    status IN ('staging', 'ready', 'purging', 'purged')
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ready_at TIMESTAMPTZ,
  purged_at TIMESTAMPTZ,
  FOREIGN KEY (
    child_chunk_id, materialization_id
  ) REFERENCES knowledge_child_chunks(id, materialization_id) ON DELETE RESTRICT,
  FOREIGN KEY (
    parent_chunk_id, materialization_id, index_generation_id,
    document_id, document_version_id
  ) REFERENCES knowledge_parent_chunks(
    id, materialization_id, index_generation_id, document_id,
    document_version_id
  ) ON DELETE RESTRICT,
  FOREIGN KEY (collection_id, document_id)
    REFERENCES knowledge_documents(collection_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (document_id, document_version_id)
    REFERENCES knowledge_document_versions(document_id, id) ON DELETE RESTRICT,
  CHECK (array_position(exact_terms, NULL) IS NULL),
  CHECK (
    embedding_vector IS NULL
    OR (
      cardinality(embedding_vector) = 1024
      AND array_position(embedding_vector, NULL) IS NULL
    )
  ),
  CHECK (
    (status = 'staging' AND ready_at IS NULL AND purged_at IS NULL)
    OR (status = 'ready' AND ready_at IS NOT NULL AND purged_at IS NULL
      AND embedding_vector IS NOT NULL AND embedding_vector_sha256 IS NOT NULL)
    OR (status = 'purging' AND purged_at IS NULL)
    OR (status = 'purged' AND purged_at IS NOT NULL)
  )
);
CREATE INDEX idx_knowledge_child_search_projection_generation
  ON knowledge_child_search_projections(index_generation_id, status, document_id);
CREATE INDEX idx_knowledge_child_search_projection_lexical
  ON knowledge_child_search_projections USING GIN (lexical_tsv);
CREATE INDEX idx_knowledge_child_search_projection_exact
  ON knowledge_child_search_projections USING GIN (exact_terms);
CREATE INDEX idx_knowledge_child_search_projection_ready
  ON knowledge_child_search_projections(index_generation_id, search_profile_id)
  WHERE status = 'ready';

CREATE FUNCTION knowledge_assert_materialization_search_complete(
  p_materialization_id UUID,
  p_expected_child_count BIGINT,
  p_expected_embedding_model_id TEXT,
  p_expected_embedding_dimensions INTEGER
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  materialization knowledge_document_materializations%ROWTYPE;
  child_count BIGINT;
  ready_count BIGINT;
BEGIN
  IF p_materialization_id IS NULL
    OR p_expected_child_count IS NULL OR p_expected_child_count < 0
    OR p_expected_embedding_model_id <> 'jina-embeddings-v4'
    OR p_expected_embedding_dimensions <> 1024
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_SEARCH_COMPLETENESS_ARGUMENT_INVALID';
  END IF;

  SELECT * INTO materialization
  FROM knowledge_document_materializations
  WHERE id = p_materialization_id
  FOR SHARE;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_SEARCH_MATERIALIZATION_MISSING';
  END IF;

  SELECT count(*) INTO child_count
  FROM knowledge_child_chunks child
  WHERE child.materialization_id = materialization.id;
  IF child_count <> p_expected_child_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_SEARCH_CHILD_COUNT_MISMATCH';
  END IF;

  SELECT count(*) INTO ready_count
  FROM knowledge_child_chunks child
  JOIN knowledge_child_search_projections search
    ON search.child_chunk_id = child.id
    AND search.materialization_id = child.materialization_id
    AND search.index_generation_id = child.index_generation_id
    AND search.document_id = child.document_id
    AND search.document_version_id = child.document_version_id
    AND search.source_span_hash = child.source_span_hash
    AND search.chunk_profile_hash = child.chunk_profile_hash
    AND search.content_hash = child.content_hash
  WHERE child.materialization_id = materialization.id
    AND search.status = 'ready'
    AND search.embedding_model_id = p_expected_embedding_model_id
    AND search.embedding_dimensions = p_expected_embedding_dimensions
    AND search.embedding_vector IS NOT NULL
    AND cardinality(search.embedding_vector) = p_expected_embedding_dimensions;

  IF ready_count <> child_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_SEARCH_PROJECTION_INCOMPLETE';
  END IF;

  RETURN true;
END
$function$;

ALTER FUNCTION knowledge_assert_materialization_search_complete(
  UUID, BIGINT, TEXT, INTEGER
) OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_assert_materialization_search_complete(
  UUID, BIGINT, TEXT, INTEGER
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_assert_materialization_search_complete(
  UUID, BIGINT, TEXT, INTEGER
) TO rag_worker_executor;

GRANT SELECT, INSERT ON
  knowledge_search_profiles,
  knowledge_child_search_projections
TO rag_projection_owner;
GRANT UPDATE(status, embedding_vector, embedding_vector_sha256, ready_at, purged_at)
  ON knowledge_child_search_projections
TO rag_projection_owner;
