\set ON_ERROR_STOP on

SELECT set_config('mm_chat.g18_expected_major', :'expected_major', false);

DO $restore_contract$
DECLARE
  migration_count INTEGER;
  authority_count INTEGER;
  projection_count INTEGER;
BEGIN
  IF current_setting('server_version_num')::integer / 10000
      <> current_setting('mm_chat.g18_expected_major')::integer THEN
    RAISE EXCEPTION 'expected PostgreSQL %, got %',
      current_setting('mm_chat.g18_expected_major'), version();
  END IF;

  SELECT count(*) INTO migration_count
  FROM schema_migrations
  WHERE version BETWEEN 1 AND 36
    AND length(checksum) = 64;
  IF migration_count <> 36
    OR (SELECT min(version) FROM schema_migrations) <> 1
    OR (SELECT max(version) FROM schema_migrations) <> 36 THEN
    RAISE EXCEPTION 'migration manifest is incomplete';
  END IF;

  SELECT count(*) INTO authority_count
  FROM knowledge_collections collection
  JOIN knowledge_documents document
    ON document.collection_id = collection.id
  JOIN knowledge_document_versions version
    ON version.document_id = document.id
    AND version.id = document.current_version_id
  JOIN files source_file ON source_file.id = version.file_id
  WHERE collection.id = '18180000-0000-0000-0000-000000000003'
    AND collection.deleted_at IS NULL
    AND document.status = 'active'
    AND version.status = 'active'
    AND source_file.object_key = 'g18/synthetic/source/g18-restore-fixture.pdf';
  IF authority_count <> 1 THEN
    RAISE EXCEPTION 'authoritative Knowledge graph did not restore';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_parser_artifact_sets artifact_set
    JOIN knowledge_parser_artifacts artifact
      ON artifact.artifact_set_id = artifact_set.id
    WHERE artifact_set.id = '18180000-0000-0000-0000-000000000008'
      AND artifact.object_key = 'g18/synthetic/artifacts/canonical-ir.json'
      AND artifact.sha256 = repeat('f', 64)
  ) THEN
    RAISE EXCEPTION 'parser object reference did not restore';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM knowledge_corpus_projection_head head
    JOIN knowledge_index_generations generation
      ON generation.id = head.active_index_generation_id
    JOIN knowledge_projection_state state
      ON state.index_generation_id = generation.id
    WHERE head.singleton_id = 1
      AND generation.id = '18180000-0000-0000-0000-000000000007'
      AND generation.status = 'active'
      AND state.readiness = 'ready'
      AND state.document_count = 1
      AND state.parent_count = 1
      AND state.child_count = 1
  ) THEN
    RAISE EXCEPTION 'active generation head/state did not restore';
  END IF;

  SELECT count(*) INTO projection_count
  FROM knowledge_document_projection_heads document_head
  JOIN knowledge_document_materializations materialization
    ON materialization.id = document_head.active_materialization_id
  JOIN knowledge_parent_chunks parent
    ON parent.materialization_id = materialization.id
  JOIN knowledge_child_chunks child
    ON child.parent_chunk_id = parent.id
  JOIN knowledge_child_search_projections search
    ON search.child_chunk_id = child.id
  WHERE document_head.index_generation_id =
      '18180000-0000-0000-0000-000000000007'
    AND document_head.document_id =
      '18180000-0000-0000-0000-000000000004'
    AND materialization.status = 'published'
    AND search.status = 'ready'
    AND search.embedding_model_id = 'jina-embeddings-v4'
    AND cardinality(search.embedding_vector) = 1024
    AND search.exact_terms @> ARRAY['G18_RESTORE']::text[];
  IF projection_count <> 1 THEN
    RAISE EXCEPTION 'published projection graph did not restore';
  END IF;
END
$restore_contract$;

\if :expect_extensions
DO $extension_contract$
BEGIN
  IF current_setting('shared_preload_libraries') !~ '(^|,)pg_textsearch(,|$)' THEN
    RAISE EXCEPTION 'pg_textsearch is not preloaded after restore';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'pg_textsearch')
      <> '1.3.1' THEN
    RAISE EXCEPTION 'restored database lacks pg_textsearch 1.3.1';
  END IF;
  IF (SELECT extversion FROM pg_extension WHERE extname = 'vector')
      <> '0.8.5' THEN
    RAISE EXCEPTION 'restored database lacks vector 0.8.5';
  END IF;
END
$extension_contract$;
\endif

SELECT format(
  'PASS PG%s migrations=36 authority=1 objects=2 generation=active projection=ready',
  :expected_major
) AS result;
