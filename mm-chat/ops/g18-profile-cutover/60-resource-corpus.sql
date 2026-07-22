\set ON_ERROR_STOP on

BEGIN;
SET CONSTRAINTS ALL DEFERRED;

INSERT INTO files (
  id, user_id, original_filename, mime_type, byte_size, sha256, object_key
) VALUES (
  '18600000-0000-0000-0000-000000000001',
  '18180000-0000-0000-0000-000000000001',
  'g18-resource-corpus.pdf',
  'application/pdf',
  1048576,
  repeat('6', 64),
  'g18/synthetic/source/g18-resource-corpus.pdf'
);

INSERT INTO knowledge_documents (
  id, collection_id, status, visibility_epoch, created_by_user_id
) VALUES (
  '18600000-0000-0000-0000-000000000002',
  '18180000-0000-0000-0000-000000000003',
  'processing',
  1,
  '18180000-0000-0000-0000-000000000001'
);

INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, visibility_epoch, status,
  content_hash, created_by_user_id
) VALUES (
  '18600000-0000-0000-0000-000000000003',
  '18600000-0000-0000-0000-000000000002',
  '18600000-0000-0000-0000-000000000001',
  1,
  1,
  'active',
  repeat('7', 64),
  '18180000-0000-0000-0000-000000000001'
);

UPDATE knowledge_documents
SET current_version_id = '18600000-0000-0000-0000-000000000003',
    status = 'active',
    updated_at = clock_timestamp()
WHERE id = '18600000-0000-0000-0000-000000000002';

INSERT INTO knowledge_document_materializations (
  id, index_generation_id, collection_id, document_id, document_version_id,
  file_id, materialization_seq, source_content_hash, base_profile_hash,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch, status,
  manifest_hash, result_hash, verified_at, published_at
) VALUES (
  '18600000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000003',
  '18600000-0000-0000-0000-000000000002',
  '18600000-0000-0000-0000-000000000003',
  '18600000-0000-0000-0000-000000000001',
  1,
  repeat('7', 64),
  repeat('e', 64),
  1,
  1,
  1,
  1,
  'published',
  repeat('8', 64),
  repeat('9', 64),
  clock_timestamp(),
  clock_timestamp()
);

INSERT INTO knowledge_parent_chunks (
  id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count, heading_path, locator_summary
) VALUES (
  '18600000-0000-0000-0000-000000000005',
  '18600000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000007',
  '18600000-0000-0000-0000-000000000002',
  '18600000-0000-0000-0000-000000000003',
  0,
  repeat('d', 64),
  repeat('6', 64),
  repeat('7', 64),
  'Synthetic parent for the representative G18 resource corpus.',
  10,
  ARRAY['G18', 'Resource Corpus']::TEXT[],
  '{"kind":"page","page":1}'::JSONB
);

INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
)
SELECT
  ('18610000-0000-0000-0000-' || lpad(series.id::TEXT, 12, '0'))::UUID,
  '18600000-0000-0000-0000-000000000005'::UUID,
  '18600000-0000-0000-0000-000000000004'::UUID,
  '18180000-0000-0000-0000-000000000007'::UUID,
  '18600000-0000-0000-0000-000000000002'::UUID,
  '18600000-0000-0000-0000-000000000003'::UUID,
  series.id - 1,
  repeat('d', 64),
  encode(sha256(convert_to('span:' || series.id::TEXT, 'UTF8')), 'hex'),
  encode(sha256(convert_to('content:' || series.id::TEXT, 'UTF8')), 'hex'),
  'G18 resource chunk ' || series.id::TEXT ||
    ' covers bounded single-server retrieval qualification.',
  12
FROM generate_series(1, 4096) AS series(id);

INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
)
SELECT
  child.id,
  child.parent_chunk_id,
  child.materialization_id,
  child.index_generation_id,
  '18180000-0000-0000-0000-000000000003'::UUID,
  child.document_id,
  child.document_version_id,
  '18180000-0000-0000-0000-000000000011'::UUID,
  'jina-embeddings-v4',
  1024,
  ARRAY[
    ((child.ordinal + 1) % 97)::REAL / 100::REAL,
    (((child.ordinal + 1) * 7) % 101)::REAL / 100::REAL,
    1::REAL
  ] || array_fill(0::REAL, ARRAY[1021]),
  encode(
    sha256(convert_to('vector:' || (child.ordinal + 1)::TEXT, 'UTF8')),
    'hex'
  ),
  'G18 representative retrieval row ' || (child.ordinal + 1)::TEXT ||
    ' 单机知识库检索性能样本',
  ARRAY['BULK_G18_' || (child.ordinal + 1)::TEXT]::TEXT[],
  child.source_span_hash,
  child.chunk_profile_hash,
  child.content_hash,
  jsonb_build_object('kind', 'page', 'page', child.ordinal + 1),
  'ready',
  clock_timestamp()
FROM knowledge_child_chunks child
WHERE child.materialization_id =
  '18600000-0000-0000-0000-000000000004'::UUID
ORDER BY child.ordinal;

COMMIT;

DO $prepublication_contract$
BEGIN
  IF (SELECT count(*) FROM knowledge_child_search_projections
      WHERE materialization_id =
        '18600000-0000-0000-0000-000000000004') <> 4096
    OR EXISTS (
      SELECT 1 FROM knowledge_child_vector_shadow_projections
      WHERE materialization_id =
        '18600000-0000-0000-0000-000000000004'
    )
    OR EXISTS (
      SELECT 1 FROM knowledge_child_bm25_shadow_projections
      WHERE materialization_id =
        '18600000-0000-0000-0000-000000000004'
    )
  THEN
    RAISE EXCEPTION 'representative corpus prepublication state mismatch';
  END IF;
END
$prepublication_contract$;

SELECT 'PASS G18.5B.2c resource corpus chunks=4096 heads=0' AS result;
