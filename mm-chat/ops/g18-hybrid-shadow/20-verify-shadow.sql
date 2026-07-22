\set ON_ERROR_STOP on

DO $backfill_contract$
DECLARE
  first_run RECORD;
  second_run RECORD;
BEGIN
  SELECT * INTO first_run
  FROM knowledge_backfill_bm25_shadow(
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  IF first_run.eligible_count <> 4
    OR first_run.inserted_count <> 4
    OR first_run.verified_shadow_count <> 4
  THEN
    RAISE EXCEPTION 'unexpected first BM25 backfill result: %', first_run;
  END IF;

  SELECT * INTO second_run
  FROM knowledge_backfill_bm25_shadow(
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  IF second_run.eligible_count <> 4
    OR second_run.inserted_count <> 0
    OR second_run.verified_shadow_count <> 4
  THEN
    RAISE EXCEPTION 'unexpected idempotent BM25 backfill result: %', second_run;
  END IF;
END
$backfill_contract$;

DO $insert_guard_contract$
BEGIN
  BEGIN
    INSERT INTO knowledge_child_bm25_shadow_projections (
      child_chunk_id, parent_chunk_id, materialization_id,
      index_generation_id, collection_id, document_id, document_version_id,
      search_profile_id, bm25_text, exact_terms, source_span_hash,
      chunk_profile_hash, content_hash, child_ordinal,
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
      shadow.bm25_text || ' forged',
      shadow.exact_terms,
      shadow.source_span_hash,
      shadow.chunk_profile_hash,
      shadow.content_hash,
      shadow.child_ordinal,
      shadow.collection_visibility_epoch,
      shadow.collection_processing_revision,
      shadow.document_visibility_epoch
    FROM knowledge_child_bm25_shadow_projections shadow
    WHERE shadow.child_chunk_id =
      '18180000-0000-0000-0000-000000000013';
    RAISE EXCEPTION 'forged BM25 text insert was accepted';
  EXCEPTION WHEN SQLSTATE 'P0001' THEN
    IF SQLERRM <> 'RAG_BM25_SHADOW_CONTENT_MISMATCH' THEN
      RAISE;
    END IF;
  END;
  IF (SELECT count(*) FROM knowledge_child_bm25_shadow_projections) <> 4 THEN
    RAISE EXCEPTION 'failed direct insert changed BM25 shadow rows';
  END IF;
END
$insert_guard_contract$;

DO $schema_security_contract$
DECLARE
  mismatch_count BIGINT;
BEGIN
  SELECT count(*) INTO mismatch_count
  FROM knowledge_bm25_shadow_sources source
  FULL JOIN knowledge_child_bm25_shadow_projections shadow
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
  WHERE source.child_chunk_id IS NULL OR shadow.child_chunk_id IS NULL;
  IF mismatch_count <> 0 THEN
    RAISE EXCEPTION 'BM25 identity/content mismatches = %', mismatch_count;
  END IF;

  IF has_table_privilege(
    'go_api_runtime',
    'knowledge_child_bm25_shadow_projections',
    'SELECT'
  ) OR has_table_privilege(
    'rag_worker_executor',
    'knowledge_child_bm25_shadow_projections',
    'SELECT'
  ) OR has_function_privilege(
    'go_api_runtime',
    'knowledge_backfill_bm25_shadow(uuid,uuid)',
    'EXECUTE'
  ) OR has_function_privilege(
    'rag_worker_executor',
    'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'production runtime received hybrid shadow privileges';
  END IF;
  IF NOT has_function_privilege(
    'rag_replay_operator',
    'knowledge_backfill_bm25_shadow(uuid,uuid)',
    'EXECUTE'
  ) OR NOT has_function_privilege(
    'rag_replay_operator',
    'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'controlled replay operator lacks shadow privileges';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM pg_proc function
    WHERE function.oid IN (
      'knowledge_validate_bm25_shadow_insert()'::REGPROCEDURE,
      'knowledge_backfill_bm25_shadow(uuid,uuid)'::REGPROCEDURE,
      'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)'::REGPROCEDURE
    )
      AND NOT EXISTS (
        SELECT 1
        FROM unnest(function.proconfig) setting
        WHERE setting LIKE 'search_path=%pg_catalog%pg_temp'
          AND setting NOT LIKE '%$user%'
      )
  ) THEN
    RAISE EXCEPTION 'hybrid SECURITY DEFINER search_path is not hardened';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM pg_proc function
    CROSS JOIN LATERAL unnest(function.proargnames) argument(name)
    WHERE function.oid =
      'knowledge_fetch_hybrid_shadow_diagnostics(uuid[],text,vector,integer)'::REGPROCEDURE
      AND argument.name IN (
        'lexical_text', 'bm25_text', 'source_text', 'exact_terms'
      )
  ) THEN
    RAISE EXCEPTION 'shadow diagnostics expose private source text';
  END IF;
END
$schema_security_contract$;

SET ROLE rag_replay_operator;
DO $operator_execution_contract$
DECLARE
  replay_result RECORD;
  diagnostic_count BIGINT;
BEGIN
  SELECT * INTO replay_result
  FROM knowledge_backfill_bm25_shadow(
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  IF replay_result.eligible_count <> 4
    OR replay_result.inserted_count <> 0
    OR replay_result.verified_shadow_count <> 4
  THEN
    RAISE EXCEPTION 'operator replay result: %', replay_result;
  END IF;

  SELECT count(*) INTO diagnostic_count
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'ERR_CONN_RESET',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  );
  IF diagnostic_count = 0 THEN
    RAISE EXCEPTION 'controlled operator diagnostic returned no candidates';
  END IF;
END
$operator_execution_contract$;
RESET ROLE;

DO $lexical_dense_contract$
DECLARE
  winner UUID;
  winner_bm25_rank INTEGER;
  winner_bm25_score DOUBLE PRECISION;
  winner_dense_rank INTEGER;
  result_count BIGINT;
BEGIN
  SELECT child_chunk_id, bm25_rank, bm25_score
  INTO winner, winner_bm25_rank, winner_bm25_score
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'ERR_CONN_RESET',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  )
  ORDER BY fused_rank
  LIMIT 1;
  IF winner <> '18180000-0000-0000-0000-000000000013'
    OR winner_bm25_rank <> 1
    OR winner_bm25_score >= 0
  THEN
    RAISE EXCEPTION 'identifier winner=% rank=% score=%',
      winner, winner_bm25_rank, winner_bm25_score;
  END IF;

  SELECT child_chunk_id INTO winner
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    '/api/v1/jobs',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  )
  ORDER BY fused_rank
  LIMIT 1;
  IF winner <> '18180000-0000-0000-0000-000000000013' THEN
    RAISE EXCEPTION 'path winner = %', winner;
  END IF;

  SELECT child_chunk_id INTO winner
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'bounded exponential backoff',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  )
  ORDER BY fused_rank
  LIMIT 1;
  IF winner <> '18180000-0000-0000-0000-000000000013' THEN
    RAISE EXCEPTION 'phrase winner = %', winner;
  END IF;

  SELECT child_chunk_id, bm25_rank
  INTO winner, winner_bm25_rank
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    '重试策略',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  )
  ORDER BY fused_rank
  LIMIT 1;
  IF winner <> '18180000-0000-0000-0000-000000000013'
    OR winner_bm25_rank <> 1
  THEN
    RAISE EXCEPTION 'Chinese lexical winner=% rank=%',
      winner, winner_bm25_rank;
  END IF;

  SELECT count(*) INTO result_count
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    '怎么重试',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  ) diagnostic
  WHERE diagnostic.child_chunk_id =
    '18180000-0000-0000-0000-000000000013'
    AND diagnostic.bm25_rank IS NOT NULL;
  IF result_count <> 1 THEN
    RAISE EXCEPTION 'bounded CJK bigram recall failed';
  END IF;

  SELECT child_chunk_id, dense_rank
  INTO winner, winner_dense_rank
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    '服务暂时中断该怎么办',
    (ARRAY[0.8::REAL, 0.6::REAL] ||
      array_fill(0::REAL, ARRAY[1022]))::VECTOR(1024),
    5
  )
  ORDER BY fused_rank
  LIMIT 1;
  IF winner <> '18300000-0000-0000-0000-000000000001'
    OR winner_dense_rank <> 1
  THEN
    RAISE EXCEPTION 'semantic winner=% dense_rank=%',
      winner, winner_dense_rank;
  END IF;

  SELECT count(*) INTO result_count
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'RETENTION_BLUE retention policy',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  ) diagnostic
  WHERE diagnostic.collection_id =
    '18300000-0000-0000-0000-000000000012';
  IF result_count <> 0 THEN
    RAISE EXCEPTION 'unselected collection leaked hybrid diagnostics';
  END IF;
END
$lexical_dense_contract$;

CREATE TEMP TABLE g18_hybrid_golden_cases (
  case_id TEXT PRIMARY KEY,
  query_text TEXT NOT NULL,
  query_embedding VECTOR(1024) NOT NULL,
  collection_ids UUID[] NOT NULL,
  expected_top_child_id UUID,
  expect_no_evidence BOOLEAN NOT NULL DEFAULT false
);

INSERT INTO g18_hybrid_golden_cases (
  case_id, query_text, query_embedding, collection_ids,
  expected_top_child_id, expect_no_evidence
) VALUES
  (
    'exact-error-identifier',
    'ERR_CONN_RESET /api/v1/jobs',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    '18180000-0000-0000-0000-000000000013',
    false
  ),
  (
    'zh-lexical-retry',
    '重试策略',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    '18180000-0000-0000-0000-000000000013',
    false
  ),
  (
    'zh-semantic-retry',
    '服务暂时中断该怎么办',
    (ARRAY[0.8::REAL, 0.6::REAL] ||
      array_fill(0::REAL, ARRAY[1022]))::VECTOR(1024),
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    '18300000-0000-0000-0000-000000000001',
    false
  ),
  (
    'context-follow-up',
    'RETENTION_BLUE',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY[
      '18180000-0000-0000-0000-000000000003'::UUID,
      '18300000-0000-0000-0000-000000000012'::UUID
    ],
    '18180000-0000-0000-0000-000000000013',
    false
  ),
  (
    'cross-collection-retention',
    'RETENTION_BLUE retention policy',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    ARRAY[
      '18180000-0000-0000-0000-000000000003'::UUID,
      '18300000-0000-0000-0000-000000000012'::UUID
    ],
    '18180000-0000-0000-0000-000000000013',
    false
  ),
  (
    'negative-weather',
    '杭州明天会下雨吗？',
    (ARRAY[0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1021]))::VECTOR(1024),
    ARRAY[
      '18180000-0000-0000-0000-000000000003'::UUID,
      '18300000-0000-0000-0000-000000000012'::UUID
    ],
    NULL,
    true
  ),
  (
    'negative-cooking',
    'How long should fresh pasta boil?',
    (ARRAY[0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1020]))::VECTOR(1024),
    ARRAY[
      '18180000-0000-0000-0000-000000000003'::UUID,
      '18300000-0000-0000-0000-000000000012'::UUID
    ],
    NULL,
    true
  );

DO $golden_hybrid_contract$
DECLARE
  golden_case RECORD;
  result_count BIGINT;
  top_child_id UUID;
BEGIN
  FOR golden_case IN
    SELECT * FROM g18_hybrid_golden_cases ORDER BY case_id
  LOOP
    SELECT count(*), (array_agg(
      diagnostic.child_chunk_id ORDER BY diagnostic.fused_rank
    ))[1]
    INTO result_count, top_child_id
    FROM knowledge_fetch_hybrid_shadow_diagnostics(
      golden_case.collection_ids,
      golden_case.query_text,
      golden_case.query_embedding,
      5
    ) diagnostic;

    IF golden_case.expect_no_evidence THEN
      IF result_count <> 0 OR top_child_id IS NOT NULL THEN
        RAISE EXCEPTION 'Golden case % returned % candidates, top=%',
          golden_case.case_id, result_count, top_child_id;
      END IF;
    ELSIF top_child_id IS DISTINCT FROM golden_case.expected_top_child_id THEN
      RAISE EXCEPTION 'Golden case % top=%, expected=%',
        golden_case.case_id,
        top_child_id,
        golden_case.expected_top_child_id;
    END IF;
  END LOOP;
END
$golden_hybrid_contract$;

DO $context_lane_rrf_contract$
DECLARE
  first_order UUID[];
  second_order UUID[];
BEGIN
  WITH query_lanes(lane_ordinal, query_text, query_embedding) AS (
    VALUES
      (
        1,
        '它的保留规则呢',
        (ARRAY[1::REAL] ||
          array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024)
      ),
      (
        2,
        'RETENTION_BLUE retention policy',
        (ARRAY[1::REAL] ||
          array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024)
      )
  ), lane_candidates AS (
    SELECT lane.lane_ordinal, diagnostic.child_chunk_id,
      diagnostic.fused_rank
    FROM query_lanes lane
    CROSS JOIN LATERAL knowledge_fetch_hybrid_shadow_diagnostics(
      ARRAY[
        '18180000-0000-0000-0000-000000000003'::UUID,
        '18300000-0000-0000-0000-000000000012'::UUID
      ],
      lane.query_text,
      lane.query_embedding,
      5
    ) diagnostic
  ), outer_fused AS (
    SELECT candidate.child_chunk_id,
      sum(1.0 / (60.0 + candidate.fused_rank)) AS score
    FROM lane_candidates candidate
    GROUP BY candidate.child_chunk_id
  )
  SELECT array_agg(fused.child_chunk_id ORDER BY fused.score DESC,
    fused.child_chunk_id)
  INTO first_order
  FROM outer_fused fused;

  WITH query_lanes(lane_ordinal, query_text, query_embedding) AS (
    VALUES
      (
        1,
        '它的保留规则呢',
        (ARRAY[1::REAL] ||
          array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024)
      ),
      (
        2,
        'RETENTION_BLUE retention policy',
        (ARRAY[1::REAL] ||
          array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024)
      )
  ), lane_candidates AS (
    SELECT lane.lane_ordinal, diagnostic.child_chunk_id,
      diagnostic.fused_rank
    FROM query_lanes lane
    CROSS JOIN LATERAL knowledge_fetch_hybrid_shadow_diagnostics(
      ARRAY[
        '18180000-0000-0000-0000-000000000003'::UUID,
        '18300000-0000-0000-0000-000000000012'::UUID
      ],
      lane.query_text,
      lane.query_embedding,
      5
    ) diagnostic
  ), outer_fused AS (
    SELECT candidate.child_chunk_id,
      sum(1.0 / (60.0 + candidate.fused_rank)) AS score
    FROM lane_candidates candidate
    GROUP BY candidate.child_chunk_id
  )
  SELECT array_agg(fused.child_chunk_id ORDER BY fused.score DESC,
    fused.child_chunk_id)
  INTO second_order
  FROM outer_fused fused;

  IF first_order IS DISTINCT FROM second_order
    OR first_order[1] IS DISTINCT FROM
      '18180000-0000-0000-0000-000000000013'::UUID
  THEN
    RAISE EXCEPTION 'context query-lane RRF is unstable: % / %',
      first_order, second_order;
  END IF;
END
$context_lane_rrf_contract$;

DO $argument_rejection_contract$
BEGIN
  BEGIN
    PERFORM * FROM knowledge_fetch_hybrid_shadow_diagnostics(
      ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
      'invalid zero vector',
      array_fill(0::REAL, ARRAY[1024])::VECTOR(1024),
      5
    );
    RAISE EXCEPTION 'zero query vector was accepted';
  EXCEPTION WHEN SQLSTATE '22023' THEN
    IF SQLERRM <> 'RAG_HYBRID_SHADOW_ARGUMENT_INVALID' THEN
      RAISE;
    END IF;
  END;
END
$argument_rejection_contract$;

SET enable_seqscan = off;
SET plan_cache_mode = force_generic_plan;
PREPARE g18_bm25_plan (TEXT) AS
SELECT child_chunk_id
FROM knowledge_child_bm25_shadow_projections
WHERE bm25_text <@> to_bm25query(
  $1,
  'idx_knowledge_child_bm25_shadow_text'
) < 0
ORDER BY bm25_text <@> to_bm25query(
  $1,
  'idx_knowledge_child_bm25_shadow_text'
), child_chunk_id
LIMIT 4;
EXPLAIN (COSTS OFF) EXECUTE g18_bm25_plan('ERR_CONN_RESET');
DEALLOCATE g18_bm25_plan;

SET hnsw.ef_search = 100;
PREPARE g18_hybrid_hnsw_plan (VECTOR(1024)) AS
SELECT child_chunk_id
FROM knowledge_child_vector_shadow_projections
ORDER BY embedding_vector <=> $1
LIMIT 4;
EXPLAIN (COSTS OFF)
EXECUTE g18_hybrid_hnsw_plan(
  (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024)
);
DEALLOCATE g18_hybrid_hnsw_plan;
RESET enable_seqscan;
RESET hnsw.ef_search;
RESET plan_cache_mode;

UPDATE knowledge_documents
SET status = 'tombstoned',
    visibility_epoch = visibility_epoch + 1,
    deleted_at = clock_timestamp(),
    updated_at = clock_timestamp()
WHERE id = '18180000-0000-0000-0000-000000000004';

DO $deletion_contract$
DECLARE
  candidate_count BIGINT;
BEGIN
  IF (SELECT count(*) FROM knowledge_bm25_shadow_sources) <> 1 THEN
    RAISE EXCEPTION 'tombstoned authority remains BM25-source-visible';
  END IF;
  IF (SELECT count(*) FROM knowledge_child_bm25_shadow_projections) <> 4 THEN
    RAISE EXCEPTION 'deletion destroyed immutable BM25 rollback rows';
  END IF;
  IF (SELECT count(*) FROM knowledge_child_vector_shadow_projections) <> 4 THEN
    RAISE EXCEPTION 'deletion destroyed immutable vector rollback rows';
  END IF;

  SELECT count(*) INTO candidate_count
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'ERR_CONN_RESET /api/v1/jobs',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  );
  IF candidate_count <> 0 THEN
    RAISE EXCEPTION 'hybrid shadow leaked tombstoned evidence';
  END IF;

  SELECT count(*) INTO candidate_count
  FROM knowledge_fetch_hybrid_shadow_diagnostics(
    ARRAY['18300000-0000-0000-0000-000000000012'::UUID],
    'RETENTION_BLUE retention policy',
    (ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]))::VECTOR(1024),
    5
  );
  IF candidate_count <> 1 THEN
    RAISE EXCEPTION 'separately selected current collection disappeared';
  END IF;

  SELECT count(*) INTO candidate_count
  FROM knowledge_fetch_hybrid_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'ERR_CONN_RESET /api/v1/jobs',
    ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]),
    5
  );
  IF candidate_count <> 0 THEN
    RAISE EXCEPTION 'legacy production reader leaked tombstoned evidence';
  END IF;
END
$deletion_contract$;

DROP TABLE g18_hybrid_golden_cases;

SELECT
  'PASS G18.4 golden=7 bm25+dense=rrf identifiers/cjk=recall negatives=0 deletion=hidden diagnostics=redacted' AS result;
