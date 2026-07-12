\set ON_ERROR_STOP on
SET search_path = phase15_bakeoff, public, paradedb;

\set tenant_a 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'
\set tenant_b 'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb'
\set version_a 'aaaaaaaa-0000-0000-0000-000000000001'
\set version_a_old 'aaaaaaaa-0000-0000-0000-000000000002'
\set version_b 'bbbbbbbb-0000-0000-0000-000000000001'

INSERT INTO documents (document_id, tenant_id, current_version_id) VALUES
  ('aaaaaaaa-1111-1111-1111-111111111111', :'tenant_a', :'version_a'),
  ('bbbbbbbb-1111-1111-1111-111111111111', :'tenant_b', :'version_b');

INSERT INTO lexical_jieba
  (chunk_id, tenant_id, document_version_id, content)
VALUES
  (1, :'tenant_a', :'version_a',
   'PostgreSQL 全文检索与向量召回指南'),
  (2, :'tenant_a', :'version_a',
   'Neo Chat 混合搜索 hybrid retrieval 教程'),
  (3, :'tenant_a', :'version_a',
   'Jieba 中文分词支持知识库搜索'),
  (4, :'tenant_a', :'version_a',
   '权限过滤 ACL prevents tenant leakage'),
  (5, :'tenant_a', :'version_a_old',
   '混合搜索的旧版本内容不得出现在结果中');

-- Ninety-five matching rows belong to another tenant. Authorized current
-- rows are therefore a low-selectivity slice of the BM25 candidate set.
INSERT INTO lexical_jieba
  (chunk_id, tenant_id, document_version_id, content)
SELECT
  1000 + n,
  :'tenant_b'::uuid,
  :'version_b'::uuid,
  ('机密混合搜索 tenant-b 文档 ' || n)::pdb.jieba
FROM generate_series(1, 95) AS fixture(n);

INSERT INTO lexical_lindera
  (chunk_id, tenant_id, document_version_id, content)
SELECT chunk_id, tenant_id, document_version_id,
       content::text::pdb.lindera(chinese)
FROM lexical_jieba;

INSERT INTO lexical_chinese_compatible
  (chunk_id, tenant_id, document_version_id, content)
SELECT chunk_id, tenant_id, document_version_id,
       content::text::pdb.chinese_compatible
FROM lexical_jieba;

INSERT INTO exact_keywords
  (chunk_id, tenant_id, document_version_id, exact_values, exact_phrases)
VALUES
  (1, :'tenant_a', :'version_a',
   ARRAY['ERR_AUTH_401', '/v1/knowledge/query', 'NeoChat-v2.0'],
   ARRAY['权限过滤失败', 'strict grounded RAG']),
  (2, :'tenant_a', :'version_a_old',
   ARRAY['ERR_LEGACY_410'], ARRAY['旧版本内容']),
  (3, :'tenant_b', :'version_b',
   ARRAY['ERR_AUTH_401'], ARRAY['权限过滤失败']);

INSERT INTO vectors
  (chunk_id, tenant_id, document_version_id, embedding, embedding_half)
SELECT
  n,
  CASE WHEN n <= 20 THEN :'tenant_a'::uuid ELSE :'tenant_b'::uuid END,
  CASE WHEN n <= 20 THEN :'version_a'::uuid ELSE :'version_b'::uuid END,
  vector_1024(n),
  halfvec_2048(n)
FROM generate_series(1, 500) AS fixture(n);

-- One BM25 index for this Jieba table/tokenizer lane.
CREATE INDEX lexical_jieba_bm25_idx
ON lexical_jieba
USING bm25 (chunk_id, tenant_id, document_version_id, content)
WITH (key_field = 'chunk_id');

CREATE INDEX lexical_lindera_bm25_idx
ON lexical_lindera
USING bm25 (chunk_id, tenant_id, document_version_id, content)
WITH (key_field = 'chunk_id');

CREATE INDEX lexical_chinese_compatible_bm25_idx
ON lexical_chinese_compatible
USING bm25 (chunk_id, tenant_id, document_version_id, content)
WITH (key_field = 'chunk_id');

CREATE INDEX exact_keywords_values_idx
ON exact_keywords USING gin (exact_values);
CREATE INDEX exact_keywords_phrases_idx
ON exact_keywords USING gin (exact_phrases);

CREATE INDEX vectors_embedding_hnsw_idx
ON vectors USING hnsw (embedding vector_l2_ops)
WITH (m = 8, ef_construction = 64);

CREATE INDEX vectors_half_hnsw_idx
ON vectors USING hnsw (embedding_half halfvec_l2_ops)
WITH (m = 8, ef_construction = 64);

ANALYZE lexical_jieba;
ANALYZE lexical_lindera;
ANALYZE lexical_chinese_compatible;
ANALYZE exact_keywords;
ANALYZE vectors;
