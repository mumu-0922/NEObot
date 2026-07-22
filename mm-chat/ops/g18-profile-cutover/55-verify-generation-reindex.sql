\set ON_ERROR_STOP on

DO $building_projection_contract$
BEGIN
  IF (SELECT count(*)
      FROM knowledge_bm25_shadow_build_sources
      WHERE index_generation_id =
        '18500000-0000-0000-0000-000000000007') <> 1
    OR (SELECT count(*)
        FROM knowledge_child_vector_shadow_projections
        WHERE index_generation_id =
          '18500000-0000-0000-0000-000000000007') <> 1
    OR (SELECT count(*)
        FROM knowledge_child_bm25_shadow_projections
        WHERE index_generation_id =
          '18500000-0000-0000-0000-000000000007') <> 1
    OR EXISTS (
      SELECT 1
      FROM knowledge_bm25_shadow_sources
      WHERE index_generation_id =
        '18500000-0000-0000-0000-000000000007'
    )
  THEN
    RAISE EXCEPTION
      'building generation publication/source authority mismatch';
  END IF;
END
$building_projection_contract$;

DO $incomplete_cutover_contract$
DECLARE
  caught_message TEXT;
BEGIN
  BEGIN
    UPDATE knowledge_index_generations
    SET status = 'retired', retired_at = clock_timestamp()
    WHERE id = '18180000-0000-0000-0000-000000000007'
      AND status = 'active';
    UPDATE knowledge_projection_state
    SET readiness = 'retired', updated_at = clock_timestamp()
    WHERE index_generation_id =
      '18180000-0000-0000-0000-000000000007'
      AND readiness = 'ready';

    UPDATE knowledge_index_generations
    SET status = 'active',
        artifact_manifest_hash = repeat('5', 64),
        verified_at = clock_timestamp(),
        activated_at = clock_timestamp()
    WHERE id = '18500000-0000-0000-0000-000000000007'
      AND status = 'building';
    UPDATE knowledge_projection_state
    SET readiness = 'ready',
        manifest_hash = repeat('5', 64),
        document_count = 3,
        parent_count = 3,
        child_count = 3,
        verified_at = clock_timestamp(),
        updated_at = clock_timestamp()
    WHERE index_generation_id =
      '18500000-0000-0000-0000-000000000007'
      AND readiness = 'building';

    UPDATE knowledge_corpus_projection_head
    SET active_index_generation_id =
          '18500000-0000-0000-0000-000000000007',
        corpus_projection_revision = corpus_projection_revision + 1,
        head_revision = head_revision + 1,
        updated_at = clock_timestamp()
    WHERE singleton_id = 1
      AND active_index_generation_id =
        '18180000-0000-0000-0000-000000000007';
    RAISE EXCEPTION 'incomplete generation cutover unexpectedly succeeded';
  EXCEPTION WHEN SQLSTATE '55000' THEN
    GET STACKED DIAGNOSTICS caught_message = MESSAGE_TEXT;
    IF caught_message <> 'RAG_RETRIEVAL_GENERATION_BACKFILL_INCOMPLETE' THEN
      RAISE;
    END IF;
  END;

  IF (SELECT status FROM knowledge_index_generations
      WHERE id = '18180000-0000-0000-0000-000000000007') <> 'active'
    OR (SELECT status FROM knowledge_index_generations
        WHERE id = '18500000-0000-0000-0000-000000000007') <> 'building'
    OR (SELECT active_index_generation_id
        FROM knowledge_corpus_projection_head
        WHERE singleton_id = 1) <>
      '18180000-0000-0000-0000-000000000007'
  THEN
    RAISE EXCEPTION 'rejected generation cutover left partial state';
  END IF;
END
$incomplete_cutover_contract$;

BEGIN;
INSERT INTO knowledge_document_projection_heads (
  index_generation_id, document_id, active_materialization_id,
  last_corpus_projection_revision
) VALUES
  (
    '18500000-0000-0000-0000-000000000007',
    '18300000-0000-0000-0000-000000000013',
    '18500000-0000-0000-0000-000000000020',
    2
  ),
  (
    '18500000-0000-0000-0000-000000000007',
    '18400000-0000-0000-0000-000000000012',
    '18500000-0000-0000-0000-000000000030',
    2
  );
COMMIT;

SET ROLE rag_replay_operator;
DO $complete_generation_contract$
DECLARE
  readiness RECORD;
  vector_backfill RECORD;
  bm25_backfill RECORD;
BEGIN
  SELECT * INTO readiness
  FROM knowledge_assert_pg17_generation_ready(
    '18500000-0000-0000-0000-000000000007'
  );
  IF readiness.document_count <> 3
    OR readiness.eligible_count <> 3
    OR readiness.vector_count <> 3
    OR readiness.bm25_count <> 3
  THEN
    RAISE EXCEPTION 'complete generation readiness result: %', readiness;
  END IF;

  SELECT * INTO vector_backfill
  FROM knowledge_backfill_pgvector_shadow(
    '18500000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  SELECT * INTO bm25_backfill
  FROM knowledge_backfill_bm25_shadow(
    '18500000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000011'
  );
  IF vector_backfill.eligible_count <> 3
    OR vector_backfill.inserted_count <> 0
    OR vector_backfill.verified_shadow_count <> 3
    OR bm25_backfill.eligible_count <> 3
    OR bm25_backfill.inserted_count <> 0
    OR bm25_backfill.verified_shadow_count <> 3
  THEN
    RAISE EXCEPTION 'idempotent generation backfill vector=% bm25=%',
      vector_backfill, bm25_backfill;
  END IF;
END
$complete_generation_contract$;
RESET ROLE;

DO $generation_security_contract$
BEGIN
  IF has_function_privilege(
    'go_api_runtime',
    'knowledge_assert_pg17_generation_ready(uuid)',
    'EXECUTE'
  ) OR has_function_privilege(
    'rag_worker_executor',
    'knowledge_assert_pg17_generation_ready(uuid)',
    'EXECUTE'
  ) OR NOT has_function_privilege(
    'rag_replay_operator',
    'knowledge_assert_pg17_generation_ready(uuid)',
    'EXECUTE'
  ) THEN
    RAISE EXCEPTION 'generation readiness privileges are not bounded';
  END IF;
  IF EXISTS (
    SELECT 1 FROM pg_proc function
    WHERE function.oid IN (
      'knowledge_assert_pg17_generation_ready(uuid)'::REGPROCEDURE,
      'knowledge_fence_pg17_generation_cutover()'::REGPROCEDURE
    )
      AND NOT EXISTS (
        SELECT 1 FROM unnest(function.proconfig) setting
        WHERE setting LIKE 'search_path=%pg_catalog%pg_temp'
          AND setting NOT LIKE '%$user%'
      )
  ) THEN
    RAISE EXCEPTION 'generation SECURITY DEFINER path is not hardened';
  END IF;
END
$generation_security_contract$;

BEGIN;
UPDATE knowledge_index_generations
SET status = 'retired', retired_at = clock_timestamp()
WHERE id = '18180000-0000-0000-0000-000000000007'
  AND status = 'active';
UPDATE knowledge_projection_state
SET readiness = 'retired', updated_at = clock_timestamp()
WHERE index_generation_id = '18180000-0000-0000-0000-000000000007'
  AND readiness = 'ready';
UPDATE knowledge_index_generations
SET status = 'active',
    artifact_manifest_hash = repeat('5', 64),
    verified_at = clock_timestamp(),
    activated_at = clock_timestamp()
WHERE id = '18500000-0000-0000-0000-000000000007'
  AND status = 'building';
UPDATE knowledge_projection_state
SET readiness = 'ready',
    manifest_hash = repeat('5', 64),
    document_count = 3,
    parent_count = 3,
    child_count = 3,
    verified_at = clock_timestamp(),
    updated_at = clock_timestamp()
WHERE index_generation_id = '18500000-0000-0000-0000-000000000007'
  AND readiness = 'building';
UPDATE knowledge_corpus_projection_head
SET active_index_generation_id =
      '18500000-0000-0000-0000-000000000007',
    corpus_projection_revision = corpus_projection_revision + 1,
    head_revision = head_revision + 1,
    updated_at = clock_timestamp()
WHERE singleton_id = 1
  AND active_index_generation_id =
    '18180000-0000-0000-0000-000000000007';
COMMIT;

SET ROLE go_api_runtime;
DO $promoted_reader_contract$
DECLARE
  winner UUID;
BEGIN
  SELECT child_chunk_id INTO winner
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'LIVE_BETA',
    ARRAY[0::REAL, 0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1019]),
    5
  )
  ORDER BY rank_score DESC, child_chunk_id
  LIMIT 1;
  IF winner <> '18500000-0000-0000-0000-000000000032' THEN
    RAISE EXCEPTION 'promoted generation winner=%', winner;
  END IF;
END
$promoted_reader_contract$;
RESET ROLE;

BEGIN;
UPDATE knowledge_index_generations
SET status = 'retired', retired_at = clock_timestamp()
WHERE id = '18500000-0000-0000-0000-000000000007'
  AND status = 'active';
UPDATE knowledge_projection_state
SET readiness = 'retired', updated_at = clock_timestamp()
WHERE index_generation_id = '18500000-0000-0000-0000-000000000007'
  AND readiness = 'ready';
UPDATE knowledge_index_generations
SET status = 'active', retired_at = NULL
WHERE id = '18180000-0000-0000-0000-000000000007'
  AND status = 'retired';
UPDATE knowledge_projection_state
SET readiness = 'ready', updated_at = clock_timestamp()
WHERE index_generation_id = '18180000-0000-0000-0000-000000000007'
  AND readiness = 'retired';
UPDATE knowledge_corpus_projection_head
SET active_index_generation_id =
      '18180000-0000-0000-0000-000000000007',
    corpus_projection_revision = corpus_projection_revision + 1,
    head_revision = head_revision + 1,
    updated_at = clock_timestamp()
WHERE singleton_id = 1
  AND active_index_generation_id =
    '18500000-0000-0000-0000-000000000007';
COMMIT;

SET ROLE go_api_runtime;
DO $rollback_reader_contract$
DECLARE
  winner UUID;
BEGIN
  SELECT child_chunk_id INTO winner
  FROM knowledge_fetch_profiled_query_evidence_candidates(
    ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
    'LIVE_BETA',
    ARRAY[0::REAL, 0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1019]),
    5
  )
  ORDER BY rank_score DESC, child_chunk_id
  LIMIT 1;
  IF winner <> '18400000-0000-0000-0000-000000000016' THEN
    RAISE EXCEPTION 'restored generation winner=%', winner;
  END IF;
END
$rollback_reader_contract$;
RESET ROLE;

DO $rollback_retention_contract$
BEGIN
  IF (SELECT status FROM knowledge_index_generations
      WHERE id = '18500000-0000-0000-0000-000000000007') <> 'retired'
    OR (SELECT count(*) FROM knowledge_child_vector_shadow_projections
        WHERE index_generation_id =
          '18500000-0000-0000-0000-000000000007') <> 3
    OR (SELECT count(*) FROM knowledge_child_bm25_shadow_projections
        WHERE index_generation_id =
          '18500000-0000-0000-0000-000000000007') <> 3
  THEN
    RAISE EXCEPTION 'generation rollback destroyed retained projection rows';
  END IF;
END
$rollback_retention_contract$;

SELECT 'PASS G18.5B.2b reindex partial=rejected promotion=3 rollback=exact'
  AS result;
