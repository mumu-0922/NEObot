-- G18.5B.3 formal PostgreSQL 17 BM25/pgvector retrieval cutover.
--
-- This migration is intentionally PostgreSQL-17-only. It is applied only to a
-- fresh logical restore; never run PostgreSQL 17 against the old PG16 data
-- directory.

DO $pg17_extension_prerequisite$
BEGIN
  IF current_setting('server_version_num')::INTEGER / 10000 <> 17 THEN
    RAISE EXCEPTION USING
      ERRCODE = '0A000',
      MESSAGE = 'RAG_PG17_RETRIEVAL_REQUIRES_POSTGRESQL_17';
  END IF;
  IF NOT (
    'pg_textsearch' = ANY(regexp_split_to_array(
      current_setting('shared_preload_libraries'),
      '\s*,\s*'
    ))
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_PG17_RETRIEVAL_REQUIRES_PG_TEXTSEARCH_PRELOAD';
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM pg_available_extension_versions
    WHERE name = 'vector' AND version = '0.8.5'
  ) OR NOT EXISTS (
    SELECT 1
    FROM pg_available_extension_versions
    WHERE name = 'pg_textsearch' AND version = '1.3.1'
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '0A000',
      MESSAGE = 'RAG_PG17_RETRIEVAL_EXTENSION_VERSION_UNAVAILABLE';
  END IF;
END
$pg17_extension_prerequisite$;

CREATE EXTENSION IF NOT EXISTS vector VERSION '0.8.5';
CREATE EXTENSION IF NOT EXISTS pg_textsearch VERSION '1.3.1';

-- Frozen from `mm-chat/ops/g18-pgvector-shadow/00-shadow-schema.up.sql`.
DO $extension_contract$
BEGIN
  IF current_setting('server_version_num')::integer / 10000 <> 17 THEN
    RAISE EXCEPTION 'G18 pgvector shadow requires PostgreSQL 17';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'vector')
      <> '0.8.5' THEN
    RAISE EXCEPTION 'G18 pgvector shadow requires pgvector 0.8.5';
  END IF;
END
$extension_contract$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE VIEW knowledge_pgvector_shadow_sources
WITH (security_barrier = true)
AS
SELECT
  search.child_chunk_id,
  search.parent_chunk_id,
  search.materialization_id,
  search.index_generation_id,
  search.collection_id,
  search.document_id,
  search.document_version_id,
  search.search_profile_id,
  search.embedding_model_id,
  search.embedding_dimensions,
  search.embedding_vector,
  search.embedding_vector_sha256,
  search.source_span_hash,
  search.chunk_profile_hash,
  search.content_hash,
  materialization.collection_visibility_epoch,
  materialization.collection_processing_revision,
  materialization.document_visibility_epoch
FROM knowledge_child_search_projections search
JOIN knowledge_search_profiles search_profile
  ON search_profile.id = search.search_profile_id
 AND search_profile.embedding_processor = 'jina'
 AND search_profile.embedding_model_id = 'jina-embeddings-v4'
 AND search_profile.embedding_dimensions = 1024
JOIN knowledge_index_generations generation
  ON generation.id = search.index_generation_id
 AND generation.index_profile_id = search_profile.index_profile_id
 AND generation.status IN ('building', 'verified', 'active', 'retired')
JOIN knowledge_document_projection_heads document_head
  ON document_head.index_generation_id = search.index_generation_id
 AND document_head.document_id = search.document_id
 AND document_head.active_materialization_id = search.materialization_id
JOIN knowledge_document_materializations materialization
  ON materialization.id = search.materialization_id
 AND materialization.index_generation_id = search.index_generation_id
 AND materialization.collection_id = search.collection_id
 AND materialization.document_id = search.document_id
 AND materialization.document_version_id = search.document_version_id
 AND materialization.status = 'published'
JOIN knowledge_collections collection
  ON collection.id = search.collection_id
 AND collection.deleted_at IS NULL
 AND collection.visibility_epoch = materialization.collection_visibility_epoch
 AND collection.collection_processing_revision =
   materialization.collection_processing_revision
JOIN knowledge_documents document
  ON document.id = search.document_id
 AND document.collection_id = search.collection_id
 AND document.status = 'active'
 AND document.deleted_at IS NULL
 AND document.current_version_id = search.document_version_id
 AND document.visibility_epoch = materialization.document_visibility_epoch
JOIN knowledge_document_versions version
  ON version.id = search.document_version_id
 AND version.document_id = search.document_id
 AND version.status = 'active'
 AND version.content_hash = materialization.source_content_hash
JOIN knowledge_child_chunks child
  ON child.id = search.child_chunk_id
 AND child.parent_chunk_id = search.parent_chunk_id
 AND child.materialization_id = search.materialization_id
 AND child.index_generation_id = search.index_generation_id
 AND child.document_id = search.document_id
 AND child.document_version_id = search.document_version_id
 AND child.source_span_hash = search.source_span_hash
 AND child.chunk_profile_hash = search.chunk_profile_hash
 AND child.content_hash = search.content_hash
WHERE search.status = 'ready'
  AND search.embedding_model_id = 'jina-embeddings-v4'
  AND search.embedding_dimensions = 1024
  AND search.embedding_vector IS NOT NULL
  AND search.embedding_vector_sha256 IS NOT NULL
  AND cardinality(search.embedding_vector) = 1024;

CREATE TABLE knowledge_child_vector_shadow_projections (
  child_chunk_id UUID PRIMARY KEY
    REFERENCES knowledge_child_search_projections(child_chunk_id)
    ON DELETE RESTRICT,
  parent_chunk_id UUID NOT NULL,
  materialization_id UUID NOT NULL,
  index_generation_id UUID NOT NULL
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  collection_id UUID NOT NULL,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  search_profile_id UUID NOT NULL
    REFERENCES knowledge_search_profiles(id) ON DELETE RESTRICT,
  embedding_model_id TEXT NOT NULL CHECK (
    embedding_model_id = 'jina-embeddings-v4'
  ),
  embedding_dimensions INTEGER NOT NULL CHECK (embedding_dimensions = 1024),
  embedding_vector VECTOR(1024) NOT NULL,
  embedding_vector_sha256 TEXT NOT NULL CHECK (
    embedding_vector_sha256 ~ '^[0-9a-f]{64}$'
  ),
  embedding_norm DOUBLE PRECISION NOT NULL CHECK (
    embedding_norm > 0
    AND embedding_norm::TEXT NOT IN ('NaN', 'Infinity', '-Infinity')
  ),
  source_span_hash TEXT NOT NULL CHECK (
    source_span_hash ~ '^[0-9a-f]{64}$'
  ),
  chunk_profile_hash TEXT NOT NULL CHECK (
    chunk_profile_hash ~ '^[0-9a-f]{64}$'
  ),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  collection_visibility_epoch BIGINT NOT NULL CHECK (
    collection_visibility_epoch >= 1
  ),
  collection_processing_revision BIGINT NOT NULL CHECK (
    collection_processing_revision >= 1
  ),
  document_visibility_epoch BIGINT NOT NULL CHECK (
    document_visibility_epoch >= 1
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (child_chunk_id, index_generation_id, search_profile_id),
  FOREIGN KEY (child_chunk_id, materialization_id)
    REFERENCES knowledge_child_chunks(id, materialization_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (
    parent_chunk_id, materialization_id, index_generation_id,
    document_id, document_version_id
  ) REFERENCES knowledge_parent_chunks(
    id, materialization_id, index_generation_id,
    document_id, document_version_id
  ) ON DELETE RESTRICT,
  FOREIGN KEY (collection_id, document_id)
    REFERENCES knowledge_documents(collection_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (document_id, document_version_id)
    REFERENCES knowledge_document_versions(document_id, id)
    ON DELETE RESTRICT
);

CREATE INDEX idx_knowledge_child_vector_shadow_scope
  ON knowledge_child_vector_shadow_projections(
    index_generation_id, search_profile_id, collection_id, child_chunk_id
  );

CREATE INDEX idx_knowledge_child_vector_shadow_hnsw
  ON knowledge_child_vector_shadow_projections
  USING hnsw (embedding_vector vector_cosine_ops)
  WITH (m = 16, ef_construction = 64);

CREATE FUNCTION knowledge_validate_pgvector_shadow_insert()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  source knowledge_pgvector_shadow_sources%ROWTYPE;
  source_norm DOUBLE PRECISION;
BEGIN
  SELECT candidate.* INTO source
  FROM knowledge_pgvector_shadow_sources candidate
  WHERE candidate.child_chunk_id = NEW.child_chunk_id
    AND candidate.parent_chunk_id = NEW.parent_chunk_id
    AND candidate.materialization_id = NEW.materialization_id
    AND candidate.index_generation_id = NEW.index_generation_id
    AND candidate.collection_id = NEW.collection_id
    AND candidate.document_id = NEW.document_id
    AND candidate.document_version_id = NEW.document_version_id
    AND candidate.search_profile_id = NEW.search_profile_id
    AND candidate.embedding_model_id = NEW.embedding_model_id
    AND candidate.embedding_dimensions = NEW.embedding_dimensions
    AND candidate.embedding_vector_sha256 = NEW.embedding_vector_sha256
    AND candidate.source_span_hash = NEW.source_span_hash
    AND candidate.chunk_profile_hash = NEW.chunk_profile_hash
    AND candidate.content_hash = NEW.content_hash
    AND candidate.collection_visibility_epoch =
      NEW.collection_visibility_epoch
    AND candidate.collection_processing_revision =
      NEW.collection_processing_revision
    AND candidate.document_visibility_epoch = NEW.document_visibility_epoch;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_SOURCE_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM unnest(source.embedding_vector) component
    WHERE component::TEXT IN ('NaN', 'Infinity', '-Infinity')
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_SOURCE_INVALID';
  END IF;

  SELECT sqrt(sum(component::DOUBLE PRECISION * component::DOUBLE PRECISION))
  INTO source_norm
  FROM unnest(source.embedding_vector) component;
  IF source_norm IS NULL
    OR source_norm <= 0
    OR source_norm::TEXT IN ('NaN', 'Infinity', '-Infinity')
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_SOURCE_INVALID';
  END IF;

  IF NEW.embedding_vector::REAL[] IS DISTINCT FROM source.embedding_vector
    OR abs(NEW.embedding_norm - source_norm) > 0.000001
    OR abs(NEW.embedding_norm - vector_norm(NEW.embedding_vector)) > 0.000001
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_CONVERSION_MISMATCH';
  END IF;

  RETURN NEW;
END
$function$;

CREATE TRIGGER knowledge_child_vector_shadow_validate_insert
BEFORE INSERT ON knowledge_child_vector_shadow_projections
FOR EACH ROW EXECUTE FUNCTION knowledge_validate_pgvector_shadow_insert();

CREATE TRIGGER knowledge_child_vector_shadow_immutable
BEFORE UPDATE OR DELETE ON knowledge_child_vector_shadow_projections
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE FUNCTION knowledge_backfill_pgvector_shadow(
  p_index_generation_id UUID,
  p_search_profile_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  inserted_count BIGINT,
  verified_shadow_count BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  profile_matches INTEGER;
  generation_matches INTEGER;
  v_eligible_count BIGINT;
  v_inserted_count BIGINT;
  v_verified_shadow_count BIGINT;
BEGIN
  IF p_index_generation_id IS NULL OR p_search_profile_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_ARGUMENT_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 3);

  SELECT count(*) INTO profile_matches
  FROM knowledge_search_profiles profile
  WHERE profile.id = p_search_profile_id
    AND profile.embedding_processor = 'jina'
    AND profile.embedding_model_id = 'jina-embeddings-v4'
    AND profile.embedding_dimensions = 1024;
  SELECT count(*) INTO generation_matches
  FROM knowledge_index_generations generation
  JOIN knowledge_search_profiles profile
    ON profile.id = p_search_profile_id
   AND profile.index_profile_id = generation.index_profile_id
  WHERE generation.id = p_index_generation_id
    AND generation.status IN ('building', 'verified', 'active', 'retired');
  IF profile_matches <> 1 OR generation_matches <> 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_PROFILE_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_pgvector_shadow_sources source
    WHERE source.index_generation_id = p_index_generation_id
      AND source.search_profile_id = p_search_profile_id
      AND (
        EXISTS (
          SELECT 1
          FROM unnest(source.embedding_vector) component
          WHERE component::TEXT IN ('NaN', 'Infinity', '-Infinity')
        )
        OR (
          SELECT sqrt(sum(
            component::DOUBLE PRECISION * component::DOUBLE PRECISION
          ))
          FROM unnest(source.embedding_vector) component
        ) <= 0
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_SOURCE_INVALID';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_child_vector_shadow_projections shadow
    JOIN knowledge_pgvector_shadow_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
    WHERE shadow.index_generation_id = p_index_generation_id
      AND shadow.search_profile_id = p_search_profile_id
      AND (
        shadow.index_generation_id <> source.index_generation_id
        OR shadow.search_profile_id <> source.search_profile_id
        OR shadow.embedding_vector_sha256 <>
          source.embedding_vector_sha256
        OR shadow.embedding_vector::REAL[] IS DISTINCT FROM
          source.embedding_vector
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_EXISTING_MISMATCH';
  END IF;

  SELECT count(*) INTO v_eligible_count
  FROM knowledge_pgvector_shadow_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id;

  INSERT INTO knowledge_child_vector_shadow_projections (
    child_chunk_id, parent_chunk_id, materialization_id,
    index_generation_id, collection_id, document_id, document_version_id,
    search_profile_id, embedding_model_id, embedding_dimensions,
    embedding_vector, embedding_vector_sha256, embedding_norm,
    source_span_hash, chunk_profile_hash, content_hash,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch
  )
  SELECT
    source.child_chunk_id,
    source.parent_chunk_id,
    source.materialization_id,
    source.index_generation_id,
    source.collection_id,
    source.document_id,
    source.document_version_id,
    source.search_profile_id,
    source.embedding_model_id,
    source.embedding_dimensions,
    source.embedding_vector::VECTOR(1024),
    source.embedding_vector_sha256,
    vector_norm(source.embedding_vector::VECTOR(1024)),
    source.source_span_hash,
    source.chunk_profile_hash,
    source.content_hash,
    source.collection_visibility_epoch,
    source.collection_processing_revision,
    source.document_visibility_epoch
  FROM knowledge_pgvector_shadow_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS v_inserted_count = ROW_COUNT;

  SELECT count(*) INTO v_verified_shadow_count
  FROM knowledge_pgvector_shadow_sources source
  JOIN knowledge_child_vector_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.embedding_model_id = source.embedding_model_id
   AND shadow.embedding_dimensions = source.embedding_dimensions
   AND shadow.embedding_vector_sha256 = source.embedding_vector_sha256
   AND shadow.embedding_vector::REAL[] = source.embedding_vector
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id;
  IF v_verified_shadow_count <> v_eligible_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_PGVECTOR_SHADOW_BACKFILL_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    v_eligible_count,
    v_inserted_count,
    v_verified_shadow_count;
END
$function$;

ALTER VIEW knowledge_pgvector_shadow_sources OWNER TO rag_projection_owner;
ALTER TABLE knowledge_child_vector_shadow_projections
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_validate_pgvector_shadow_insert()
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_backfill_pgvector_shadow(UUID, UUID)
  OWNER TO rag_projection_owner;

REVOKE ALL ON knowledge_pgvector_shadow_sources FROM PUBLIC;
REVOKE ALL ON knowledge_child_vector_shadow_projections FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_validate_pgvector_shadow_insert()
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_backfill_pgvector_shadow(UUID, UUID)
  FROM PUBLIC;

GRANT SELECT ON knowledge_pgvector_shadow_sources TO rag_projection_owner;
GRANT SELECT, INSERT ON knowledge_child_vector_shadow_projections
  TO rag_projection_owner;
GRANT EXECUTE ON FUNCTION knowledge_backfill_pgvector_shadow(UUID, UUID)
  TO rag_replay_operator;

SELECT 'PASS G18.3 pgvector shadow schema' AS result;

-- Frozen from `mm-chat/ops/g18-hybrid-shadow/00-shadow-schema.up.sql`.
DO $extension_contract$
BEGIN
  IF current_setting('server_version_num')::integer / 10000 <> 17 THEN
    RAISE EXCEPTION 'G18 hybrid shadow requires PostgreSQL 17';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'pg_textsearch')
      <> '1.3.1' THEN
    RAISE EXCEPTION 'G18 hybrid shadow requires pg_textsearch 1.3.1';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'vector')
      <> '0.8.5' THEN
    RAISE EXCEPTION 'G18 hybrid shadow requires pgvector 0.8.5';
  END IF;
  IF to_regclass('knowledge_child_vector_shadow_projections') IS NULL THEN
    RAISE EXCEPTION 'G18 hybrid shadow requires the G18.3 vector shadow';
  END IF;
END
$extension_contract$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_normalize_bm25_shadow_terms(p_terms TEXT[])
RETURNS TEXT[]
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $function$
  SELECT COALESCE(
    array_agg(normalized.term ORDER BY normalized.term),
    ARRAY[]::TEXT[]
  )
  FROM (
    SELECT DISTINCT lower(btrim(source.term)) AS term
    FROM unnest(COALESCE(p_terms, ARRAY[]::TEXT[])) AS source(term)
    WHERE octet_length(btrim(source.term)) BETWEEN 1 AND 512
    ORDER BY term
    LIMIT 64
  ) normalized;
$function$;

CREATE FUNCTION knowledge_bm25_shadow_query_terms(p_query_text TEXT)
RETURNS TEXT[]
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $function$
  SELECT knowledge_normalize_bm25_shadow_terms(
    array_agg(source.term ORDER BY source.ordinal)
  )
  FROM regexp_split_to_table(
    btrim(COALESCE(p_query_text, '')),
    '[[:space:],;，。！？；：、（）【】《》“”‘’…—]+'
  ) WITH ORDINALITY AS source(term, ordinal)
  WHERE octet_length(btrim(source.term)) BETWEEN 1 AND 512;
$function$;

CREATE FUNCTION knowledge_build_bm25_shadow_text(
  p_lexical_text TEXT,
  p_exact_terms TEXT[]
) RETURNS TEXT
LANGUAGE plpgsql
IMMUTABLE
PARALLEL SAFE
AS $function$
DECLARE
  normalized_text TEXT;
  normalized_terms TEXT[];
  compact_text TEXT;
  bigram_text TEXT;
BEGIN
  normalized_text := lower(btrim(COALESCE(p_lexical_text, '')));
  IF octet_length(normalized_text) NOT BETWEEN 1 AND 65536 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_BM25_SHADOW_TEXT_INVALID';
  END IF;

  normalized_terms := knowledge_normalize_bm25_shadow_terms(p_exact_terms);
  -- The simple configuration already tokenizes Latin text. Generate bounded
  -- bigrams only for CJK ideographs; Latin bigrams create broad false matches
  -- such as `weather` sharing `er` with unrelated English identifiers.
  compact_text := regexp_replace(normalized_text, '[^一-龥]+', '', 'g');
  SELECT string_agg(substr(compact_text, position, 2), ' ' ORDER BY position)
  INTO bigram_text
  FROM generate_series(
    1,
    least(greatest(char_length(compact_text) - 1, 0), 512)
  ) AS position;

  RETURN concat_ws(
    ' ',
    normalized_text,
    NULLIF(array_to_string(normalized_terms, ' '), ''),
    NULLIF(bigram_text, '')
  );
END
$function$;

CREATE VIEW knowledge_bm25_shadow_build_sources
AS
SELECT
  search.child_chunk_id,
  search.parent_chunk_id,
  search.materialization_id,
  search.index_generation_id,
  search.collection_id,
  search.document_id,
  search.document_version_id,
  search.search_profile_id,
  search.lexical_text,
  search.exact_terms,
  search.source_span_hash,
  search.chunk_profile_hash,
  search.content_hash,
  child.ordinal AS child_ordinal,
  materialization.collection_visibility_epoch,
  materialization.collection_processing_revision,
  materialization.document_visibility_epoch
FROM knowledge_child_search_projections search
JOIN knowledge_search_profiles search_profile
  ON search_profile.id = search.search_profile_id
 AND search_profile.embedding_processor = 'jina'
 AND search_profile.embedding_model_id = 'jina-embeddings-v4'
 AND search_profile.embedding_dimensions = 1024
JOIN knowledge_index_generations generation
  ON generation.id = search.index_generation_id
 AND generation.index_profile_id = search_profile.index_profile_id
 AND generation.status IN ('building', 'verified', 'active', 'retired')
JOIN knowledge_document_projection_heads document_head
  ON document_head.index_generation_id = search.index_generation_id
 AND document_head.document_id = search.document_id
 AND document_head.active_materialization_id = search.materialization_id
JOIN knowledge_document_materializations materialization
  ON materialization.id = search.materialization_id
 AND materialization.index_generation_id = search.index_generation_id
 AND materialization.collection_id = search.collection_id
 AND materialization.document_id = search.document_id
 AND materialization.document_version_id = search.document_version_id
 AND materialization.status = 'published'
JOIN knowledge_collections collection
  ON collection.id = search.collection_id
 AND collection.deleted_at IS NULL
 AND collection.visibility_epoch = materialization.collection_visibility_epoch
 AND collection.collection_processing_revision =
   materialization.collection_processing_revision
JOIN knowledge_documents document
  ON document.id = search.document_id
 AND document.collection_id = search.collection_id
 AND document.status = 'active'
 AND document.deleted_at IS NULL
 AND document.current_version_id = search.document_version_id
 AND document.visibility_epoch = materialization.document_visibility_epoch
JOIN knowledge_document_versions version
  ON version.id = search.document_version_id
 AND version.document_id = search.document_id
 AND version.status = 'active'
 AND version.content_hash = materialization.source_content_hash
JOIN knowledge_child_chunks child
  ON child.id = search.child_chunk_id
 AND child.parent_chunk_id = search.parent_chunk_id
 AND child.materialization_id = search.materialization_id
 AND child.index_generation_id = search.index_generation_id
 AND child.document_id = search.document_id
 AND child.document_version_id = search.document_version_id
 AND child.source_span_hash = search.source_span_hash
 AND child.chunk_profile_hash = search.chunk_profile_hash
 AND child.content_hash = search.content_hash
WHERE search.status = 'ready'
  AND search.embedding_model_id = 'jina-embeddings-v4'
  AND search.embedding_dimensions = 1024
  AND search.embedding_vector IS NOT NULL
  AND search.embedding_vector_sha256 IS NOT NULL
  AND cardinality(search.embedding_vector) = 1024;

CREATE VIEW knowledge_bm25_shadow_sources
AS
SELECT source.*
FROM knowledge_bm25_shadow_build_sources source
JOIN knowledge_corpus_projection_head corpus
  ON corpus.active_index_generation_id = source.index_generation_id
JOIN knowledge_index_generations generation
  ON generation.id = source.index_generation_id
 AND generation.status = 'active'
WHERE corpus.singleton_id = 1;

CREATE TABLE knowledge_child_bm25_shadow_projections (
  child_chunk_id UUID PRIMARY KEY
    REFERENCES knowledge_child_search_projections(child_chunk_id)
    ON DELETE RESTRICT,
  parent_chunk_id UUID NOT NULL,
  materialization_id UUID NOT NULL,
  index_generation_id UUID NOT NULL
    REFERENCES knowledge_index_generations(id) ON DELETE RESTRICT,
  collection_id UUID NOT NULL,
  document_id UUID NOT NULL,
  document_version_id UUID NOT NULL,
  search_profile_id UUID NOT NULL
    REFERENCES knowledge_search_profiles(id) ON DELETE RESTRICT,
  bm25_text TEXT NOT NULL CHECK (
    octet_length(bm25_text) BETWEEN 1 AND 131072
  ),
  exact_terms TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[] CHECK (
    array_position(exact_terms, NULL) IS NULL
    AND cardinality(exact_terms) <= 64
  ),
  source_span_hash TEXT NOT NULL CHECK (
    source_span_hash ~ '^[0-9a-f]{64}$'
  ),
  chunk_profile_hash TEXT NOT NULL CHECK (
    chunk_profile_hash ~ '^[0-9a-f]{64}$'
  ),
  content_hash TEXT NOT NULL CHECK (content_hash ~ '^[0-9a-f]{64}$'),
  child_ordinal INTEGER NOT NULL CHECK (child_ordinal >= 0),
  collection_visibility_epoch BIGINT NOT NULL CHECK (
    collection_visibility_epoch >= 1
  ),
  collection_processing_revision BIGINT NOT NULL CHECK (
    collection_processing_revision >= 1
  ),
  document_visibility_epoch BIGINT NOT NULL CHECK (
    document_visibility_epoch >= 1
  ),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (child_chunk_id, index_generation_id, search_profile_id),
  FOREIGN KEY (child_chunk_id, materialization_id)
    REFERENCES knowledge_child_chunks(id, materialization_id)
    ON DELETE RESTRICT,
  FOREIGN KEY (
    parent_chunk_id, materialization_id, index_generation_id,
    document_id, document_version_id
  ) REFERENCES knowledge_parent_chunks(
    id, materialization_id, index_generation_id,
    document_id, document_version_id
  ) ON DELETE RESTRICT,
  FOREIGN KEY (collection_id, document_id)
    REFERENCES knowledge_documents(collection_id, id) ON DELETE RESTRICT,
  FOREIGN KEY (document_id, document_version_id)
    REFERENCES knowledge_document_versions(document_id, id)
    ON DELETE RESTRICT
);

CREATE INDEX idx_knowledge_child_bm25_shadow_scope
  ON knowledge_child_bm25_shadow_projections(
    index_generation_id, search_profile_id, collection_id, child_chunk_id
  );
CREATE INDEX idx_knowledge_child_bm25_shadow_exact
  ON knowledge_child_bm25_shadow_projections USING gin(exact_terms);
CREATE INDEX idx_knowledge_child_bm25_shadow_text
  ON knowledge_child_bm25_shadow_projections
  USING bm25(bm25_text) WITH (text_config = 'simple');

CREATE FUNCTION knowledge_validate_bm25_shadow_insert()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  source knowledge_bm25_shadow_build_sources%ROWTYPE;
  expected_terms TEXT[];
  expected_text TEXT;
BEGIN
  SELECT candidate.* INTO source
  FROM knowledge_bm25_shadow_build_sources candidate
  WHERE candidate.child_chunk_id = NEW.child_chunk_id
    AND candidate.parent_chunk_id = NEW.parent_chunk_id
    AND candidate.materialization_id = NEW.materialization_id
    AND candidate.index_generation_id = NEW.index_generation_id
    AND candidate.collection_id = NEW.collection_id
    AND candidate.document_id = NEW.document_id
    AND candidate.document_version_id = NEW.document_version_id
    AND candidate.search_profile_id = NEW.search_profile_id
    AND candidate.source_span_hash = NEW.source_span_hash
    AND candidate.chunk_profile_hash = NEW.chunk_profile_hash
    AND candidate.content_hash = NEW.content_hash
    AND candidate.child_ordinal = NEW.child_ordinal
    AND candidate.collection_visibility_epoch =
      NEW.collection_visibility_epoch
    AND candidate.collection_processing_revision =
      NEW.collection_processing_revision
    AND candidate.document_visibility_epoch = NEW.document_visibility_epoch;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_BM25_SHADOW_SOURCE_MISMATCH';
  END IF;

  expected_terms := knowledge_normalize_bm25_shadow_terms(source.exact_terms);
  expected_text := knowledge_build_bm25_shadow_text(
    source.lexical_text,
    source.exact_terms
  );
  IF NEW.exact_terms IS DISTINCT FROM expected_terms
    OR NEW.bm25_text IS DISTINCT FROM expected_text
  THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_BM25_SHADOW_CONTENT_MISMATCH';
  END IF;

  RETURN NEW;
END
$function$;

CREATE TRIGGER knowledge_child_bm25_shadow_validate_insert
BEFORE INSERT ON knowledge_child_bm25_shadow_projections
FOR EACH ROW EXECUTE FUNCTION knowledge_validate_bm25_shadow_insert();

CREATE TRIGGER knowledge_child_bm25_shadow_immutable
BEFORE UPDATE OR DELETE ON knowledge_child_bm25_shadow_projections
FOR EACH ROW EXECUTE FUNCTION knowledge_reject_immutable_projection_mutation();

CREATE FUNCTION knowledge_backfill_bm25_shadow(
  p_index_generation_id UUID,
  p_search_profile_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  inserted_count BIGINT,
  verified_shadow_count BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  profile_matches INTEGER;
  generation_matches INTEGER;
  v_eligible_count BIGINT;
  v_inserted_count BIGINT;
  v_verified_shadow_count BIGINT;
BEGIN
  IF p_index_generation_id IS NULL OR p_search_profile_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_BM25_SHADOW_ARGUMENT_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 4);

  SELECT count(*) INTO profile_matches
  FROM knowledge_search_profiles profile
  WHERE profile.id = p_search_profile_id
    AND profile.embedding_processor = 'jina'
    AND profile.embedding_model_id = 'jina-embeddings-v4'
    AND profile.embedding_dimensions = 1024;
  SELECT count(*) INTO generation_matches
  FROM knowledge_index_generations generation
  JOIN knowledge_search_profiles profile
    ON profile.id = p_search_profile_id
   AND profile.index_profile_id = generation.index_profile_id
  WHERE generation.id = p_index_generation_id
    AND generation.status IN ('building', 'verified', 'active', 'retired');
  IF profile_matches <> 1 OR generation_matches <> 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_BM25_SHADOW_PROFILE_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN knowledge_bm25_shadow_build_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
    WHERE shadow.index_generation_id = p_index_generation_id
      AND shadow.search_profile_id = p_search_profile_id
      AND (
        shadow.index_generation_id <> source.index_generation_id
        OR shadow.search_profile_id <> source.search_profile_id
        OR shadow.bm25_text IS DISTINCT FROM
          knowledge_build_bm25_shadow_text(
            source.lexical_text,
            source.exact_terms
          )
        OR shadow.exact_terms IS DISTINCT FROM
          knowledge_normalize_bm25_shadow_terms(source.exact_terms)
      )
  ) THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_BM25_SHADOW_EXISTING_MISMATCH';
  END IF;

  SELECT count(*) INTO v_eligible_count
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id;

  INSERT INTO knowledge_child_bm25_shadow_projections (
    child_chunk_id, parent_chunk_id, materialization_id,
    index_generation_id, collection_id, document_id, document_version_id,
    search_profile_id, bm25_text, exact_terms, source_span_hash,
    chunk_profile_hash, content_hash, child_ordinal,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch
  )
  SELECT
    source.child_chunk_id,
    source.parent_chunk_id,
    source.materialization_id,
    source.index_generation_id,
    source.collection_id,
    source.document_id,
    source.document_version_id,
    source.search_profile_id,
    knowledge_build_bm25_shadow_text(
      source.lexical_text,
      source.exact_terms
    ),
    knowledge_normalize_bm25_shadow_terms(source.exact_terms),
    source.source_span_hash,
    source.chunk_profile_hash,
    source.content_hash,
    source.child_ordinal,
    source.collection_visibility_epoch,
    source.collection_processing_revision,
    source.document_visibility_epoch
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS v_inserted_count = ROW_COUNT;

  SELECT count(*) INTO v_verified_shadow_count
  FROM knowledge_bm25_shadow_build_sources source
  JOIN knowledge_child_bm25_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.bm25_text = knowledge_build_bm25_shadow_text(
     source.lexical_text,
     source.exact_terms
   )
   AND shadow.exact_terms =
     knowledge_normalize_bm25_shadow_terms(source.exact_terms)
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.child_ordinal = source.child_ordinal
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id;
  IF v_verified_shadow_count <> v_eligible_count THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_BM25_SHADOW_BACKFILL_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    v_eligible_count,
    v_inserted_count,
    v_verified_shadow_count;
END
$function$;

CREATE FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding VECTOR(1024),
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
  bm25_rank INTEGER,
  bm25_score DOUBLE PRECISION,
  dense_rank INTEGER,
  dense_score DOUBLE PRECISION,
  fused_rank INTEGER,
  fused_score DOUBLE PRECISION
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  normalized_query TEXT;
  query_terms TEXT[];
  bm25_query TEXT;
  query_norm DOUBLE PRECISION;
  oversample_limit INTEGER;
  minimum_dense_similarity CONSTANT DOUBLE PRECISION := 0.48;
  minimum_dense_query_characters CONSTANT INTEGER := 8;
  rrf_constant CONSTANT DOUBLE PRECISION := 60.0;
BEGIN
  normalized_query := btrim(p_query_text);
  IF p_collection_ids IS NULL
    OR cardinality(p_collection_ids) NOT BETWEEN 1 AND 32
    OR array_position(p_collection_ids, NULL) IS NOT NULL
    OR normalized_query IS NULL
    OR octet_length(normalized_query) NOT BETWEEN 1 AND 2048
    OR p_limit IS NULL
    OR p_limit NOT BETWEEN 1 AND 50
    OR p_query_embedding IS NULL
    OR EXISTS (
      SELECT 1
      FROM unnest(p_query_embedding::REAL[]) component
      WHERE component::TEXT IN ('NaN', 'Infinity', '-Infinity')
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_HYBRID_SHADOW_ARGUMENT_INVALID';
  END IF;

  query_norm := vector_norm(p_query_embedding);
  IF query_norm IS NULL
    OR query_norm <= 0
    OR query_norm::TEXT IN ('NaN', 'Infinity', '-Infinity')
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_HYBRID_SHADOW_ARGUMENT_INVALID';
  END IF;

  query_terms := knowledge_bm25_shadow_query_terms(normalized_query);
  bm25_query := knowledge_build_bm25_shadow_text(
    normalized_query,
    query_terms
  );
  oversample_limit := least(p_limit * 8, 400);

  RETURN QUERY
  WITH selected_collection AS (
    SELECT DISTINCT unnest(p_collection_ids) AS id
  ), bm25_probe AS MATERIALIZED (
    SELECT
      shadow.child_chunk_id,
      shadow.bm25_text <@> to_bm25query(
        bm25_query,
        'idx_knowledge_child_bm25_shadow_text'
      ) AS raw_score
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN selected_collection selected
      ON selected.id = shadow.collection_id
    WHERE shadow.bm25_text <@> to_bm25query(
      bm25_query,
      'idx_knowledge_child_bm25_shadow_text'
    ) < 0
    ORDER BY raw_score, shadow.child_chunk_id
    LIMIT oversample_limit
  ), exact_probe AS MATERIALIZED (
    SELECT shadow.child_chunk_id, 0::DOUBLE PRECISION AS raw_score
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN selected_collection selected
      ON selected.id = shadow.collection_id
    WHERE cardinality(query_terms) > 0
      AND shadow.exact_terms && query_terms
    ORDER BY shadow.child_chunk_id
    LIMIT oversample_limit
  ), lexical_pool AS (
    SELECT combined.child_chunk_id, min(combined.raw_score) AS raw_score
    FROM (
      SELECT probe.child_chunk_id, probe.raw_score FROM bm25_probe probe
      UNION ALL
      SELECT probe.child_chunk_id, probe.raw_score FROM exact_probe probe
    ) combined
    GROUP BY combined.child_chunk_id
  ), bm25_authorized AS (
    SELECT
      source.collection_id,
      source.document_id,
      source.document_version_id,
      source.index_generation_id,
      source.materialization_id,
      source.parent_chunk_id,
      source.child_chunk_id,
      source.source_span_hash,
      source.content_hash,
      pool.raw_score,
      shadow.exact_terms && query_terms AS exact_match,
      position(lower(normalized_query) IN lower(source.lexical_text)) > 0
        AS phrase_match,
      source.child_ordinal
    FROM lexical_pool pool
    JOIN knowledge_child_bm25_shadow_projections shadow
      ON shadow.child_chunk_id = pool.child_chunk_id
    CROSS JOIN LATERAL (
      SELECT candidate.*
      FROM knowledge_bm25_shadow_sources candidate
      WHERE candidate.child_chunk_id = shadow.child_chunk_id
        AND candidate.index_generation_id = shadow.index_generation_id
        AND candidate.search_profile_id = shadow.search_profile_id
        AND candidate.content_hash = shadow.content_hash
      OFFSET 0
    ) source
  ), bm25_ranked_unbounded AS (
    SELECT
      authorized.*,
      row_number() OVER (
        ORDER BY
          authorized.exact_match DESC,
          authorized.phrase_match DESC,
          authorized.raw_score,
          authorized.child_ordinal,
          authorized.child_chunk_id
      )::INTEGER AS lane_rank
    FROM bm25_authorized authorized
  ), bm25_ranked AS (
    SELECT *
    FROM bm25_ranked_unbounded ranked
    WHERE ranked.lane_rank <= p_limit
  ), dense_probe AS MATERIALIZED (
    SELECT
      shadow.child_chunk_id,
      shadow.index_generation_id,
      shadow.search_profile_id,
      shadow.content_hash,
      shadow.embedding_vector <=> p_query_embedding AS distance
    FROM knowledge_child_vector_shadow_projections shadow
    WHERE char_length(normalized_query) >= minimum_dense_query_characters
      AND shadow.collection_id = ANY(p_collection_ids)
      AND shadow.embedding_vector <=> p_query_embedding <=
        1 - minimum_dense_similarity
    ORDER BY shadow.embedding_vector <=> p_query_embedding
    LIMIT oversample_limit
  ), dense_scored AS MATERIALIZED (
    SELECT
      source.collection_id,
      source.document_id,
      source.document_version_id,
      source.index_generation_id,
      source.materialization_id,
      source.parent_chunk_id,
      source.child_chunk_id,
      source.source_span_hash,
      source.content_hash,
      1 - probe.distance AS similarity,
      source.child_ordinal
    FROM dense_probe probe
    CROSS JOIN LATERAL (
      SELECT candidate.*
      FROM knowledge_bm25_shadow_sources candidate
      WHERE candidate.child_chunk_id = probe.child_chunk_id
        AND candidate.index_generation_id = probe.index_generation_id
        AND candidate.search_profile_id = probe.search_profile_id
        AND candidate.content_hash = probe.content_hash
      OFFSET 0
    ) source
    ORDER BY
      probe.distance,
      source.child_ordinal,
      source.child_chunk_id
    LIMIT p_limit
  ), dense_ranked AS (
    SELECT
      scored.*,
      row_number() OVER (
        ORDER BY
          scored.similarity DESC,
          scored.child_ordinal,
          scored.child_chunk_id
      )::INTEGER AS lane_rank
    FROM dense_scored scored
  ), fused_base AS (
    SELECT
      COALESCE(bm25.collection_id, dense.collection_id) AS collection_id,
      COALESCE(bm25.document_id, dense.document_id) AS document_id,
      COALESCE(
        bm25.document_version_id,
        dense.document_version_id
      ) AS document_version_id,
      COALESCE(
        bm25.index_generation_id,
        dense.index_generation_id
      ) AS index_generation_id,
      COALESCE(
        bm25.materialization_id,
        dense.materialization_id
      ) AS materialization_id,
      COALESCE(bm25.parent_chunk_id, dense.parent_chunk_id)
        AS parent_chunk_id,
      COALESCE(bm25.child_chunk_id, dense.child_chunk_id) AS child_chunk_id,
      COALESCE(bm25.source_span_hash, dense.source_span_hash)
        AS source_span_hash,
      COALESCE(bm25.content_hash, dense.content_hash) AS content_hash,
      bm25.lane_rank AS bm25_rank,
      bm25.raw_score AS bm25_score,
      dense.lane_rank AS dense_rank,
      dense.similarity AS dense_score,
      (
        CASE WHEN bm25.lane_rank IS NULL THEN 0
          ELSE 1.0 / (rrf_constant + bm25.lane_rank) END
        + CASE WHEN dense.lane_rank IS NULL THEN 0
          ELSE 1.0 / (rrf_constant + dense.lane_rank) END
      ) AS fused_score
    FROM bm25_ranked bm25
    FULL JOIN dense_ranked dense
      ON dense.child_chunk_id = bm25.child_chunk_id
  ), fused_ranked AS (
    SELECT
      fused.*,
      row_number() OVER (
        ORDER BY
          fused.fused_score DESC,
          least(
            COALESCE(fused.bm25_rank, 2147483647),
            COALESCE(fused.dense_rank, 2147483647)
          ),
          fused.child_chunk_id
      )::INTEGER AS fused_rank
    FROM fused_base fused
  )
  SELECT
    fused.collection_id,
    fused.document_id,
    fused.document_version_id,
    fused.index_generation_id,
    fused.materialization_id,
    fused.parent_chunk_id,
    fused.child_chunk_id,
    fused.source_span_hash,
    fused.content_hash,
    fused.bm25_rank,
    fused.bm25_score,
    fused.dense_rank,
    fused.dense_score,
    fused.fused_rank,
    fused.fused_score
  FROM fused_ranked fused
  ORDER BY fused.fused_rank
  LIMIT p_limit;
END
$function$;

ALTER FUNCTION knowledge_normalize_bm25_shadow_terms(TEXT[])
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_bm25_shadow_query_terms(TEXT)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_build_bm25_shadow_text(TEXT, TEXT[])
  OWNER TO rag_projection_owner;
ALTER VIEW knowledge_bm25_shadow_build_sources OWNER TO rag_projection_owner;
ALTER VIEW knowledge_bm25_shadow_sources OWNER TO rag_projection_owner;
ALTER TABLE knowledge_child_bm25_shadow_projections
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_validate_bm25_shadow_insert()
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_backfill_bm25_shadow(UUID, UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_normalize_bm25_shadow_terms(TEXT[])
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_bm25_shadow_query_terms(TEXT)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_build_bm25_shadow_text(TEXT, TEXT[])
  FROM PUBLIC;
REVOKE ALL ON knowledge_bm25_shadow_build_sources FROM PUBLIC;
REVOKE ALL ON knowledge_bm25_shadow_sources FROM PUBLIC;
REVOKE ALL ON knowledge_child_bm25_shadow_projections FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_validate_bm25_shadow_insert()
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_backfill_bm25_shadow(UUID, UUID)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) FROM PUBLIC;

GRANT SELECT ON knowledge_bm25_shadow_build_sources TO rag_projection_owner;
GRANT SELECT ON knowledge_bm25_shadow_sources TO rag_projection_owner;
GRANT SELECT, INSERT ON knowledge_child_bm25_shadow_projections
  TO rag_projection_owner;
GRANT EXECUTE ON FUNCTION knowledge_backfill_bm25_shadow(UUID, UUID)
  TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) TO rag_replay_operator;

SELECT 'PASS G18.4 BM25 hybrid shadow schema' AS result;

-- Frozen from `mm-chat/ops/g18-profile-cutover/00-profile-router.up.sql`.
DO $extension_contract$
BEGIN
  IF current_setting('server_version_num')::integer / 10000 <> 17 THEN
    RAISE EXCEPTION 'G18 profile cutover requires PostgreSQL 17';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'pg_textsearch')
      <> '1.3.1' THEN
    RAISE EXCEPTION 'G18 profile cutover requires pg_textsearch 1.3.1';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'vector')
      <> '0.8.5' THEN
    RAISE EXCEPTION 'G18 profile cutover requires pgvector 0.8.5';
  END IF;
  IF to_regclass('knowledge_child_vector_shadow_projections') IS NULL
    OR to_regclass('knowledge_child_bm25_shadow_projections') IS NULL
    OR to_regprocedure(
      'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)'
    ) IS NULL
  THEN
    RAISE EXCEPTION 'G18 profile cutover requires reviewed shadow schema';
  END IF;
END
$extension_contract$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  eligible_count BIGINT,
  vector_count BIGINT,
  bm25_count BIGINT
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_generation_id UUID;
  active_search_profile_id UUID;
  expected_count BIGINT;
  verified_vector_count BIGINT;
  verified_bm25_count BIGINT;
BEGIN
  SELECT generation.id, profile.id
  INTO active_generation_id, active_search_profile_id
  FROM knowledge_corpus_projection_head corpus
  JOIN knowledge_index_generations generation
    ON generation.id = corpus.active_index_generation_id
   AND generation.status = 'active'
  JOIN knowledge_search_profiles profile
    ON profile.index_profile_id = generation.index_profile_id
   AND profile.provider_profile_id = 'mineru_jina_postgres_v1'
   AND profile.embedding_processor = 'jina'
   AND profile.embedding_model_id = 'jina-embeddings-v4'
   AND profile.embedding_dimensions = 1024
  WHERE corpus.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_ACTIVE_GENERATION_MISSING';
  END IF;

  SELECT count(*) INTO expected_count
  FROM knowledge_bm25_shadow_sources source
  WHERE source.index_generation_id = active_generation_id
    AND source.search_profile_id = active_search_profile_id;

  SELECT count(*) INTO verified_vector_count
  FROM knowledge_pgvector_shadow_sources source
  JOIN knowledge_child_vector_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.embedding_model_id = source.embedding_model_id
   AND shadow.embedding_dimensions = source.embedding_dimensions
   AND shadow.embedding_vector_sha256 = source.embedding_vector_sha256
   AND shadow.embedding_vector::REAL[] = source.embedding_vector
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = active_generation_id
    AND source.search_profile_id = active_search_profile_id;

  SELECT count(*) INTO verified_bm25_count
  FROM knowledge_bm25_shadow_sources source
  JOIN knowledge_child_bm25_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.bm25_text = knowledge_build_bm25_shadow_text(
     source.lexical_text,
     source.exact_terms
   )
   AND shadow.exact_terms =
     knowledge_normalize_bm25_shadow_terms(source.exact_terms)
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.child_ordinal = source.child_ordinal
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = active_generation_id
    AND source.search_profile_id = active_search_profile_id;

  IF verified_vector_count <> expected_count
    OR verified_bm25_count <> expected_count
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_BACKFILL_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    active_generation_id,
    active_search_profile_id,
    expected_count,
    verified_vector_count,
    verified_bm25_count;
END
$function$;

CREATE OR REPLACE FUNCTION knowledge_set_retrieval_profile(
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

  PERFORM pg_advisory_xact_lock(1296912978, 3);
  PERFORM pg_advisory_xact_lock(1296912978, 4);
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
    PERFORM * FROM knowledge_assert_pg17_retrieval_profile_ready();
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

CREATE OR REPLACE FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
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
  query_norm DOUBLE PRECISION;
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

  IF selected_profile = 'pg17_bm25_pgvector_v1' THEN
    IF p_query_embedding IS NULL
      OR cardinality(p_query_embedding) <> 1024
      OR array_position(p_query_embedding, NULL) IS NOT NULL
      OR EXISTS (
        SELECT 1
        FROM unnest(p_query_embedding) component
        WHERE component::TEXT IN ('NaN', 'Infinity', '-Infinity')
      )
    THEN
      RAISE EXCEPTION USING
        ERRCODE = '22023',
        MESSAGE = 'RAG_HYBRID_QUERY_EMBEDDING_INVALID';
    END IF;
    SELECT sqrt(sum(
      component::DOUBLE PRECISION * component::DOUBLE PRECISION
    )) INTO query_norm
    FROM unnest(p_query_embedding) component;
    IF query_norm IS NULL OR query_norm <= 0 THEN
      RAISE EXCEPTION USING
        ERRCODE = '22023',
        MESSAGE = 'RAG_HYBRID_QUERY_EMBEDDING_INVALID';
    END IF;

    RETURN QUERY
    SELECT
      candidate.collection_id,
      candidate.document_id,
      candidate.document_version_id,
      candidate.index_generation_id,
      candidate.materialization_id,
      candidate.parent_chunk_id,
      candidate.child_chunk_id,
      candidate.source_span_hash,
      candidate.content_hash,
      candidate.fused_score::REAL
    FROM knowledge_fetch_hybrid_shadow_diagnostics(
      p_collection_ids,
      p_query_text,
      p_query_embedding::VECTOR(1024),
      p_limit
    ) candidate
    ORDER BY candidate.fused_rank;
    RETURN;
  END IF;

  RAISE EXCEPTION USING
    ERRCODE = '55000',
    MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
END
$function$;

ALTER FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_assert_pg17_retrieval_profile_ready()
  TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_set_retrieval_profile(
  TEXT, TEXT, BIGINT, TEXT
) TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
) TO rag_worker_executor, go_api_runtime;

SELECT 'PASS G18.5B.1 profile router candidate' AS result;

-- Frozen from `mm-chat/ops/g18-profile-cutover/10-active-projection-maintenance.up.sql`.
DO $maintenance_prerequisite$
BEGIN
  IF to_regprocedure(
    'knowledge_assert_pg17_retrieval_profile_ready()'
  ) IS NULL OR to_regclass(
    'knowledge_child_vector_shadow_projections'
  ) IS NULL OR to_regclass(
    'knowledge_child_bm25_shadow_projections'
  ) IS NULL OR to_regclass(
    'knowledge_bm25_shadow_build_sources'
  ) IS NULL THEN
    RAISE EXCEPTION 'G18 active maintenance requires the PG17 profile candidate';
  END IF;
END
$maintenance_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_sync_pg17_retrieval_materialization(
  p_materialization_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  vector_inserted_count BIGINT,
  bm25_inserted_count BIGINT,
  verified_count BIGINT
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  expected_count BIGINT;
  vector_source_count BIGINT;
  inserted_vector_count BIGINT;
  inserted_bm25_count BIGINT;
  verified_vector_count BIGINT;
  verified_bm25_count BIGINT;
BEGIN
  IF p_materialization_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_RETRIEVAL_MATERIALIZATION_ARGUMENT_INVALID';
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 3);
  PERFORM pg_advisory_xact_lock(1296912978, 4);

  SELECT count(*) INTO expected_count
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.materialization_id = p_materialization_id;
  SELECT count(*) INTO vector_source_count
  FROM knowledge_pgvector_shadow_sources source
  WHERE source.materialization_id = p_materialization_id;
  IF expected_count < 1 OR vector_source_count <> expected_count THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_MATERIALIZATION_SOURCE_INCOMPLETE';
  END IF;

  INSERT INTO knowledge_child_vector_shadow_projections (
    child_chunk_id, parent_chunk_id, materialization_id,
    index_generation_id, collection_id, document_id, document_version_id,
    search_profile_id, embedding_model_id, embedding_dimensions,
    embedding_vector, embedding_vector_sha256, embedding_norm,
    source_span_hash, chunk_profile_hash, content_hash,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch
  )
  SELECT
    source.child_chunk_id,
    source.parent_chunk_id,
    source.materialization_id,
    source.index_generation_id,
    source.collection_id,
    source.document_id,
    source.document_version_id,
    source.search_profile_id,
    source.embedding_model_id,
    source.embedding_dimensions,
    source.embedding_vector::VECTOR(1024),
    source.embedding_vector_sha256,
    vector_norm(source.embedding_vector::VECTOR(1024)),
    source.source_span_hash,
    source.chunk_profile_hash,
    source.content_hash,
    source.collection_visibility_epoch,
    source.collection_processing_revision,
    source.document_visibility_epoch
  FROM knowledge_pgvector_shadow_sources source
  WHERE source.materialization_id = p_materialization_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS inserted_vector_count = ROW_COUNT;

  INSERT INTO knowledge_child_bm25_shadow_projections (
    child_chunk_id, parent_chunk_id, materialization_id,
    index_generation_id, collection_id, document_id, document_version_id,
    search_profile_id, bm25_text, exact_terms, source_span_hash,
    chunk_profile_hash, content_hash, child_ordinal,
    collection_visibility_epoch, collection_processing_revision,
    document_visibility_epoch
  )
  SELECT
    source.child_chunk_id,
    source.parent_chunk_id,
    source.materialization_id,
    source.index_generation_id,
    source.collection_id,
    source.document_id,
    source.document_version_id,
    source.search_profile_id,
    knowledge_build_bm25_shadow_text(
      source.lexical_text,
      source.exact_terms
    ),
    knowledge_normalize_bm25_shadow_terms(source.exact_terms),
    source.source_span_hash,
    source.chunk_profile_hash,
    source.content_hash,
    source.child_ordinal,
    source.collection_visibility_epoch,
    source.collection_processing_revision,
    source.document_visibility_epoch
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.materialization_id = p_materialization_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS inserted_bm25_count = ROW_COUNT;

  SELECT count(*) INTO verified_vector_count
  FROM knowledge_pgvector_shadow_sources source
  JOIN knowledge_child_vector_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.embedding_model_id = source.embedding_model_id
   AND shadow.embedding_dimensions = source.embedding_dimensions
   AND shadow.embedding_vector_sha256 = source.embedding_vector_sha256
   AND shadow.embedding_vector::REAL[] = source.embedding_vector
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.materialization_id = p_materialization_id;

  SELECT count(*) INTO verified_bm25_count
  FROM knowledge_bm25_shadow_build_sources source
  JOIN knowledge_child_bm25_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.bm25_text = knowledge_build_bm25_shadow_text(
     source.lexical_text,
     source.exact_terms
   )
   AND shadow.exact_terms =
     knowledge_normalize_bm25_shadow_terms(source.exact_terms)
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.child_ordinal = source.child_ordinal
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.materialization_id = p_materialization_id;

  IF verified_vector_count <> expected_count
    OR verified_bm25_count <> expected_count
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_MATERIALIZATION_SYNC_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    expected_count,
    inserted_vector_count,
    inserted_bm25_count,
    least(verified_vector_count, verified_bm25_count);
END
$function$;

CREATE FUNCTION knowledge_maintain_pg17_retrieval_on_head()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  selected_profile TEXT;
BEGIN
  IF TG_OP = 'UPDATE'
    AND OLD.active_materialization_id = NEW.active_materialization_id
  THEN
    RETURN NEW;
  END IF;

  SELECT profile.active_profile INTO selected_profile
  FROM knowledge_retrieval_profile_head profile
  WHERE profile.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;
  IF selected_profile = 'legacy' THEN
    RETURN NEW;
  END IF;
  IF selected_profile <> 'pg17_bm25_pgvector_v1' THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
  END IF;

  PERFORM * FROM knowledge_sync_pg17_retrieval_materialization(
    NEW.active_materialization_id
  );
  RETURN NEW;
END
$function$;

CREATE TRIGGER knowledge_document_projection_head_pg17_retrieval
AFTER INSERT OR UPDATE OF active_materialization_id
ON knowledge_document_projection_heads
FOR EACH ROW EXECUTE FUNCTION knowledge_maintain_pg17_retrieval_on_head();

ALTER FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_maintain_pg17_retrieval_on_head()
  OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_maintain_pg17_retrieval_on_head()
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_sync_pg17_retrieval_materialization(UUID)
  TO rag_replay_operator;

SELECT 'PASS G18.5B.2a active projection maintenance' AS result;

-- Frozen from `mm-chat/ops/g18-profile-cutover/15-generation-cutover-fence.up.sql`.
DO $generation_fence_prerequisite$
BEGIN
  IF to_regclass('knowledge_bm25_shadow_build_sources') IS NULL
    OR to_regclass('knowledge_pgvector_shadow_sources') IS NULL
    OR to_regprocedure(
      'knowledge_sync_pg17_retrieval_materialization(uuid)'
    ) IS NULL
  THEN
    RAISE EXCEPTION
      'G18 generation fence requires PG17 projection maintenance';
  END IF;
END
$generation_fence_prerequisite$;

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_assert_pg17_generation_ready(
  p_index_generation_id UUID
) RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  document_count BIGINT,
  eligible_count BIGINT,
  vector_count BIGINT,
  bm25_count BIGINT
)
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  selected_search_profile_id UUID;
  expected_document_count BIGINT;
  source_document_count BIGINT;
  expected_count BIGINT;
  paired_source_count BIGINT;
  verified_vector_count BIGINT;
  verified_bm25_count BIGINT;
BEGIN
  IF p_index_generation_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_RETRIEVAL_GENERATION_ARGUMENT_INVALID';
  END IF;

  SELECT profile.id INTO selected_search_profile_id
  FROM knowledge_index_generations generation
  JOIN knowledge_search_profiles profile
    ON profile.index_profile_id = generation.index_profile_id
   AND profile.provider_profile_id = 'mineru_jina_postgres_v1'
   AND profile.embedding_processor = 'jina'
   AND profile.embedding_model_id = 'jina-embeddings-v4'
   AND profile.embedding_dimensions = 1024
  WHERE generation.id = p_index_generation_id
    AND generation.status IN ('building', 'verified', 'active', 'retired');
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_GENERATION_PROFILE_MISMATCH';
  END IF;

  SELECT count(*) INTO expected_document_count
  FROM knowledge_documents document
  JOIN knowledge_collections collection
    ON collection.id = document.collection_id
   AND collection.deleted_at IS NULL
  JOIN knowledge_document_versions version
    ON version.id = document.current_version_id
   AND version.document_id = document.id
   AND version.status = 'active'
  JOIN files file
    ON file.id = version.file_id
   AND file.upload_status = 'available'
   AND file.deleted_at IS NULL
  WHERE document.status = 'active'
    AND document.deleted_at IS NULL;

  SELECT count(*), count(DISTINCT source.document_id)
  INTO expected_count, source_document_count
  FROM knowledge_bm25_shadow_build_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = selected_search_profile_id;

  SELECT count(*) INTO paired_source_count
  FROM knowledge_bm25_shadow_build_sources bm25
  JOIN knowledge_pgvector_shadow_sources vector_source
    ON vector_source.child_chunk_id = bm25.child_chunk_id
   AND vector_source.parent_chunk_id = bm25.parent_chunk_id
   AND vector_source.materialization_id = bm25.materialization_id
   AND vector_source.index_generation_id = bm25.index_generation_id
   AND vector_source.collection_id = bm25.collection_id
   AND vector_source.document_id = bm25.document_id
   AND vector_source.document_version_id = bm25.document_version_id
   AND vector_source.search_profile_id = bm25.search_profile_id
   AND vector_source.source_span_hash = bm25.source_span_hash
   AND vector_source.chunk_profile_hash = bm25.chunk_profile_hash
   AND vector_source.content_hash = bm25.content_hash
   AND vector_source.collection_visibility_epoch =
     bm25.collection_visibility_epoch
   AND vector_source.collection_processing_revision =
     bm25.collection_processing_revision
   AND vector_source.document_visibility_epoch =
     bm25.document_visibility_epoch
  WHERE bm25.index_generation_id = p_index_generation_id
    AND bm25.search_profile_id = selected_search_profile_id;

  SELECT count(*) INTO verified_vector_count
  FROM knowledge_pgvector_shadow_sources source
  JOIN knowledge_child_vector_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.embedding_model_id = source.embedding_model_id
   AND shadow.embedding_dimensions = source.embedding_dimensions
   AND shadow.embedding_vector_sha256 = source.embedding_vector_sha256
   AND shadow.embedding_vector::REAL[] = source.embedding_vector
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = selected_search_profile_id;

  SELECT count(*) INTO verified_bm25_count
  FROM knowledge_bm25_shadow_build_sources source
  JOIN knowledge_child_bm25_shadow_projections shadow
    ON shadow.child_chunk_id = source.child_chunk_id
   AND shadow.parent_chunk_id = source.parent_chunk_id
   AND shadow.materialization_id = source.materialization_id
   AND shadow.index_generation_id = source.index_generation_id
   AND shadow.collection_id = source.collection_id
   AND shadow.document_id = source.document_id
   AND shadow.document_version_id = source.document_version_id
   AND shadow.search_profile_id = source.search_profile_id
   AND shadow.bm25_text = knowledge_build_bm25_shadow_text(
     source.lexical_text,
     source.exact_terms
   )
   AND shadow.exact_terms =
     knowledge_normalize_bm25_shadow_terms(source.exact_terms)
   AND shadow.source_span_hash = source.source_span_hash
   AND shadow.chunk_profile_hash = source.chunk_profile_hash
   AND shadow.content_hash = source.content_hash
   AND shadow.child_ordinal = source.child_ordinal
   AND shadow.collection_visibility_epoch =
     source.collection_visibility_epoch
   AND shadow.collection_processing_revision =
     source.collection_processing_revision
   AND shadow.document_visibility_epoch = source.document_visibility_epoch
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = selected_search_profile_id;

  IF expected_document_count < 1
    OR source_document_count <> expected_document_count
    OR expected_count < expected_document_count
    OR paired_source_count <> expected_count
    OR verified_vector_count <> expected_count
    OR verified_bm25_count <> expected_count
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_GENERATION_BACKFILL_INCOMPLETE';
  END IF;

  RETURN QUERY SELECT
    p_index_generation_id,
    selected_search_profile_id,
    expected_document_count,
    expected_count,
    verified_vector_count,
    verified_bm25_count;
END
$function$;

CREATE FUNCTION knowledge_fence_pg17_generation_cutover()
RETURNS trigger
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  selected_profile TEXT;
BEGIN
  IF OLD.active_index_generation_id IS NOT DISTINCT FROM
      NEW.active_index_generation_id
  THEN
    RETURN NEW;
  END IF;

  PERFORM pg_advisory_xact_lock(1296912978, 3);
  PERFORM pg_advisory_xact_lock(1296912978, 4);

  SELECT profile.active_profile INTO selected_profile
  FROM knowledge_retrieval_profile_head profile
  WHERE profile.singleton_id = 1;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = 'P0001',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_HEAD_MISSING';
  END IF;
  IF selected_profile = 'legacy' THEN
    RETURN NEW;
  END IF;
  IF selected_profile <> 'pg17_bm25_pgvector_v1' THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_RETRIEVAL_PROFILE_UNAVAILABLE';
  END IF;

  PERFORM * FROM knowledge_assert_pg17_generation_ready(
    NEW.active_index_generation_id
  );
  RETURN NEW;
END
$function$;

CREATE TRIGGER knowledge_corpus_head_pg17_retrieval_fence
BEFORE UPDATE OF active_index_generation_id
ON knowledge_corpus_projection_head
FOR EACH ROW EXECUTE FUNCTION knowledge_fence_pg17_generation_cutover();

ALTER FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_fence_pg17_generation_cutover()
  OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fence_pg17_generation_cutover()
  FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_assert_pg17_generation_ready(UUID)
  TO rag_replay_operator;

SELECT 'PASS G18.5B.2b generation cutover fence' AS result;
