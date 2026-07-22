-- Keep Go API document lifecycle writes outside projection-owner tables.
--
-- The API supplies only immutable entity identifiers. Each gateway derives
-- projection metadata from authoritative rows and executes the minimum
-- projection-owner operation required by upload, reprocess, or deletion.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

CREATE FUNCTION knowledge_allocate_parse_materialization(
  p_materialization_id UUID,
  p_document_id UUID,
  p_document_version_id UUID
) RETURNS TABLE(
  index_generation_id UUID,
  materialization_id UUID,
  legacy_projection_unbound BOOLEAN,
  max_attempts INTEGER
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  authoritative_collection_id UUID;
  authoritative_file_id UUID;
  authoritative_source_content_hash TEXT;
  authoritative_collection_acl_revision BIGINT;
  authoritative_collection_visibility_epoch BIGINT;
  authoritative_collection_processing_revision BIGINT;
  authoritative_document_visibility_epoch BIGINT;
  active_generation_id UUID;
  active_base_profile_hash TEXT;
  next_materialization_seq BIGINT;
BEGIN
  IF p_materialization_id IS NULL
    OR p_document_id IS NULL
    OR p_document_version_id IS NULL
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_API_PARSE_MATERIALIZATION_ARGUMENT_INVALID';
  END IF;

  SELECT
    document.collection_id,
    version.file_id,
    version.content_hash,
    collection.acl_revision,
    collection.visibility_epoch,
    collection.collection_processing_revision,
    version.visibility_epoch
  INTO
    authoritative_collection_id,
    authoritative_file_id,
    authoritative_source_content_hash,
    authoritative_collection_acl_revision,
    authoritative_collection_visibility_epoch,
    authoritative_collection_processing_revision,
    authoritative_document_visibility_epoch
  FROM knowledge_documents document
  JOIN knowledge_document_versions version
    ON version.document_id = document.id
    AND version.id = p_document_version_id
    AND version.status IN ('uploaded', 'processing', 'failed', 'active')
  JOIN knowledge_collections collection
    ON collection.id = document.collection_id
    AND collection.deleted_at IS NULL
  JOIN files source_file
    ON source_file.id = version.file_id
    AND source_file.upload_status = 'available'
    AND source_file.deleted_at IS NULL
  WHERE document.id = p_document_id
    AND document.status IN ('processing', 'active')
    AND document.deleted_at IS NULL;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_API_PARSE_MATERIALIZATION_AUTHORITY_MISSING';
  END IF;

  SELECT generation.id, profile.base_profile_hash
  INTO active_generation_id, active_base_profile_hash
  FROM knowledge_corpus_projection_head head
  JOIN knowledge_index_generations generation
    ON generation.id = head.active_index_generation_id
    AND generation.status = 'active'
  JOIN knowledge_index_profiles profile
    ON profile.id = generation.index_profile_id
  WHERE head.singleton_id = 1
  FOR UPDATE OF head;
  IF NOT FOUND THEN
    RETURN QUERY SELECT NULL::UUID, NULL::UUID, true, 8;
    RETURN;
  END IF;

  SELECT COALESCE(max(materialization.materialization_seq), 0) + 1
  INTO next_materialization_seq
  FROM knowledge_document_materializations materialization
  WHERE materialization.index_generation_id = active_generation_id
    AND materialization.document_id = p_document_id;

  INSERT INTO knowledge_document_materializations (
    id, index_generation_id, collection_id, document_id, document_version_id,
    file_id, materialization_seq, source_content_hash, base_profile_hash,
    collection_acl_revision, collection_visibility_epoch,
    collection_processing_revision, document_visibility_epoch, status
  ) VALUES (
    p_materialization_id,
    active_generation_id,
    authoritative_collection_id,
    p_document_id,
    p_document_version_id,
    authoritative_file_id,
    next_materialization_seq,
    authoritative_source_content_hash,
    active_base_profile_hash,
    authoritative_collection_acl_revision,
    authoritative_collection_visibility_epoch,
    authoritative_collection_processing_revision,
    authoritative_document_visibility_epoch,
    'staging'
  );

  RETURN QUERY SELECT
    active_generation_id,
    p_materialization_id,
    false,
    3;
END
$function$;

CREATE FUNCTION knowledge_is_document_version_actively_projected(
  p_document_id UUID,
  p_document_version_id UUID
) RETURNS BOOLEAN
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
  SELECT p_document_id IS NOT NULL
    AND p_document_version_id IS NOT NULL
    AND EXISTS (
      SELECT 1
      FROM knowledge_corpus_projection_head corpus
      JOIN knowledge_index_generations generation
        ON generation.id = corpus.active_index_generation_id
        AND generation.status = 'active'
      JOIN knowledge_document_projection_heads head
        ON head.index_generation_id = generation.id
        AND head.document_id = p_document_id
      JOIN knowledge_document_materializations materialization
        ON materialization.id = head.active_materialization_id
        AND materialization.index_generation_id = generation.id
        AND materialization.document_id = p_document_id
        AND materialization.document_version_id = p_document_version_id
        AND materialization.status = 'published'
      JOIN knowledge_documents document
        ON document.id = p_document_id
        AND document.current_version_id = p_document_version_id
        AND document.status = 'active'
        AND document.deleted_at IS NULL
      JOIN knowledge_document_versions version
        ON version.document_id = document.id
        AND version.id = p_document_version_id
        AND version.status = 'active'
      WHERE corpus.singleton_id = 1
    )
$function$;

CREATE FUNCTION knowledge_resolve_purge_projection_binding(
  p_document_id UUID,
  p_document_version_id UUID
) RETURNS TABLE(
  index_generation_id UUID,
  materialization_id UUID,
  legacy_projection_unbound BOOLEAN,
  max_attempts INTEGER
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
DECLARE
  active_generation_id UUID;
  active_materialization_id UUID;
BEGIN
  IF p_document_id IS NULL OR p_document_version_id IS NULL THEN
    RAISE EXCEPTION USING
      ERRCODE = '22023',
      MESSAGE = 'RAG_API_PURGE_BINDING_ARGUMENT_INVALID';
  END IF;

  PERFORM 1
  FROM knowledge_document_versions version
  WHERE version.document_id = p_document_id
    AND version.id = p_document_version_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION USING
      ERRCODE = '55000',
      MESSAGE = 'RAG_API_PURGE_BINDING_AUTHORITY_MISSING';
  END IF;

  SELECT corpus.active_index_generation_id, materialization.id
  INTO active_generation_id, active_materialization_id
  FROM knowledge_corpus_projection_head corpus
  JOIN knowledge_index_generations generation
    ON generation.id = corpus.active_index_generation_id
    AND generation.status = 'active'
  LEFT JOIN knowledge_document_projection_heads head
    ON head.index_generation_id = generation.id
    AND head.document_id = p_document_id
  LEFT JOIN knowledge_document_materializations materialization
    ON materialization.id = head.active_materialization_id
    AND materialization.index_generation_id = generation.id
    AND materialization.document_id = p_document_id
    AND materialization.document_version_id = p_document_version_id
  WHERE corpus.singleton_id = 1
  FOR UPDATE OF corpus;
  IF NOT FOUND THEN
    RETURN QUERY SELECT NULL::UUID, NULL::UUID, true, 8;
    RETURN;
  END IF;

  RETURN QUERY SELECT
    active_generation_id,
    active_materialization_id,
    false,
    3;
END
$function$;

ALTER FUNCTION knowledge_allocate_parse_materialization(UUID, UUID, UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_is_document_version_actively_projected(UUID, UUID)
  OWNER TO rag_projection_owner;
ALTER FUNCTION knowledge_resolve_purge_projection_binding(UUID, UUID)
  OWNER TO rag_projection_owner;

REVOKE ALL ON FUNCTION knowledge_allocate_parse_materialization(
  UUID, UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_is_document_version_actively_projected(
  UUID, UUID
) FROM PUBLIC;
REVOKE ALL ON FUNCTION knowledge_resolve_purge_projection_binding(
  UUID, UUID
) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION knowledge_allocate_parse_materialization(
  UUID, UUID, UUID
) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_is_document_version_actively_projected(
  UUID, UUID
) TO go_api_runtime;
GRANT EXECUTE ON FUNCTION knowledge_resolve_purge_projection_binding(
  UUID, UUID
) TO go_api_runtime;
