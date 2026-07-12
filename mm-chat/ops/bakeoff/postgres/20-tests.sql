\set ON_ERROR_STOP on
\pset tuples_only on
SET search_path = phase15_bakeoff, public, paradedb;

\set tenant_a 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
\set version_a 'aaaaaaaa-0000-0000-0000-000000000001'

SELECT assert_true(
  (SELECT format_type(atttypid, atttypmod)
   FROM pg_attribute
   WHERE attrelid = 'vectors'::regclass AND attname = 'embedding')
    = 'vector(1024)',
  'the full-precision lane must be vector(1024)'
);
SELECT assert_true(
  (SELECT format_type(atttypid, atttypmod)
   FROM pg_attribute
   WHERE attrelid = 'vectors'::regclass AND attname = 'embedding_half')
    = 'halfvec(2048)',
  'the compressed lane must be halfvec(2048)'
);

SELECT assert_true(
  cardinality(('南京市长江大桥'::pdb.jieba)::text[]) >= 2,
  'Jieba must segment Chinese text into multiple tokens'
);
SELECT assert_true(
  cardinality(('南京市长江大桥'::pdb.lindera(chinese))::text[]) >= 2,
  'Lindera Chinese must segment Chinese text into multiple tokens'
);
SELECT assert_true(
  ('南京市长江大桥'::pdb.chinese_compatible)::text[]
    = ARRAY['南', '京', '市', '长', '江', '大', '桥'],
  'chinese_compatible must expose its character-token baseline'
);
SELECT assert_true(
  ARRAY(SELECT chunk_id FROM lexical_jieba
        WHERE content @@@ '混合搜索' ORDER BY chunk_id) @> ARRAY[2::bigint],
  'Jieba Chinese query must find the mixed-language fixture'
);
SELECT assert_true(
  ARRAY(SELECT chunk_id FROM lexical_jieba
        WHERE content @@@ 'hybrid' ORDER BY chunk_id) @> ARRAY[2::bigint],
  'Jieba mixed-language query must preserve the English token'
);
SELECT assert_true(
  ARRAY(SELECT chunk_id FROM lexical_lindera
        WHERE content @@@ '混合搜索' ORDER BY chunk_id) @> ARRAY[2::bigint],
  'Lindera Chinese query must find the mixed-language fixture'
);
SELECT assert_true(
  ARRAY(SELECT chunk_id FROM lexical_lindera
        WHERE content @@@ 'hybrid' ORDER BY chunk_id) @> ARRAY[2::bigint],
  'Lindera mixed-language query must preserve the English token'
);
SELECT assert_true(
  ARRAY(SELECT chunk_id FROM lexical_chinese_compatible
        WHERE content @@@ '混合搜索' ORDER BY chunk_id) @> ARRAY[2::bigint],
  'chinese_compatible query must find the mixed-language fixture'
);
SELECT assert_true(
  ARRAY(SELECT chunk_id FROM lexical_chinese_compatible
        WHERE content @@@ 'hybrid' ORDER BY chunk_id) @> ARRAY[2::bigint],
  'chinese_compatible mixed-language query must preserve the English token'
);

-- The application boundary supplies an explicit authorized-version array.
-- No document-row join participates in the pushed BM25 predicate.
CREATE TEMP TABLE authorized_bm25 AS
SELECT chunk_id, pdb.score(chunk_id) AS bm25_score
FROM lexical_jieba
WHERE content @@@ '混合搜索'
  AND tenant_id = :'tenant_a'::uuid
  AND document_version_id = ANY (
    ARRAY[:'version_a'::uuid]
  )
ORDER BY bm25_score DESC, chunk_id
LIMIT 20;

DO $assert_bm25_plan$
DECLARE
  plan json;
BEGIN
  EXECUTE $query$
    EXPLAIN (FORMAT JSON, COSTS OFF)
    SELECT chunk_id
    FROM lexical_jieba
    WHERE content @@@ '混合搜索'
      AND tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'::uuid
      AND document_version_id = ANY (
        ARRAY['aaaaaaaa-0000-0000-0000-000000000001'::uuid]
      )
  $query$ INTO plan;
  PERFORM assert_true(
    plan::text LIKE '%lexical_jieba_bm25_idx%',
    'authorized lexical query did not use the BM25 index'
  );
END
$assert_bm25_plan$;

SELECT assert_true(
  (SELECT count(*) FROM authorized_bm25) = 1,
  'low-selectivity BM25 ACL query must retain the authorized hit'
);
SELECT assert_true(
  NOT EXISTS (
    SELECT 1
    FROM authorized_bm25 AS result
    JOIN lexical_jieba AS source USING (chunk_id)
    WHERE source.tenant_id <> :'tenant_a'::uuid
       OR source.document_version_id <> :'version_a'::uuid
  ),
  'BM25 ACL query leaked a tenant or unauthorized version'
);

CREATE TEMP TABLE authorized_bm25_lindera AS
SELECT chunk_id
FROM lexical_lindera
WHERE content @@@ '混合搜索'
  AND tenant_id = :'tenant_a'::uuid
  AND document_version_id = ANY (ARRAY[:'version_a'::uuid])
ORDER BY pdb.score(chunk_id) DESC, chunk_id
LIMIT 20;

CREATE TEMP TABLE authorized_bm25_chinese_compatible AS
SELECT chunk_id
FROM lexical_chinese_compatible
WHERE content @@@ '混合搜索'
  AND tenant_id = :'tenant_a'::uuid
  AND document_version_id = ANY (ARRAY[:'version_a'::uuid])
ORDER BY pdb.score(chunk_id) DESC, chunk_id
LIMIT 20;

SELECT assert_true(
  (SELECT count(*) FROM authorized_bm25_lindera) = 1
  AND (SELECT count(*) FROM authorized_bm25_chinese_compatible) = 1,
  'all Chinese tokenizer lanes must retain exactly one authorized current hit'
);
SELECT assert_true(
  NOT EXISTS (
    SELECT 1 FROM authorized_bm25_lindera result
    JOIN lexical_lindera source USING (chunk_id)
    WHERE source.tenant_id <> :'tenant_a'::uuid
       OR source.document_version_id <> :'version_a'::uuid
  )
  AND NOT EXISTS (
    SELECT 1 FROM authorized_bm25_chinese_compatible result
    JOIN lexical_chinese_compatible source USING (chunk_id)
    WHERE source.tenant_id <> :'tenant_a'::uuid
       OR source.document_version_id <> :'version_a'::uuid
  ),
  'a non-Jieba BM25 lane leaked a tenant or unauthorized version'
);

DO $assert_other_bm25_plans$
DECLARE
  plan json;
BEGIN
  EXECUTE $query$
    EXPLAIN (FORMAT JSON, COSTS OFF)
    SELECT chunk_id FROM lexical_lindera
    WHERE content @@@ '混合搜索'
      AND tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'::uuid
      AND document_version_id = ANY (
        ARRAY['aaaaaaaa-0000-0000-0000-000000000001'::uuid]
      )
  $query$ INTO plan;
  PERFORM assert_true(plan::text LIKE '%lexical_lindera_bm25_idx%',
    'authorized Lindera query did not use its BM25 index');

  EXECUTE $query$
    EXPLAIN (FORMAT JSON, COSTS OFF)
    SELECT chunk_id FROM lexical_chinese_compatible
    WHERE content @@@ '混合搜索'
      AND tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'::uuid
      AND document_version_id = ANY (
        ARRAY['aaaaaaaa-0000-0000-0000-000000000001'::uuid]
      )
  $query$ INTO plan;
  PERFORM assert_true(plan::text LIKE '%lexical_chinese_compatible_bm25_idx%',
    'authorized chinese_compatible query did not use its BM25 index');
END
$assert_other_bm25_plans$;

-- Exact Keyword/Phrase Lane is independent from BM25 and preserves raw case.
BEGIN;
SET LOCAL enable_seqscan = off;
CREATE TEMP TABLE authorized_exact ON COMMIT PRESERVE ROWS AS
SELECT chunk_id
FROM exact_keywords
WHERE tenant_id = :'tenant_a'::uuid
  AND document_version_id = ANY (ARRAY[:'version_a'::uuid])
  AND exact_values @> ARRAY['ERR_AUTH_401']::text[]
  AND exact_phrases @> ARRAY['权限过滤失败']::text[]
ORDER BY chunk_id;
COMMIT;
SELECT assert_true(
  (SELECT array_agg(chunk_id ORDER BY chunk_id) FROM authorized_exact)
    = ARRAY[1::bigint],
  'Exact Lane must match the authorized raw identifier and phrase only'
);
SELECT assert_true(
  NOT EXISTS (
    SELECT 1 FROM exact_keywords
    WHERE exact_values @> ARRAY['err_auth_401']::text[]
  ),
  'Exact Lane must not silently lowercase case-sensitive identifiers'
);

DO $assert_exact_plan$
DECLARE
  plan json;
BEGIN
  SET LOCAL enable_seqscan = off;
  EXECUTE $query$
    EXPLAIN (FORMAT JSON, COSTS OFF)
    SELECT chunk_id
    FROM exact_keywords
    WHERE tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'::uuid
      AND document_version_id = ANY (
        ARRAY['aaaaaaaa-0000-0000-0000-000000000001'::uuid]
      )
      AND exact_values @> ARRAY['ERR_AUTH_401']::text[]
      AND exact_phrases @> ARRAY['权限过滤失败']::text[]
  $query$ INTO plan;
  PERFORM assert_true(
    plan::text LIKE '%exact_keywords_values_idx%'
      OR plan::text LIKE '%exact_keywords_phrases_idx%',
    'authorized Exact Lane did not use a dedicated GIN index'
  );
END
$assert_exact_plan$;

-- Exact lane: index scans are disabled deliberately and the result is kept
-- as the recall reference for the approximate HNSW lane.
BEGIN;
SET LOCAL enable_indexscan = off;
SET LOCAL enable_indexonlyscan = off;
SET LOCAL enable_bitmapscan = off;
CREATE TEMP TABLE exact_top20 ON COMMIT PRESERVE ROWS AS
SELECT chunk_id, row_number() OVER (
  ORDER BY embedding <-> vector_1024(11), chunk_id
) AS rank
FROM vectors
ORDER BY embedding <-> vector_1024(11), chunk_id
LIMIT 20;
COMMIT;

BEGIN;
SET LOCAL enable_seqscan = off;
SET LOCAL hnsw.ef_search = 100;
SET LOCAL hnsw.iterative_scan = 'strict_order';
CREATE TEMP TABLE hnsw_top20 ON COMMIT PRESERVE ROWS AS
WITH candidates AS MATERIALIZED (
  SELECT chunk_id, embedding <-> vector_1024(11) AS distance
  FROM vectors
  ORDER BY embedding <-> vector_1024(11)
  LIMIT 20
)
SELECT chunk_id, row_number() OVER (ORDER BY distance, chunk_id) AS rank
FROM candidates;
COMMIT;

DO $assert_hnsw_plan$
DECLARE
  plan json;
BEGIN
  SET LOCAL enable_seqscan = off;
  EXECUTE $query$
    EXPLAIN (FORMAT JSON, COSTS OFF)
    SELECT chunk_id
    FROM vectors
    ORDER BY embedding <-> vector_1024(11)
    LIMIT 20
  $query$ INTO plan;
  PERFORM assert_true(
    plan::text LIKE '%vectors_embedding_hnsw_idx%',
    'approximate vector query did not use the HNSW index'
  );
END
$assert_hnsw_plan$;

SELECT assert_true(
  (SELECT count(*)::numeric / 20
   FROM exact_top20 INNER JOIN hnsw_top20 USING (chunk_id)) >= 0.90,
  'vector(1024) HNSW recall@20 fell below 0.90'
);

-- Exercise the halfvec(2048) ANN operator/index independently.
BEGIN;
SET LOCAL enable_seqscan = off;
SET LOCAL hnsw.ef_search = 100;
SELECT assert_true(
  (SELECT chunk_id FROM vectors
   ORDER BY embedding_half <-> halfvec_2048(11) LIMIT 1) = 11,
  'halfvec(2048) HNSW top-1 mismatch'
);
COMMIT;

DO $assert_half_hnsw_plan$
DECLARE
  plan json;
BEGIN
  SET LOCAL enable_seqscan = off;
  EXECUTE $query$
    EXPLAIN (FORMAT JSON, COSTS OFF)
    SELECT chunk_id
    FROM vectors
    ORDER BY embedding_half <-> halfvec_2048(11)
    LIMIT 1
  $query$ INTO plan;
  PERFORM assert_true(
    plan::text LIKE '%vectors_half_hnsw_idx%',
    'halfvec query did not use the HNSW index'
  );
END
$assert_half_hnsw_plan$;

-- Only 4% of vector rows are authorized. Iterative scanning must still fill
-- the filtered top-k without returning a single unauthorized row.
BEGIN;
SET LOCAL enable_seqscan = off;
SET LOCAL hnsw.ef_search = 100;
SET LOCAL hnsw.iterative_scan = 'strict_order';
CREATE TEMP TABLE acl_vector_top10 ON COMMIT PRESERVE ROWS AS
SELECT chunk_id
FROM vectors
WHERE tenant_id = :'tenant_a'::uuid
  AND document_version_id = ANY (ARRAY[:'version_a'::uuid])
ORDER BY embedding <-> vector_1024(11)
LIMIT 10;
COMMIT;

BEGIN;
SET LOCAL enable_indexscan = off;
SET LOCAL enable_indexonlyscan = off;
SET LOCAL enable_bitmapscan = off;
CREATE TEMP TABLE acl_vector_exact_top10 ON COMMIT PRESERVE ROWS AS
SELECT chunk_id
FROM vectors
WHERE tenant_id = :'tenant_a'::uuid
  AND document_version_id = ANY (ARRAY[:'version_a'::uuid])
ORDER BY embedding <-> vector_1024(11), chunk_id
LIMIT 10;
COMMIT;

DO $assert_filtered_hnsw_plan$
DECLARE
  plan json;
BEGIN
  SET LOCAL enable_seqscan = off;
  SET LOCAL hnsw.iterative_scan = 'strict_order';
  EXECUTE $query$
    EXPLAIN (FORMAT JSON, COSTS OFF)
    SELECT chunk_id
    FROM vectors
    WHERE tenant_id = 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'::uuid
      AND document_version_id = ANY (
        ARRAY['aaaaaaaa-0000-0000-0000-000000000001'::uuid]
      )
    ORDER BY embedding <-> vector_1024(11)
    LIMIT 10
  $query$ INTO plan;
  PERFORM assert_true(
    plan::text LIKE '%vectors_embedding_hnsw_idx%',
    'filtered vector query did not use iterative HNSW'
  );
END
$assert_filtered_hnsw_plan$;

SELECT assert_true(
  (SELECT count(*) FROM acl_vector_top10) = 10,
  'low-selectivity filtered HNSW query did not fill top-10'
);
SELECT assert_true(
  NOT EXISTS (
    SELECT 1
    FROM acl_vector_top10 AS result
    JOIN vectors AS source USING (chunk_id)
    WHERE source.tenant_id <> :'tenant_a'::uuid
       OR source.document_version_id <> :'version_a'::uuid
  ),
  'HNSW ACL query leaked a tenant or unauthorized version'
);
SELECT assert_true(
  (SELECT count(*)::numeric / 10
   FROM acl_vector_top10 JOIN acl_vector_exact_top10 USING (chunk_id)) >= 0.90,
  'low-selectivity filtered HNSW recall@10 fell below 0.90'
);

BEGIN;
SET LOCAL enable_indexscan = off;
SET LOCAL enable_indexonlyscan = off;
SET LOCAL enable_bitmapscan = off;
CREATE TEMP TABLE exact_half_top20 ON COMMIT PRESERVE ROWS AS
SELECT chunk_id
FROM vectors
ORDER BY embedding_half <-> halfvec_2048(11), chunk_id
LIMIT 20;
COMMIT;

BEGIN;
SET LOCAL enable_seqscan = off;
SET LOCAL hnsw.ef_search = 100;
CREATE TEMP TABLE hnsw_half_top20 ON COMMIT PRESERVE ROWS AS
WITH candidates AS MATERIALIZED (
  SELECT chunk_id, embedding_half <-> halfvec_2048(11) AS distance
  FROM vectors
  ORDER BY embedding_half <-> halfvec_2048(11)
  LIMIT 20
)
SELECT chunk_id FROM candidates ORDER BY distance, chunk_id;
COMMIT;
SELECT assert_true(
  (SELECT count(*)::numeric / 20
   FROM exact_half_top20 JOIN hnsw_half_top20 USING (chunk_id)) >= 0.90,
  'halfvec(2048) HNSW recall@20 fell below 0.90'
);

-- Reciprocal Rank Fusion is deterministic because every source rank and the
-- final order has chunk_id as an explicit tie-breaker.
CREATE TEMP TABLE rrf_first AS
WITH lexical(chunk_id, rank) AS (
  VALUES (1::bigint, 1), (2, 2), (3, 3)
), semantic(chunk_id, rank) AS (
  VALUES (2::bigint, 1), (3, 2), (1, 3)
), fused AS (
  SELECT chunk_id, sum(1.0 / (60 + rank)) AS score
  FROM (
    SELECT * FROM lexical
    UNION ALL
    SELECT * FROM semantic
  ) AS lanes
  GROUP BY chunk_id
)
SELECT chunk_id, score,
       row_number() OVER (ORDER BY score DESC, chunk_id) AS final_rank
FROM fused;

SELECT assert_true(
  (SELECT array_agg(chunk_id ORDER BY final_rank) FROM rrf_first)
    = ARRAY[2::bigint, 1, 3],
  'RRF order differs from the deterministic reference'
);
SELECT assert_true(
  (WITH lexical(chunk_id, rank) AS (
     VALUES (10::bigint, 1), (11, 2)
   ), semantic(chunk_id, rank) AS (
     VALUES (11::bigint, 1), (10, 2)
   ), fused AS (
     SELECT chunk_id, sum(1.0 / (60 + rank)) AS score
     FROM (SELECT * FROM lexical UNION ALL SELECT * FROM semantic) lanes
     GROUP BY chunk_id
   )
   SELECT array_agg(chunk_id ORDER BY score DESC, chunk_id) FROM fused)
    = ARRAY[10::bigint, 11],
  'RRF tie-break must be stable by chunk_id'
);

-- Transaction rollback posture: failed/aborted writes leave no residue.
BEGIN;
INSERT INTO lexical_jieba VALUES (
  999999,
  :'tenant_a'::uuid,
  :'version_a'::uuid,
  'rollback sentinel'
);
ROLLBACK;
SELECT assert_true(
  NOT EXISTS (SELECT 1 FROM lexical_jieba WHERE chunk_id = 999999),
  'rollback sentinel survived transaction rollback'
);

SELECT format(
  'PASS extensions=pg_search:%s,vector:%s vector_recall@20=%s halfvec_recall@20=%s acl_bm25=%s/%s/%s acl_vector=%s filtered_recall@10=%s exact=%s',
  (SELECT extversion FROM pg_extension WHERE extname = 'pg_search'),
  (SELECT extversion FROM pg_extension WHERE extname = 'vector'),
  (SELECT count(*)::numeric / 20 FROM exact_top20 JOIN hnsw_top20 USING (chunk_id)),
  (SELECT count(*)::numeric / 20 FROM exact_half_top20 JOIN hnsw_half_top20 USING (chunk_id)),
  (SELECT count(*) FROM authorized_bm25),
  (SELECT count(*) FROM authorized_bm25_lindera),
  (SELECT count(*) FROM authorized_bm25_chinese_compatible),
  (SELECT count(*) FROM acl_vector_top10),
  (SELECT count(*)::numeric / 10 FROM acl_vector_top10 JOIN acl_vector_exact_top10 USING (chunk_id)),
  (SELECT count(*) FROM authorized_exact)
);
