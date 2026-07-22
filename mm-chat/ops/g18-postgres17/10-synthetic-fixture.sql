\set ON_ERROR_STOP on

BEGIN;
SET CONSTRAINTS ALL DEFERRED;

INSERT INTO users (id, email, display_name)
VALUES (
  '18180000-0000-0000-0000-000000000001',
  'g18-restore@example.test',
  'G18 Restore Fixture'
);

INSERT INTO files (
  id, user_id, original_filename, mime_type, byte_size, sha256, object_key
) VALUES (
  '18180000-0000-0000-0000-000000000002',
  '18180000-0000-0000-0000-000000000001',
  'g18-restore-fixture.pdf',
  'application/pdf',
  128,
  repeat('a', 64),
  'g18/synthetic/source/g18-restore-fixture.pdf'
);

INSERT INTO knowledge_collections (
  id, name, scope, owner_user_id, created_by_user_id
) VALUES (
  '18180000-0000-0000-0000-000000000003',
  'G18 Restore Fixture',
  'personal',
  '18180000-0000-0000-0000-000000000001',
  '18180000-0000-0000-0000-000000000001'
);

INSERT INTO knowledge_documents (
  id, collection_id, status, visibility_epoch, created_by_user_id
) VALUES (
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000003',
  'processing',
  1,
  '18180000-0000-0000-0000-000000000001'
);

INSERT INTO knowledge_document_versions (
  id, document_id, file_id, source_version, visibility_epoch, status,
  content_hash, created_by_user_id
) VALUES (
  '18180000-0000-0000-0000-000000000005',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000002',
  1,
  1,
  'active',
  repeat('b', 64),
  '18180000-0000-0000-0000-000000000001'
);

UPDATE knowledge_documents
SET current_version_id = '18180000-0000-0000-0000-000000000005',
    status = 'active',
    updated_at = clock_timestamp()
WHERE id = '18180000-0000-0000-0000-000000000004';

INSERT INTO knowledge_index_profiles (
  id, contract_version, canonical_schema_version, parser_manifest,
  parser_manifest_hash, chunk_manifest, chunk_profile_hash,
  embedding_processor, embedding_endpoint_id, embedding_model_id,
  embedding_api_version, embedding_role, rerank_processor,
  rerank_endpoint_id, rerank_model_id, rerank_api_version, base_profile_hash
) VALUES (
  '18180000-0000-0000-0000-000000000006',
  1,
  'canonical-ir-v2',
  '{}'::jsonb,
  repeat('c', 64),
  '{}'::jsonb,
  repeat('d', 64),
  'jina',
  'admin-env',
  'jina-embeddings-v4',
  'v1',
  'passage',
  'jina',
  'admin-env',
  'jina-reranker-v3',
  'v1',
  repeat('e', 64)
);

INSERT INTO knowledge_index_generations (
  id, index_profile_id, generation_seq, status, build_snapshot,
  build_snapshot_hash, artifact_manifest_hash, verified_at, activated_at
) VALUES (
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000006',
  1,
  'active',
  '{}'::jsonb,
  repeat('a', 64),
  repeat('b', 64),
  clock_timestamp(),
  clock_timestamp()
);

UPDATE knowledge_corpus_projection_head
SET active_index_generation_id = '18180000-0000-0000-0000-000000000007',
    corpus_projection_revision = 2,
    head_revision = 2,
    updated_at = clock_timestamp()
WHERE singleton_id = 1;

INSERT INTO knowledge_projection_state (
  index_generation_id, readiness, projection_revision, required_outbox_floor,
  contiguous_applied_outbox_id, manifest_hash, document_count, parent_count,
  child_count, verified_at
) VALUES (
  '18180000-0000-0000-0000-000000000007',
  'ready',
  1,
  0,
  0,
  repeat('c', 64),
  1,
  1,
  1,
  clock_timestamp()
);

INSERT INTO knowledge_parser_artifact_sets (
  id, document_id, document_version_id, file_id, index_profile_id,
  parser_kind, parser_version, source_content_hash, config_hash,
  manifest_hash, status, quality_report, verified_at
) VALUES (
  '18180000-0000-0000-0000-000000000008',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  '18180000-0000-0000-0000-000000000002',
  '18180000-0000-0000-0000-000000000006',
  'synthetic',
  'g18-v1',
  repeat('b', 64),
  repeat('c', 64),
  repeat('d', 64),
  'verified',
  '{"synthetic":true}'::jsonb,
  clock_timestamp()
);

INSERT INTO knowledge_parser_artifacts (
  id, artifact_set_id, artifact_kind, object_key, content_type, byte_size,
  sha256
) VALUES (
  '18180000-0000-0000-0000-000000000009',
  '18180000-0000-0000-0000-000000000008',
  'canonical_ir',
  'g18/synthetic/artifacts/canonical-ir.json',
  'application/json',
  256,
  repeat('f', 64)
);

INSERT INTO knowledge_document_materializations (
  id, index_generation_id, collection_id, document_id, document_version_id,
  file_id, materialization_seq, parse_artifact_set_id, source_content_hash,
  base_profile_hash, collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch, status,
  manifest_hash, result_hash, verified_at, published_at
) VALUES (
  '18180000-0000-0000-0000-000000000010',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000003',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  '18180000-0000-0000-0000-000000000002',
  1,
  '18180000-0000-0000-0000-000000000008',
  repeat('b', 64),
  repeat('e', 64),
  1,
  1,
  1,
  1,
  'published',
  repeat('d', 64),
  repeat('f', 64),
  clock_timestamp(),
  clock_timestamp()
);

INSERT INTO knowledge_document_projection_heads (
  index_generation_id, document_id, active_materialization_id,
  last_corpus_projection_revision
) VALUES (
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000010',
  2
);

INSERT INTO knowledge_search_profiles (
  id, index_profile_id, provider_profile_id, embedding_processor,
  embedding_model_id, embedding_dimensions, rerank_processor, rerank_model_id,
  lexical_config, exact_config, profile_hash
) VALUES (
  '18180000-0000-0000-0000-000000000011',
  '18180000-0000-0000-0000-000000000006',
  'mineru_jina_postgres_v1',
  'jina',
  'jina-embeddings-v4',
  1024,
  'jina',
  'jina-reranker-v3',
  '{}'::jsonb,
  '{}'::jsonb,
  repeat('0', 64)
);

INSERT INTO knowledge_parent_chunks (
  id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count, heading_path, locator_summary
) VALUES (
  '18180000-0000-0000-0000-000000000012',
  '18180000-0000-0000-0000-000000000010',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  0,
  repeat('d', 64),
  repeat('e', 64),
  repeat('f', 64),
  'Synthetic parent for the G18 restore drill.',
  9,
  ARRAY['G18','Restore']::text[],
  '{"kind":"page","page":1}'::jsonb
);

INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
) VALUES (
  '18180000-0000-0000-0000-000000000013',
  '18180000-0000-0000-0000-000000000012',
  '18180000-0000-0000-0000-000000000010',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  0,
  repeat('d', 64),
  repeat('e', 64),
  repeat('f', 64),
  'Synthetic child for the G18 restore drill.',
  9
);

INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
) VALUES (
  '18180000-0000-0000-0000-000000000013',
  '18180000-0000-0000-0000-000000000012',
  '18180000-0000-0000-0000-000000000010',
  '18180000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000003',
  '18180000-0000-0000-0000-000000000004',
  '18180000-0000-0000-0000-000000000005',
  '18180000-0000-0000-0000-000000000011',
  'jina-embeddings-v4',
  1024,
  array_fill(0.001::real, ARRAY[1024]),
  repeat('a', 64),
  'Synthetic child for the G18 restore drill.',
  ARRAY['G18_RESTORE','synthetic']::text[],
  repeat('e', 64),
  repeat('d', 64),
  repeat('f', 64),
  '{"kind":"page","page":1}'::jsonb,
  'ready',
  clock_timestamp()
);

COMMIT;

SELECT 'PASS synthetic PG16 authority/projection fixture' AS result;
