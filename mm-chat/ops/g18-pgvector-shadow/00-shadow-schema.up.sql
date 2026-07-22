\set ON_ERROR_STOP on

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
