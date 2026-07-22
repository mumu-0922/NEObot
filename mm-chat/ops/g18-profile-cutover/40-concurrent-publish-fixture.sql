\set ON_ERROR_STOP on

BEGIN;
SET CONSTRAINTS ALL DEFERRED;

INSERT INTO files (
  id, user_id, original_filename, mime_type, byte_size, sha256, object_key
) VALUES
  (
    '18400000-0000-0000-0000-000000000001',
    '18180000-0000-0000-0000-000000000001',
    'g18-live-alpha.pdf',
    'application/pdf',
    128,
    repeat('6', 64),
    'g18/synthetic/source/g18-live-alpha.pdf'
  ),
  (
    '18400000-0000-0000-0000-000000000011',
    '18180000-0000-0000-0000-000000000001',
    'g18-live-beta.pdf',
    'application/pdf',
    128,
    repeat('7', 64),
    'g18/synthetic/source/g18-live-beta.pdf'
  );

INSERT INTO knowledge_documents (
  id, collection_id, status, visibility_epoch, created_by_user_id
) VALUES
  (
    '18400000-0000-0000-0000-000000000002',
    '18180000-0000-0000-0000-000000000003',
    'processing',
    1,
    '18180000-0000-0000-0000-000000000001'
  ),
  (
    '18400000-0000-0000-0000-000000000012',
    '18180000-0000-0000-0000-000000000003',
    'processing',
    1,
    '18180000-0000-0000-0000-000000000001'
  );

INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, visibility_epoch, status,
  content_hash, created_by_user_id
) VALUES
  (
    '18400000-0000-0000-0000-000000000003',
    '18400000-0000-0000-0000-000000000002',
    '18400000-0000-0000-0000-000000000001',
    1,
    1,
    'active',
    repeat('8', 64),
    '18180000-0000-0000-0000-000000000001'
  ),
  (
    '18400000-0000-0000-0000-000000000013',
    '18400000-0000-0000-0000-000000000012',
    '18400000-0000-0000-0000-000000000011',
    1,
    1,
    'active',
    repeat('9', 64),
    '18180000-0000-0000-0000-000000000001'
  );

UPDATE knowledge_documents
SET status = 'active',
    current_version_id = CASE id
      WHEN '18400000-0000-0000-0000-000000000002'::UUID
        THEN '18400000-0000-0000-0000-000000000003'::UUID
      WHEN '18400000-0000-0000-0000-000000000012'::UUID
        THEN '18400000-0000-0000-0000-000000000013'::UUID
    END,
    updated_at = clock_timestamp()
WHERE id IN (
  '18400000-0000-0000-0000-000000000002'::UUID,
  '18400000-0000-0000-0000-000000000012'::UUID
);

INSERT INTO knowledge_document_materializations (
  id, index_generation_id, collection_id, document_id, document_version_id,
  file_id, materialization_seq, source_content_hash, base_profile_hash,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch, status,
  manifest_hash, result_hash, verified_at, published_at
) VALUES
  (
    '18400000-0000-0000-0000-000000000004',
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000003',
    '18400000-0000-0000-0000-000000000002',
    '18400000-0000-0000-0000-000000000003',
    '18400000-0000-0000-0000-000000000001',
    1,
    repeat('8', 64),
    repeat('e', 64),
    1,
    1,
    1,
    1,
    'published',
    repeat('a', 64),
    repeat('b', 64),
    clock_timestamp(),
    clock_timestamp()
  ),
  (
    '18400000-0000-0000-0000-000000000014',
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000003',
    '18400000-0000-0000-0000-000000000012',
    '18400000-0000-0000-0000-000000000013',
    '18400000-0000-0000-0000-000000000011',
    1,
    repeat('9', 64),
    repeat('e', 64),
    1,
    1,
    1,
    1,
    'published',
    repeat('c', 64),
    repeat('d', 64),
    clock_timestamp(),
    clock_timestamp()
  );

INSERT INTO knowledge_parent_chunks (
  id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count, heading_path, locator_summary
) VALUES
  (
    '18400000-0000-0000-0000-000000000005',
    '18400000-0000-0000-0000-000000000004',
    '18180000-0000-0000-0000-000000000007',
    '18400000-0000-0000-0000-000000000002',
    '18400000-0000-0000-0000-000000000003',
    0,
    repeat('d', 64),
    repeat('a', 64),
    repeat('b', 64),
    'Synthetic concurrently published alpha parent.',
    7,
    ARRAY['G18', 'Live Alpha']::TEXT[],
    '{"kind":"page","page":1}'::JSONB
  ),
  (
    '18400000-0000-0000-0000-000000000015',
    '18400000-0000-0000-0000-000000000014',
    '18180000-0000-0000-0000-000000000007',
    '18400000-0000-0000-0000-000000000012',
    '18400000-0000-0000-0000-000000000013',
    0,
    repeat('d', 64),
    repeat('c', 64),
    repeat('d', 64),
    'Synthetic concurrently published beta parent.',
    7,
    ARRAY['G18', 'Live Beta']::TEXT[],
    '{"kind":"page","page":2}'::JSONB
  );

INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
) VALUES
  (
    '18400000-0000-0000-0000-000000000006',
    '18400000-0000-0000-0000-000000000005',
    '18400000-0000-0000-0000-000000000004',
    '18180000-0000-0000-0000-000000000007',
    '18400000-0000-0000-0000-000000000002',
    '18400000-0000-0000-0000-000000000003',
    0,
    repeat('d', 64),
    repeat('a', 64),
    repeat('b', 64),
    'LIVE_ALPHA concurrent publication contract.',
    6
  ),
  (
    '18400000-0000-0000-0000-000000000016',
    '18400000-0000-0000-0000-000000000015',
    '18400000-0000-0000-0000-000000000014',
    '18180000-0000-0000-0000-000000000007',
    '18400000-0000-0000-0000-000000000012',
    '18400000-0000-0000-0000-000000000013',
    0,
    repeat('d', 64),
    repeat('c', 64),
    repeat('d', 64),
    'LIVE_BETA concurrent publication contract.',
    6
  );

INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
) VALUES
  (
    '18400000-0000-0000-0000-000000000006',
    '18400000-0000-0000-0000-000000000005',
    '18400000-0000-0000-0000-000000000004',
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000003',
    '18400000-0000-0000-0000-000000000002',
    '18400000-0000-0000-0000-000000000003',
    '18180000-0000-0000-0000-000000000011',
    'jina-embeddings-v4',
    1024,
    ARRAY[0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1020]),
    repeat('6', 64),
    'LIVE_ALPHA concurrent publication contract.',
    ARRAY['LIVE_ALPHA']::TEXT[],
    repeat('a', 64),
    repeat('d', 64),
    repeat('b', 64),
    '{"kind":"page","page":1}'::JSONB,
    'ready',
    clock_timestamp()
  ),
  (
    '18400000-0000-0000-0000-000000000016',
    '18400000-0000-0000-0000-000000000015',
    '18400000-0000-0000-0000-000000000014',
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000003',
    '18400000-0000-0000-0000-000000000012',
    '18400000-0000-0000-0000-000000000013',
    '18180000-0000-0000-0000-000000000011',
    'jina-embeddings-v4',
    1024,
    ARRAY[0::REAL, 0::REAL, 0::REAL, 0::REAL, 1::REAL] ||
      array_fill(0::REAL, ARRAY[1019]),
    repeat('7', 64),
    'LIVE_BETA concurrent publication contract.',
    ARRAY['LIVE_BETA']::TEXT[],
    repeat('c', 64),
    repeat('d', 64),
    repeat('d', 64),
    '{"kind":"page","page":2}'::JSONB,
    'ready',
    clock_timestamp()
  );

COMMIT;

DO $prepublication_contract$
BEGIN
  IF EXISTS (
    SELECT 1 FROM knowledge_child_vector_shadow_projections
    WHERE child_chunk_id IN (
      '18400000-0000-0000-0000-000000000006'::UUID,
      '18400000-0000-0000-0000-000000000016'::UUID
    )
  ) OR EXISTS (
    SELECT 1 FROM knowledge_child_bm25_shadow_projections
    WHERE child_chunk_id IN (
      '18400000-0000-0000-0000-000000000006'::UUID,
      '18400000-0000-0000-0000-000000000016'::UUID
    )
  ) THEN
    RAISE EXCEPTION 'unpublished heads entered PG17 projections';
  END IF;
END
$prepublication_contract$;

SELECT 'PASS G18.5B.2a concurrent publication fixture staged=2 heads=0' AS result;
