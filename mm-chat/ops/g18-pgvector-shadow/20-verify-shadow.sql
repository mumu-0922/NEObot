\set ON_ERROR_STOP on

DO $backfill_contract$
DECLARE
  first_run RECORD;
  second_run RECORD;
BEGIN
  SELECT * INTO first_run
  FROM knowledge_backfill_pgvector_shadow(
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  IF first_run.eligible_count <> 4
    OR first_run.inserted_count <> 4
    OR first_run.verified_shadow_count <> 4
  THEN
    RAISE EXCEPTION 'unexpected first backfill result: %', first_run;
  END IF;

  SELECT * INTO second_run
  FROM knowledge_backfill_pgvector_shadow(
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  IF second_run.eligible_count <> 4
    OR second_run.inserted_count <> 0
    OR second_run.verified_shadow_count <> 4
  THEN
    RAISE EXCEPTION 'unexpected idempotent backfill result: %', second_run;
  END IF;
END
$backfill_contract$;

DO $insert_guard_contract$
BEGIN
  BEGIN
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
      shadow.child_chunk_id,
      shadow.parent_chunk_id,
      shadow.materialization_id,
      shadow.index_generation_id,
      shadow.collection_id,
      shadow.document_id,
      shadow.document_version_id,
      shadow.search_profile_id,
      shadow.embedding_model_id,
      shadow.embedding_dimensions,
      shadow.embedding_vector,
      repeat('f', 64),
      shadow.embedding_norm,
      shadow.source_span_hash,
      shadow.chunk_profile_hash,
      shadow.content_hash,
      shadow.collection_visibility_epoch,
      shadow.collection_processing_revision,
      shadow.document_visibility_epoch
    FROM knowledge_child_vector_shadow_projections shadow
    WHERE shadow.child_chunk_id =
      '18180000-0000-0000-0000-000000000013';
    RAISE EXCEPTION 'mismatched hash insert was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN
    IF SQLERRM <> 'RAG_PGVECTOR_SHADOW_SOURCE_MISMATCH' THEN
      RAISE;
    END IF;
  END;
  IF (SELECT count(*) FROM knowledge_child_vector_shadow_projections) <> 4 THEN
    RAISE EXCEPTION 'failed direct insert changed shadow rows';
  END IF;
END
$insert_guard_contract$;

DO $projection_contract$
DECLARE
  mismatch_count BIGINT;
  exact_order UUID[];
  approximate_order UUID[];
  expected_collection_order CONSTANT UUID[] := ARRAY[
    '18180000-0000-0000-0000-000000000013'::UUID,
    '18300000-0000-0000-0000-000000000001'::UUID,
    '18300000-0000-0000-0000-000000000002'::UUID
  ];
  expected_global_order CONSTANT UUID[] := ARRAY[
    '18180000-0000-0000-0000-000000000013'::UUID,
    '18300000-0000-0000-0000-000000000017'::UUID,
    '18300000-0000-0000-0000-000000000001'::UUID,
    '18300000-0000-0000-0000-000000000002'::UUID
  ];
BEGIN
  IF (SELECT count(*) FROM knowledge_child_vector_shadow_projections) <> 4 THEN
    RAISE EXCEPTION 'shadow count is not 4';
  END IF;

  SELECT count(*) INTO mismatch_count
  FROM knowledge_pgvector_shadow_sources source
  FULL JOIN knowledge_child_vector_shadow_projections shadow
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
  WHERE source.index_generation_id IS NULL
    OR shadow.index_generation_id IS NULL
    OR abs(shadow.embedding_norm - vector_norm(shadow.embedding_vector)) >
      0.000001;
  IF mismatch_count <> 0 THEN
    RAISE EXCEPTION 'shadow identity/hash/norm mismatches = %', mismatch_count;
  END IF;

  PERFORM set_config('enable_indexscan', 'off', true);
  SELECT array_agg(ranked.child_chunk_id ORDER BY ranked.distance, ranked.child_chunk_id)
  INTO exact_order
  FROM (
    SELECT
      shadow.child_chunk_id,
      shadow.embedding_vector <=>
        (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024)
        AS distance
    FROM knowledge_child_vector_shadow_projections shadow
    JOIN knowledge_pgvector_shadow_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
    WHERE source.collection_id =
      '18180000-0000-0000-0000-000000000003'
    ORDER BY distance, shadow.child_chunk_id
    LIMIT 3
  ) ranked;
  IF exact_order IS DISTINCT FROM expected_collection_order THEN
    RAISE EXCEPTION 'exact collection order = %, expected %',
      exact_order, expected_collection_order;
  END IF;

  PERFORM set_config('enable_indexscan', 'on', true);
  PERFORM set_config('enable_seqscan', 'off', true);
  PERFORM set_config('hnsw.ef_search', '100', true);
  SELECT array_agg(ranked.child_chunk_id ORDER BY ranked.distance, ranked.child_chunk_id)
  INTO approximate_order
  FROM (
    SELECT
      shadow.child_chunk_id,
      shadow.embedding_vector <=>
        (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024)
        AS distance
    FROM knowledge_child_vector_shadow_projections shadow
    ORDER BY distance, shadow.child_chunk_id
    LIMIT 4
  ) ranked;
  IF approximate_order IS DISTINCT FROM expected_global_order THEN
    RAISE EXCEPTION 'HNSW order = %, expected %',
      approximate_order, expected_global_order;
  END IF;

  PERFORM set_config('enable_seqscan', 'on', true);
  IF (
    SELECT count(*)
    FROM knowledge_child_vector_shadow_projections shadow
    JOIN knowledge_pgvector_shadow_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
    WHERE source.collection_id =
      '18300000-0000-0000-0000-000000000012'
  ) <> 1 THEN
    RAISE EXCEPTION 'other collection ownership slice is not isolated';
  END IF;
  IF (
    SELECT count(*)
    FROM knowledge_child_vector_shadow_projections shadow
    JOIN knowledge_pgvector_shadow_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
    WHERE source.collection_id =
      '18300000-0000-0000-0000-000000000099'
  ) <> 0 THEN
    RAISE EXCEPTION 'unselected collection leaked candidates';
  END IF;

  IF has_table_privilege(
    'go_api_runtime',
    'knowledge_child_vector_shadow_projections',
    'SELECT'
  ) OR has_table_privilege(
    'rag_worker_executor',
    'knowledge_child_vector_shadow_projections',
    'SELECT'
  ) OR has_function_privilege(
    'go_api_runtime',
    'knowledge_backfill_pgvector_shadow(uuid,uuid)',
    'EXECUTE'
  ) OR has_function_privilege(
    'rag_worker_executor',
    'knowledge_backfill_pgvector_shadow(uuid,uuid)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'production runtime received shadow privileges';
  END IF;
  IF NOT has_function_privilege(
    'rag_replay_operator',
    'knowledge_backfill_pgvector_shadow(uuid,uuid)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'controlled replay operator lacks backfill privilege';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM pg_proc function
    WHERE function.oid IN (
      'knowledge_validate_pgvector_shadow_insert()'::REGPROCEDURE,
      'knowledge_backfill_pgvector_shadow(uuid,uuid)'::REGPROCEDURE
    )
      AND NOT EXISTS (
        SELECT 1
        FROM unnest(function.proconfig) setting
        WHERE setting LIKE 'search_path=%pg_catalog%pg_temp'
          AND setting NOT LIKE '%$user%'
      )
  ) THEN
    RAISE EXCEPTION 'shadow SECURITY DEFINER search_path is not hardened';
  END IF;
END
$projection_contract$;

CREATE TEMP TABLE g18_pgvector_golden_cases (
  case_id TEXT PRIMARY KEY,
  query_embedding VECTOR(1024) NOT NULL,
  collection_ids UUID[] NOT NULL,
  expected_child_ids UUID[] NOT NULL
);

INSERT INTO g18_pgvector_golden_cases (
  case_id, query_embedding, collection_ids, expected_child_ids
) VALUES
  (
    'exact-error-identifier',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    ARRAY[
      '18180000-0000-0000-0000-000000000013'::UUID,
      '18300000-0000-0000-0000-000000000001'::UUID
    ]
  ),
  (
    'zh-lexical-retry',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    ARRAY[
      '18180000-0000-0000-0000-000000000013'::UUID,
      '18300000-0000-0000-0000-000000000001'::UUID
    ]
  ),
  (
    'zh-semantic-retry',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    ARRAY[
      '18180000-0000-0000-0000-000000000013'::UUID,
      '18300000-0000-0000-0000-000000000001'::UUID
    ]
  ),
  (
    'context-follow-up',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    ARRAY[
      '18180000-0000-0000-0000-000000000013'::UUID,
      '18300000-0000-0000-0000-000000000001'::UUID
    ]
  ),
  (
    'cross-collection-retention',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY[
      '18180000-0000-0000-0000-000000000003'::UUID,
      '18300000-0000-0000-0000-000000000012'::UUID
    ],
    ARRAY[
      '18180000-0000-0000-0000-000000000013'::UUID,
      '18300000-0000-0000-0000-000000000017'::UUID,
      '18300000-0000-0000-0000-000000000001'::UUID
    ]
  ),
  (
    'negative-weather',
    (ARRAY[0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1021]))::VECTOR(1024),
    ARRAY[
      '18180000-0000-0000-0000-000000000003'::UUID,
      '18300000-0000-0000-0000-000000000012'::UUID
    ],
    ARRAY[]::UUID[]
  ),
  (
    'negative-cooking',
    (ARRAY[0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1020]))::VECTOR(1024),
    ARRAY[
      '18180000-0000-0000-0000-000000000003'::UUID,
      '18300000-0000-0000-0000-000000000012'::UUID
    ],
    ARRAY[]::UUID[]
  );

DO $golden_vector_contract$
DECLARE
  golden_case RECORD;
  exact_ids UUID[];
  approximate_ids UUID[];
BEGIN
  FOR golden_case IN
    SELECT * FROM g18_pgvector_golden_cases ORDER BY case_id
  LOOP
    PERFORM set_config('enable_indexscan', 'off', true);
    SELECT COALESCE(
      array_agg(ranked.child_chunk_id ORDER BY ranked.distance, ranked.child_chunk_id),
      ARRAY[]::UUID[]
    ) INTO exact_ids
    FROM (
      SELECT
        shadow.child_chunk_id,
        shadow.embedding_vector <=> golden_case.query_embedding AS distance
      FROM knowledge_child_vector_shadow_projections shadow
      JOIN knowledge_pgvector_shadow_sources source
        ON source.child_chunk_id = shadow.child_chunk_id
      WHERE source.collection_id = ANY(golden_case.collection_ids)
        AND 1 - (shadow.embedding_vector <=> golden_case.query_embedding) >=
          0.48
      ORDER BY distance, shadow.child_chunk_id
      LIMIT 5
    ) ranked;

    PERFORM set_config('enable_indexscan', 'on', true);
    PERFORM set_config('enable_seqscan', 'off', true);
    PERFORM set_config('hnsw.ef_search', '100', true);
    SELECT COALESCE(
      array_agg(ranked.child_chunk_id ORDER BY ranked.distance, ranked.child_chunk_id),
      ARRAY[]::UUID[]
    ) INTO approximate_ids
    FROM (
      SELECT
        shadow.child_chunk_id,
        shadow.embedding_vector <=> golden_case.query_embedding AS distance
      FROM knowledge_child_vector_shadow_projections shadow
      JOIN knowledge_pgvector_shadow_sources source
        ON source.child_chunk_id = shadow.child_chunk_id
      WHERE source.collection_id = ANY(golden_case.collection_ids)
        AND 1 - (shadow.embedding_vector <=> golden_case.query_embedding) >=
          0.48
      ORDER BY distance, shadow.child_chunk_id
      LIMIT 5
    ) ranked;

    IF exact_ids IS DISTINCT FROM golden_case.expected_child_ids
      OR approximate_ids IS DISTINCT FROM golden_case.expected_child_ids
    THEN
      RAISE EXCEPTION 'Golden case % exact=% hnsw=% expected=%',
        golden_case.case_id,
        exact_ids,
        approximate_ids,
        golden_case.expected_child_ids;
    END IF;
  END LOOP;
END
$golden_vector_contract$;

DROP TABLE g18_pgvector_golden_cases;

SET enable_seqscan = off;
SET hnsw.ef_search = 100;
SET plan_cache_mode = force_generic_plan;
PREPARE g18_hnsw_plan (VECTOR(1024)) AS
SELECT child_chunk_id
FROM knowledge_child_vector_shadow_projections
ORDER BY embedding_vector <=> $1
LIMIT 4;
EXPLAIN (COSTS OFF)
EXECUTE g18_hnsw_plan(
  (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024)
);
DEALLOCATE g18_hnsw_plan;
RESET enable_seqscan;
RESET hnsw.ef_search;
RESET plan_cache_mode;

INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
) VALUES (
  '18300000-0000-0000-0000-000000000030',
  '18180000-0000-0000-0000-000000000012',
  '18180000-0000-0000-0000-000000000010',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  3,
  repeat('d', 64),
  repeat('8', 64),
  repeat('8', 64),
  'Synthetic zero-vector rejection candidate.',
  5
);

INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
) VALUES (
  '18300000-0000-0000-0000-000000000030',
  '18180000-0000-0000-0000-000000000012',
  '18180000-0000-0000-0000-000000000010',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000003',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  '18180000-0000-0000-0000-000000000011',
  'jina-embeddings-v4',
  1024,
  array_fill(0::REAL, ARRAY[1024]),
  repeat('8', 64),
  'Synthetic zero-vector rejection candidate.',
  ARRAY['G18_ZERO']::TEXT[],
  repeat('8', 64),
  repeat('d', 64),
  repeat('8', 64),
  '{"kind":"page","page":4}'::JSONB,
  'ready',
  clock_timestamp()
);

DO $zero_vector_rejection$
BEGIN
  BEGIN
    PERFORM * FROM knowledge_backfill_pgvector_shadow(
      '18180000-0000-0000-0000-000000000007',
      '18180000-0000-0000-0000-000000000011'
    );
    RAISE EXCEPTION 'zero vector was accepted';
  EXCEPTION WHEN SQLSTATE '22023' THEN
    IF SQLERRM <> 'RAG_PGVECTOR_SHADOW_SOURCE_INVALID' THEN
      RAISE;
    END IF;
  END;
  IF (SELECT count(*) FROM knowledge_child_vector_shadow_projections) <> 4 THEN
    RAISE EXCEPTION 'failed zero-vector backfill changed shadow rows';
  END IF;
END
$zero_vector_rejection$;

UPDATE knowledge_child_search_projections
SET status = 'purged', purged_at = clock_timestamp()
WHERE child_chunk_id = '18300000-0000-0000-0000-000000000030';

INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
) VALUES (
  '18300000-0000-0000-0000-000000000031',
  '18180000-0000-0000-0000-000000000012',
  '18180000-0000-0000-0000-000000000010',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  4,
  repeat('d', 64),
  repeat('9', 64),
  repeat('9', 64),
  'Synthetic non-finite rejection candidate.',
  5
);

INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
) VALUES (
  '18300000-0000-0000-0000-000000000031',
  '18180000-0000-0000-0000-000000000012',
  '18180000-0000-0000-0000-000000000010',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000003',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  '18180000-0000-0000-0000-000000000011',
  'jina-embeddings-v4',
  1024,
  ARRAY['NaN'::REAL] || array_fill(0::REAL, ARRAY[1023]),
  repeat('9', 64),
  'Synthetic non-finite rejection candidate.',
  ARRAY['G18_NONFINITE']::TEXT[],
  repeat('9', 64),
  repeat('d', 64),
  repeat('9', 64),
  '{"kind":"page","page":5}'::JSONB,
  'ready',
  clock_timestamp()
);

DO $non_finite_rejection$
BEGIN
  BEGIN
    PERFORM * FROM knowledge_backfill_pgvector_shadow(
      '18180000-0000-0000-0000-000000000007',
      '18180000-0000-0000-0000-000000000011'
    );
    RAISE EXCEPTION 'non-finite vector was accepted';
  EXCEPTION WHEN SQLSTATE '22023' THEN
    IF SQLERRM <> 'RAG_PGVECTOR_SHADOW_SOURCE_INVALID' THEN
      RAISE;
    END IF;
  END;
  IF (SELECT count(*) FROM knowledge_child_vector_shadow_projections) <> 4 THEN
    RAISE EXCEPTION 'failed non-finite backfill changed shadow rows';
  END IF;
END
$non_finite_rejection$;

UPDATE knowledge_child_search_projections
SET status = 'purged', purged_at = clock_timestamp()
WHERE child_chunk_id = '18300000-0000-0000-0000-000000000031';

UPDATE knowledge_documents
SET status = 'tombstoned',
    visibility_epoch = visibility_epoch + 1,
    deleted_at = clock_timestamp(),
    updated_at = clock_timestamp()
WHERE id = '18180000-0000-0000-0000-000000000004';

DO $deletion_contract$
DECLARE
  legacy_candidate_count BIGINT;
BEGIN
  IF (
    SELECT count(*)
    FROM knowledge_child_vector_shadow_projections shadow
    JOIN knowledge_pgvector_shadow_sources source
      ON source.child_chunk_id = shadow.child_chunk_id
    WHERE source.collection_id =
      '18180000-0000-0000-0000-000000000003'
  ) <> 0 THEN
    RAISE EXCEPTION 'tombstoned collection remains shadow-visible';
  END IF;
  IF (
    SELECT count(*)
    FROM knowledge_child_vector_shadow_projections
    WHERE collection_id = '18180000-0000-0000-0000-000000000003'
  ) <> 3 THEN
    RAISE EXCEPTION 'deletion destroyed rollback shadow rows';
  END IF;
  IF (
    SELECT count(*)
    FROM knowledge_child_search_projections
    WHERE collection_id = '18180000-0000-0000-0000-000000000003'
      AND status = 'ready'
      AND embedding_vector IS NOT NULL
  ) <> 3 THEN
    RAISE EXCEPTION 'deletion changed legacy REAL[] rollback rows';
  END IF;

  SELECT count(*) INTO legacy_candidate_count
  FROM knowledge_fetch_hybrid_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'synthetic semantic query',
    ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]),
    10
  );
  IF legacy_candidate_count <> 0 THEN
    RAISE EXCEPTION 'legacy production reader leaked tombstoned evidence';
  END IF;
END
$deletion_contract$;

SELECT
  'PASS G18.3 golden=7 backfill=4 exact/hnsw=parity acl=2 deletion=hidden invalid=rollback' AS result;
