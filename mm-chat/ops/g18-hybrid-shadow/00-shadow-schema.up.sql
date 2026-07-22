\set ON_ERROR_STOP on

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

CREATE VIEW knowledge_bm25_shadow_sources
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
  search.lexical_text,
  search.exact_terms,
  search.source_span_hash,
  search.chunk_profile_hash,
  search.content_hash,
  child.ordinal AS child_ordinal,
  materialization.collection_visibility_epoch,
  materialization.collection_processing_revision,
  materialization.document_visibility_epoch
FROM knowledge_corpus_projection_head corpus
JOIN knowledge_child_search_projections search
  ON search.index_generation_id = corpus.active_index_generation_id
JOIN knowledge_search_profiles search_profile
  ON search_profile.id = search.search_profile_id
 AND search_profile.embedding_processor = 'jina'
 AND search_profile.embedding_model_id = 'jina-embeddings-v4'
 AND search_profile.embedding_dimensions = 1024
JOIN knowledge_index_generations generation
  ON generation.id = search.index_generation_id
 AND generation.index_profile_id = search_profile.index_profile_id
 AND generation.status = 'active'
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
WHERE corpus.singleton_id = 1
  AND search.status = 'ready'
  AND search.embedding_model_id = 'jina-embeddings-v4'
  AND search.embedding_dimensions = 1024
  AND search.embedding_vector IS NOT NULL
  AND search.embedding_vector_sha256 IS NOT NULL
  AND cardinality(search.embedding_vector) = 1024;

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
  source knowledge_bm25_shadow_sources%ROWTYPE;
  expected_terms TEXT[];
  expected_text TEXT;
BEGIN
  SELECT candidate.* INTO source
  FROM knowledge_bm25_shadow_sources candidate
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
  FROM knowledge_corpus_projection_head corpus
  JOIN knowledge_index_generations generation
    ON generation.id = corpus.active_index_generation_id
  JOIN knowledge_search_profiles profile
    ON profile.id = p_search_profile_id
   AND profile.index_profile_id = generation.index_profile_id
  WHERE corpus.singleton_id = 1
    AND generation.id = p_index_generation_id
    AND generation.status = 'active';
  IF profile_matches <> 1 OR generation_matches <> 1 THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_BM25_SHADOW_PROFILE_MISMATCH';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM knowledge_child_bm25_shadow_projections shadow
    JOIN knowledge_bm25_shadow_sources source
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
  FROM knowledge_bm25_shadow_sources source
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
  FROM knowledge_bm25_shadow_sources source
  WHERE source.index_generation_id = p_index_generation_id
    AND source.search_profile_id = p_search_profile_id
  ORDER BY source.child_chunk_id
  ON CONFLICT (child_chunk_id) DO NOTHING;
  GET DIAGNOSTICS v_inserted_count = ROW_COUNT;

  SELECT count(*) INTO v_verified_shadow_count
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
    JOIN knowledge_bm25_shadow_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
     AND source.index_generation_id = shadow.index_generation_id
     AND source.search_profile_id = shadow.search_profile_id
     AND source.content_hash = shadow.content_hash
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
      1 - (shadow.embedding_vector <=> p_query_embedding) AS similarity,
      source.child_ordinal
    FROM knowledge_child_vector_shadow_projections shadow
    JOIN knowledge_bm25_shadow_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
     AND source.index_generation_id = shadow.index_generation_id
     AND source.search_profile_id = shadow.search_profile_id
     AND source.content_hash = shadow.content_hash
    JOIN selected_collection selected
      ON selected.id = source.collection_id
    WHERE char_length(normalized_query) >= minimum_dense_query_characters
      AND 1 - (shadow.embedding_vector <=> p_query_embedding) >=
        minimum_dense_similarity
    ORDER BY
      shadow.embedding_vector <=> p_query_embedding,
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
REVOKE ALL ON knowledge_bm25_shadow_sources FROM PUBLIC;
REVOKE ALL ON knowledge_child_bm25_shadow_projections FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_validate_bm25_shadow_insert()
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_backfill_bm25_shadow(UUID, UUID)
  FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) FROM PUBLIC;

GRANT SELECT ON knowledge_bm25_shadow_sources TO rag_projection_owner;
GRANT SELECT, INSERT ON knowledge_child_bm25_shadow_projections
  TO rag_projection_owner;
GRANT EXECUTE ON FUNCTION knowledge_backfill_bm25_shadow(UUID, UUID)
  TO rag_replay_operator;
GRANT EXECUTE ON FUNCTION knowledge_fetch_hybrid_shadow_diagnostics(
  UUID[], TEXT, VECTOR, INTEGER
) TO rag_replay_operator;

SELECT 'PASS G18.4 BM25 hybrid shadow schema' AS result;
