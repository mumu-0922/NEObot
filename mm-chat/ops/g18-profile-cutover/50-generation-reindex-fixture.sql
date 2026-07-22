\set ON_ERROR_STOP on

BEGIN;
SET CONSTRAINTS ALL DEFERRED;

INSERT INTO knowledge_index_generations (
  id, index_profile_id, generation_seq, status, build_snapshot,
  build_snapshot_hash
) VALUES (
  '18500000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000006',
  2,
  'building',
  jsonb_build_object(
    'schemaVersion', 'g11.9d-structure-rebuild-snapshot.v1',
    'sourceGenerationId', '18180000-0000-0000-0000-000000000007'
  ),
  repeat('5', 64)
);

INSERT INTO knowledge_projection_state (
  index_generation_id, readiness, projection_revision,
  required_outbox_floor, contiguous_applied_outbox_id
) VALUES (
  '18500000-0000-0000-0000-000000000007',
  'building',
  1,
  0,
  0
);

WITH source_map(
  source_materialization_id,
  candidate_materialization_id
) AS (
  VALUES
    (
      '18180000-0000-0000-0000-000000000010'::UUID,
      '18500000-0000-0000-0000-000000000010'::UUID
    ),
    (
      '18300000-0000-0000-0000-000000000015'::UUID,
      '18500000-0000-0000-0000-000000000020'::UUID
    ),
    (
      '18400000-0000-0000-0000-000000000014'::UUID,
      '18500000-0000-0000-0000-000000000030'::UUID
    )
)
INSERT INTO knowledge_document_materializations (
  id, index_generation_id, collection_id, document_id, document_version_id,
  file_id, materialization_seq, parse_artifact_set_id, source_content_hash,
  base_profile_hash, collection_acl_revision, collection_visibility_epoch,
  collection_processing_revision, document_visibility_epoch, status,
  manifest_hash, result_hash, verified_at, published_at
)
SELECT
  source_map.candidate_materialization_id,
  '18500000-0000-0000-0000-000000000007'::UUID,
  materialization.collection_id,
  materialization.document_id,
  materialization.document_version_id,
  materialization.file_id,
  1,
  materialization.parse_artifact_set_id,
  materialization.source_content_hash,
  materialization.base_profile_hash,
  materialization.collection_acl_revision,
  materialization.collection_visibility_epoch,
  materialization.collection_processing_revision,
  materialization.document_visibility_epoch,
  'published',
  materialization.manifest_hash,
  materialization.result_hash,
  clock_timestamp(),
  clock_timestamp()
FROM source_map
JOIN knowledge_document_materializations materialization
  ON materialization.id = source_map.source_materialization_id;

WITH source_map(
  source_parent_id,
  candidate_parent_id,
  candidate_materialization_id
) AS (
  VALUES
    (
      '18180000-0000-0000-0000-000000000012'::UUID,
      '18500000-0000-0000-0000-000000000011'::UUID,
      '18500000-0000-0000-0000-000000000010'::UUID
    ),
    (
      '18300000-0000-0000-0000-000000000016'::UUID,
      '18500000-0000-0000-0000-000000000021'::UUID,
      '18500000-0000-0000-0000-000000000020'::UUID
    ),
    (
      '18400000-0000-0000-0000-000000000015'::UUID,
      '18500000-0000-0000-0000-000000000031'::UUID,
      '18500000-0000-0000-0000-000000000030'::UUID
    )
)
INSERT INTO knowledge_parent_chunks (
  id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count, heading_path, locator_summary
)
SELECT
  source_map.candidate_parent_id,
  source_map.candidate_materialization_id,
  '18500000-0000-0000-0000-000000000007'::UUID,
  parent.document_id,
  parent.document_version_id,
  parent.ordinal,
  parent.chunk_profile_hash,
  parent.source_span_hash,
  parent.content_hash,
  parent.content,
  parent.token_count,
  parent.heading_path,
  parent.locator_summary
FROM source_map
JOIN knowledge_parent_chunks parent ON parent.id = source_map.source_parent_id;

WITH source_map(
  source_child_id,
  candidate_child_id,
  candidate_parent_id,
  candidate_materialization_id
) AS (
  VALUES
    (
      '18180000-0000-0000-0000-000000000013'::UUID,
      '18500000-0000-0000-0000-000000000012'::UUID,
      '18500000-0000-0000-0000-000000000011'::UUID,
      '18500000-0000-0000-0000-000000000010'::UUID
    ),
    (
      '18300000-0000-0000-0000-000000000017'::UUID,
      '18500000-0000-0000-0000-000000000022'::UUID,
      '18500000-0000-0000-0000-000000000021'::UUID,
      '18500000-0000-0000-0000-000000000020'::UUID
    ),
    (
      '18400000-0000-0000-0000-000000000016'::UUID,
      '18500000-0000-0000-0000-000000000032'::UUID,
      '18500000-0000-0000-0000-000000000031'::UUID,
      '18500000-0000-0000-0000-000000000030'::UUID
    )
)
INSERT INTO knowledge_child_chunks (
  id, parent_chunk_id, materialization_id, index_generation_id, document_id,
  document_version_id, ordinal, chunk_profile_hash, source_span_hash,
  content_hash, content, token_count
)
SELECT
  source_map.candidate_child_id,
  source_map.candidate_parent_id,
  source_map.candidate_materialization_id,
  '18500000-0000-0000-0000-000000000007'::UUID,
  child.document_id,
  child.document_version_id,
  child.ordinal,
  child.chunk_profile_hash,
  child.source_span_hash,
  child.content_hash,
  child.content,
  child.token_count
FROM source_map
JOIN knowledge_child_chunks child ON child.id = source_map.source_child_id;

WITH source_map(
  source_child_id,
  candidate_child_id,
  candidate_parent_id,
  candidate_materialization_id
) AS (
  VALUES
    (
      '18180000-0000-0000-0000-000000000013'::UUID,
      '18500000-0000-0000-0000-000000000012'::UUID,
      '18500000-0000-0000-0000-000000000011'::UUID,
      '18500000-0000-0000-0000-000000000010'::UUID
    ),
    (
      '18300000-0000-0000-0000-000000000017'::UUID,
      '18500000-0000-0000-0000-000000000022'::UUID,
      '18500000-0000-0000-0000-000000000021'::UUID,
      '18500000-0000-0000-0000-000000000020'::UUID
    ),
    (
      '18400000-0000-0000-0000-000000000016'::UUID,
      '18500000-0000-0000-0000-000000000032'::UUID,
      '18500000-0000-0000-0000-000000000031'::UUID,
      '18500000-0000-0000-0000-000000000030'::UUID
    )
)
INSERT INTO knowledge_child_search_projections (
  child_chunk_id, parent_chunk_id, materialization_id, index_generation_id,
  collection_id, document_id, document_version_id, search_profile_id,
  embedding_model_id, embedding_dimensions, embedding_vector,
  embedding_vector_sha256, lexical_text, exact_terms, source_span_hash,
  chunk_profile_hash, content_hash, locator_summary, status, ready_at
)
SELECT
  source_map.candidate_child_id,
  source_map.candidate_parent_id,
  source_map.candidate_materialization_id,
  '18500000-0000-0000-0000-000000000007'::UUID,
  search.collection_id,
  search.document_id,
  search.document_version_id,
  search.search_profile_id,
  search.embedding_model_id,
  search.embedding_dimensions,
  search.embedding_vector,
  search.embedding_vector_sha256,
  search.lexical_text,
  search.exact_terms,
  search.source_span_hash,
  search.chunk_profile_hash,
  search.content_hash,
  search.locator_summary,
  'ready',
  clock_timestamp()
FROM source_map
JOIN knowledge_child_search_projections search
  ON search.child_chunk_id = source_map.source_child_id;

-- Publish only the first candidate head. The active PG17 maintenance trigger
-- must populate it even though its generation is still building.
INSERT INTO knowledge_document_projection_heads (
  index_generation_id, document_id, active_materialization_id,
  last_corpus_projection_revision
) VALUES (
  '18500000-0000-0000-0000-000000000007',
  '18180000-0000-0000-0000-000000000004',
  '18500000-0000-0000-0000-000000000010',
  2
);

COMMIT;

SELECT 'PASS G18.5B.2b reindex fixture building_heads=1 expected_documents=3'
  AS result;
