\set ON_ERROR_STOP on

BEGIN;
SET CONSTRAINTS ALL DEFERRED;

UPDATE knowledge_child_search_projections
SET embedding_vector =
      ARRAY[1::REAL] || array_fill(0::REAL, ARRAY[1023]),
    embedding_vector_sha256 = repeat('1', 64)
WHERE child_chunk_id = '18180000-0000-0000-0000-000000000013';

INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
) VALUES
  (
    '18300000-0000-0000-0000-000000000001',
    '18180000-0000-0000-0000-000000000012',
    '18180000-0000-0000-0000-000000000010',
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000004',
    '18180000-0000-0000-0000-000000000005',
    1,
    repeat('d', 64),
    repeat('1', 64),
    repeat('2', 64),
    'Synthetic secondary semantic match.',
    5
  ),
  (
    '18300000-0000-0000-0000-000000000002',
    '18180000-0000-0000-0000-000000000012',
    '18180000-0000-0000-0000-000000000010',
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000004',
    '18180000-0000-0000-0000-000000000005',
    2,
    repeat('d', 64),
    repeat('2', 64),
    repeat('3', 64),
    'Synthetic unrelated semantic candidate.',
    5
  );

INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
) VALUES
  (
    '18300000-0000-0000-0000-000000000001',
    '18180000-0000-0000-0000-000000000012',
    '18180000-0000-0000-0000-000000000010',
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000003',
    '18180000-0000-0000-0000-000000000004',
    '18180000-0000-0000-0000-000000000005',
    '18180000-0000-0000-0000-000000000011',
    'jina-embeddings-v4',
    1024,
    ARRAY[0.8::REAL, 0.6::REAL] || array_fill(0::REAL, ARRAY[1022]),
    repeat('2', 64),
    'Synthetic secondary semantic match.',
    ARRAY['G18_SECONDARY']::TEXT[],
    repeat('1', 64),
    repeat('d', 64),
    repeat('2', 64),
    '{"kind":"page","page":2}'::JSONB,
    'ready',
    clock_timestamp()
  ),
  (
    '18300000-0000-0000-0000-000000000002',
    '18180000-0000-0000-0000-000000000012',
    '18180000-0000-0000-0000-000000000010',
    '18180000-0000-0000-0000-000000000007',
    '18180000-0000-0000-0000-000000000003',
    '18180000-0000-0000-0000-000000000004',
    '18180000-0000-0000-0000-000000000005',
    '18180000-0000-0000-0000-000000000011',
    'jina-embeddings-v4',
    1024,
    ARRAY[0::REAL, 1::REAL] || array_fill(0::REAL, ARRAY[1022]),
    repeat('3', 64),
    'Synthetic unrelated semantic candidate.',
    ARRAY['G18_UNRELATED']::TEXT[],
    repeat('2', 64),
    repeat('d', 64),
    repeat('3', 64),
    '{"kind":"page","page":3}'::JSONB,
    'ready',
    clock_timestamp()
  );

INSERT INTO users (id, email, display_name)
VALUES (
  '18300000-0000-0000-0000-000000000010',
  'g18-shadow-other@example.test',
  'G18 Shadow Other Owner'
);

INSERT INTO files (
  id, user_id, original_filename, mime_type, byte_size, sha256, object_key
) VALUES (
  '18300000-0000-0000-0000-000000000011',
  '18300000-0000-0000-0000-000000000010',
  'g18-shadow-other.pdf',
  'application/pdf',
  128,
  repeat('4', 64),
  'g18/synthetic/source/g18-shadow-other.pdf'
);

INSERT INTO knowledge_collections (
  id, name, scope, owner_user_id, created_by_user_id
) VALUES (
  '18300000-0000-0000-0000-000000000012',
  'G18 Shadow Other Collection',
  'personal',
  '18300000-0000-0000-0000-000000000010',
  '18300000-0000-0000-0000-000000000010'
);

INSERT INTO knowledge_documents (
  id, collection_id, status, visibility_epoch, created_by_user_id
) VALUES (
  '18300000-0000-0000-0000-000000000013',
  '18300000-0000-0000-0000-000000000012',
  'processing',
  1,
  '18300000-0000-0000-0000-000000000010'
);

INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, visibility_epoch, status,
  content_hash, created_by_user_id
) VALUES (
  '18300000-0000-0000-0000-000000000014',
  '18300000-0000-0000-0000-000000000013',
  '18300000-0000-0000-0000-000000000011',
  1,
  1,
  'active',
  repeat('5', 64),
  '18300000-0000-0000-0000-000000000010'
);

UPDATE knowledge_documents
SET current_version_id = '18300000-0000-0000-0000-000000000014',
    status = 'active',
    updated_at = clock_timestamp()
WHERE id = '18300000-0000-0000-0000-000000000013';

INSERT INTO knowledge_document_materializations (
  id, index_generation_id, collection_id, document_id, document_version_id,
  file_id, materialization_seq, source_content_hash, base_profile_hash,
  collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch, status,
  manifest_hash, result_hash, verified_at, published_at
) VALUES (
  '18300000-0000-0000-0000-000000000015',
  '18180000-0000-0000-0000-000000000007',
  '18300000-0000-0000-0000-000000000012',
  '18300000-0000-0000-0000-000000000013',
  '18300000-0000-0000-0000-000000000014',
  '18300000-0000-0000-0000-000000000011',
  1,
  repeat('5', 64),
  repeat('e', 64),
  1,
  1,
  1,
  1,
  'published',
  repeat('6', 64),
  repeat('7', 64),
  clock_timestamp(),
  clock_timestamp()
);

INSERT INTO knowledge_document_projection_heads (
  index_generation_id, document_id, active_materialization_id,
  last_corpus_projection_revision
) VALUES (
  '18180000-0000-0000-0000-000000000007',
  '18300000-0000-0000-0000-000000000013',
  '18300000-0000-0000-0000-000000000015',
  2
);

INSERT INTO knowledge_parent_chunks (
  id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count, heading_path, locator_summary
) VALUES (
  '18300000-0000-0000-0000-000000000016',
  '18300000-0000-0000-0000-000000000015',
  '18180000-0000-0000-0000-000000000007',
  '18300000-0000-0000-0000-000000000013',
  '18300000-0000-0000-0000-000000000014',
  0,
  repeat('d', 64),
  repeat('3', 64),
  repeat('4', 64),
  'Synthetic parent owned by another collection.',
  7,
  ARRAY['G18', 'Other']::TEXT[],
  '{"kind":"page","page":1}'::JSONB
);

INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
) VALUES (
  '18300000-0000-0000-0000-000000000017',
  '18300000-0000-0000-0000-000000000016',
  '18300000-0000-0000-0000-000000000015',
  '18180000-0000-0000-0000-000000000007',
  '18300000-0000-0000-0000-000000000013',
  '18300000-0000-0000-0000-000000000014',
  0,
  repeat('d', 64),
  repeat('3', 64),
  repeat('4', 64),
  'Synthetic match owned by another collection.',
  7
);

INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
) VALUES (
  '18300000-0000-0000-0000-000000000017',
  '18300000-0000-0000-0000-000000000016',
  '18300000-0000-0000-0000-000000000015',
  '18180000-0000-0000-0000-000000000007',
  '18300000-0000-0000-0000-000000000012',
  '18300000-0000-0000-0000-000000000013',
  '18300000-0000-0000-0000-000000000014',
  '18180000-0000-0000-0000-000000000011',
  'jina-embeddings-v4',
  1024,
  ARRAY[0.95::REAL, 0.3122499::REAL] ||
    array_fill(0::REAL, ARRAY[1022]),
  repeat('4', 64),
  'Synthetic match owned by another collection.',
  ARRAY['G18_OTHER_OWNER']::TEXT[],
  repeat('3', 64),
  repeat('d', 64),
  repeat('4', 64),
  '{"kind":"page","page":1}'::JSONB,
  'ready',
  clock_timestamp()
);

UPDATE knowledge_projection_state
SET document_count = 2,
    parent_count = 2,
    child_count = 4,
    updated_at = clock_timestamp()
WHERE index_generation_id = '18180000-0000-0000-0000-000000000007';

COMMIT;

SELECT 'PASS G18.3 synthetic pgvector fixture rows=4 collections=2' AS result;
