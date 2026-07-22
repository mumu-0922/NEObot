\set ON_ERROR_STOP on

CREATE TEMP TABLE g18_query_latency (
  iteration INTEGER PRIMARY KEY,
  latency_ms DOUBLE PRECISION NOT NULL CHECK (latency_ms >= 0)
) ON COMMIT PRESERVE ROWS;

DO $query_latency_contract$
DECLARE
  iteration INTEGER;
  target_ordinal INTEGER;
  target_child_id UUID;
  query_vector REAL[];
  started_at TIMESTAMPTZ;
  found_count BIGINT;
BEGIN
  FOR iteration IN 1..30 LOOP
    target_ordinal := ((iteration * 131) % 4096) + 1;
    target_child_id := (
      '18610000-0000-0000-0000-' ||
      lpad(target_ordinal::TEXT, 12, '0')
    )::UUID;
    query_vector := ARRAY[
      (target_ordinal % 97)::REAL / 100::REAL,
      ((target_ordinal * 7) % 101)::REAL / 100::REAL,
      1::REAL
    ] || array_fill(0::REAL, ARRAY[1021]);

    started_at := clock_timestamp();
    SELECT count(*) INTO found_count
    FROM knowledge_fetch_profiled_query_evidence_candidates(
      ARRAY['18180000-0000-0000-0000-000000000003'::UUID],
      'BULK_G18_' || target_ordinal::TEXT,
      query_vector,
      10
    ) candidate
    WHERE candidate.child_chunk_id = target_child_id;
    INSERT INTO g18_query_latency(iteration, latency_ms)
    VALUES (
      iteration,
      extract(epoch FROM clock_timestamp() - started_at) * 1000
    );
    IF found_count <> 1 THEN
      RAISE EXCEPTION 'resource query % missed child %',
        iteration, target_child_id;
    END IF;
  END LOOP;
END
$query_latency_contract$;

DO $database_budget_contract$
DECLARE
  latency_p95 DOUBLE PRECISION;
  latency_max DOUBLE PRECISION;
  projection_and_index_bytes BIGINT;
BEGIN
  SELECT
    percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms),
    max(latency_ms)
  INTO latency_p95, latency_max
  FROM g18_query_latency;

  SELECT
    pg_total_relation_size('knowledge_child_vector_shadow_projections') +
    pg_total_relation_size('knowledge_child_bm25_shadow_projections')
  INTO projection_and_index_bytes;

  IF latency_p95 > 500
    OR latency_max > 1000
    OR projection_and_index_bytes > 536870912
  THEN
    RAISE EXCEPTION
      'resource budget exceeded p95_ms=% max_ms=% projection_bytes=%',
      latency_p95, latency_max, projection_and_index_bytes;
  END IF;
END
$database_budget_contract$;

SELECT
  count(*) AS query_count,
  round(min(latency_ms)::NUMERIC, 3) AS minimum_ms,
  round(avg(latency_ms)::NUMERIC, 3) AS average_ms,
  round(
    percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms)::NUMERIC,
    3
  ) AS p95_ms,
  round(max(latency_ms)::NUMERIC, 3) AS maximum_ms
FROM g18_query_latency;

SELECT
  pg_total_relation_size(
    'knowledge_child_vector_shadow_projections'
  ) AS vector_projection_bytes,
  pg_total_relation_size(
    'knowledge_child_bm25_shadow_projections'
  ) AS bm25_projection_bytes,
  pg_indexes_size(
    'knowledge_child_vector_shadow_projections'
  ) AS vector_index_bytes,
  pg_indexes_size(
    'knowledge_child_bm25_shadow_projections'
  ) AS bm25_index_bytes;

SELECT 'PASS G18.5B.2c queries=30 p95<=500ms indexes<=512MiB'
  AS result;
