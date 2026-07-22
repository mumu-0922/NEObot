\set ON_ERROR_STOP on

BEGIN;

UPDATE knowledge_child_search_projections
SET lexical_text =
      'ERR_CONN_RESET on /api/v1/jobs. '
      '重试策略 uses bounded exponential backoff. '
      'RETENTION_BLUE retention policy keeps audit records for 30 days.',
    exact_terms = ARRAY[
      'ERR_CONN_RESET',
      '/api/v1/jobs',
      '重试策略',
      'RETENTION_BLUE'
    ]::TEXT[]
WHERE child_chunk_id = '18180000-0000-0000-0000-000000000013';

UPDATE knowledge_child_search_projections
SET lexical_text =
      '研究方向：连接中断恢复采用指数退避，并限制最大重试次数。',
    exact_terms = ARRAY['RETRY_BACKOFF', '研究方向']::TEXT[]
WHERE child_chunk_id = '18300000-0000-0000-0000-000000000001';

UPDATE knowledge_child_search_projections
SET lexical_text =
      'RETENTION_RED retention policy removes temporary exports after 7 days.',
    exact_terms = ARRAY['RETENTION_RED']::TEXT[]
WHERE child_chunk_id = '18300000-0000-0000-0000-000000000002';

UPDATE knowledge_child_search_projections
SET lexical_text =
      'RETENTION_BLUE retention policy archives records in the selected '
      'secondary collection.',
    exact_terms = ARRAY['RETENTION_BLUE']::TEXT[]
WHERE child_chunk_id = '18300000-0000-0000-0000-000000000017';

COMMIT;

DO $fixture_contract$
BEGIN
  IF (SELECT count(*) FROM knowledge_bm25_shadow_sources) <> 4 THEN
    RAISE EXCEPTION 'G18.4 source fixture count is not 4';
  END IF;
  IF (SELECT count(*) FROM knowledge_child_vector_shadow_projections) <> 4 THEN
    RAISE EXCEPTION 'G18.4 vector fixture count is not 4';
  END IF;
  IF EXISTS (
    SELECT 1
    FROM knowledge_bm25_shadow_sources
    WHERE lexical_text ~* 'weather|cooking'
  ) THEN
    RAISE EXCEPTION 'negative Golden terms leaked into the corpus fixture';
  END IF;
END
$fixture_contract$;

SELECT 'PASS G18.4 synthetic hybrid fixture rows=4 collections=2' AS result;
