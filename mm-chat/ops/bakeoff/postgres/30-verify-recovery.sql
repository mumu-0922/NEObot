\set ON_ERROR_STOP on
\pset tuples_only on
SET search_path = phase15_bakeoff, public, paradedb;

SELECT assert_true(
  (SELECT extversion FROM pg_extension WHERE extname = 'pg_search') = '0.24.2',
  'pg_search version changed after restore/restart'
);
SELECT assert_true(
  (SELECT extversion FROM pg_extension WHERE extname = 'vector') = '0.8.2',
  'vector version changed after restore/restart'
);
SELECT assert_true(
  (SELECT count(*) FROM vectors) = 500,
  'vector fixtures missing after restore/restart'
);
SELECT assert_true(
  (SELECT count(*) FROM lexical_jieba WHERE content @@@ '混合搜索') = 97,
  'BM25 index/content mismatch after restore/restart'
);
SELECT assert_true(
  (SELECT count(*) FROM lexical_lindera WHERE content @@@ '混合搜索') = 97
  AND (SELECT count(*) FROM lexical_chinese_compatible
       WHERE content @@@ '混合搜索') = 97,
  'non-Jieba BM25 index/content mismatch after restore/restart'
);
SELECT assert_true(
  (SELECT count(*) FROM exact_keywords
   WHERE exact_values @> ARRAY['ERR_AUTH_401']::text[]) = 2,
  'Exact Lane index/content mismatch after restore/restart'
);
SELECT assert_true(
  (SELECT chunk_id FROM vectors
   ORDER BY embedding <-> vector_1024(11), chunk_id LIMIT 1) = 11,
  'vector exact lane mismatch after restore/restart'
);
SELECT 'PASS recovery/restore verification';
