-- Restore the 023 Child-only hydration result while retaining its exact
-- content-hash and current-authority fences.

SELECT set_config(
  'search_path',
  quote_ident(current_schema()) || ', pg_catalog, pg_temp',
  false
);

DROP FUNCTION IF EXISTS knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
);

CREATE FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  p_actor_user_id UUID,
  p_session_id UUID,
  p_conversation_id UUID,
  p_references JSONB
) RETURNS TABLE(
  collection_id UUID,
  document_id UUID,
  document_version_id UUID,
  index_generation_id UUID,
  materialization_id UUID,
  parent_chunk_id UUID,
  child_chunk_id UUID,
  source_span_hash TEXT,
  content_hash TEXT,
  source_text TEXT,
  locator JSONB
)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path FROM CURRENT
AS $function$
BEGIN
  IF p_references IS NULL
    OR jsonb_typeof(p_references) <> 'array'
    OR jsonb_array_length(p_references) NOT BETWEEN 1 AND 16
    OR NOT EXISTS (
      SELECT 1 FROM sessions s
      JOIN conversations c ON c.id = p_conversation_id
      WHERE s.id = p_session_id AND s.user_id = p_actor_user_id
        AND s.revoked_at IS NULL AND s.expires_at > clock_timestamp()
        AND c.user_id = p_actor_user_id AND c.status <> 'deleted'
        AND c.deleted_at IS NULL
    )
  THEN
    RAISE EXCEPTION USING
      ERRCODE = '42501',
      MESSAGE = 'RAG_HYDRATION_NOT_AUTHORIZED';
  END IF;

  RETURN QUERY
  WITH requested AS (
    SELECT * FROM jsonb_to_recordset(p_references) AS reference(
      collection_id UUID,
      document_id UUID,
      document_version_id UUID,
      index_generation_id UUID,
      materialization_id UUID,
      parent_chunk_id UUID,
      child_chunk_id UUID,
      source_span_hash TEXT,
      content_hash TEXT
    )
  ), authorized AS (
    SELECT r.*, child.content, parent.locator_summary
    FROM requested r
    JOIN knowledge_corpus_projection_head corpus
      ON corpus.singleton_id = 1
      AND corpus.active_index_generation_id = r.index_generation_id
    JOIN knowledge_document_projection_heads head
      ON head.index_generation_id = r.index_generation_id
      AND head.document_id = r.document_id
      AND head.active_materialization_id = r.materialization_id
    JOIN knowledge_document_materializations m
      ON m.id = r.materialization_id
      AND m.collection_id = r.collection_id
      AND m.document_id = r.document_id
      AND m.document_version_id = r.document_version_id
      AND m.index_generation_id = r.index_generation_id
      AND m.status = 'published'
    JOIN knowledge_collections collection ON collection.id = r.collection_id
    JOIN knowledge_documents document
      ON document.id = r.document_id
      AND document.collection_id = collection.id
      AND document.current_version_id = r.document_version_id
      AND document.status = 'active' AND document.deleted_at IS NULL
    JOIN knowledge_document_versions version
      ON version.id = r.document_version_id
      AND version.document_id = document.id
      AND version.status = 'active'
      AND version.visibility_epoch = m.document_visibility_epoch
      AND version.content_hash = m.source_content_hash
    JOIN knowledge_parent_chunks parent
      ON parent.id = r.parent_chunk_id
      AND parent.materialization_id = r.materialization_id
    JOIN knowledge_child_chunks child
      ON child.id = r.child_chunk_id AND child.parent_chunk_id = parent.id
      AND child.materialization_id = r.materialization_id
      AND child.source_span_hash = r.source_span_hash
      AND child.content_hash = r.content_hash
    WHERE collection.deleted_at IS NULL
      AND collection.acl_revision = m.collection_acl_revision
      AND collection.visibility_epoch = m.collection_visibility_epoch
      AND collection.collection_processing_revision = m.collection_processing_revision
      AND document.visibility_epoch = m.document_visibility_epoch
      AND (
        (collection.scope = 'personal' AND collection.owner_user_id = p_actor_user_id)
        OR (
          collection.scope = 'team'
          AND EXISTS (
            SELECT 1 FROM team_memberships membership
            JOIN teams team ON team.id = membership.team_id
            WHERE membership.team_id = collection.team_id
              AND membership.user_id = p_actor_user_id
              AND membership.status = 'active' AND team.deleted_at IS NULL
          )
        )
      )
  )
  SELECT
    authorized.collection_id,
    authorized.document_id,
    authorized.document_version_id,
    authorized.index_generation_id,
    authorized.materialization_id,
    authorized.parent_chunk_id,
    authorized.child_chunk_id,
    authorized.source_span_hash,
    authorized.content_hash,
    authorized.content,
    authorized.locator_summary
  FROM authorized
  WHERE octet_length(authorized.content) <= 65536;
END
$function$;

ALTER FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) OWNER TO rag_projection_owner;
REVOKE ALL ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) TO go_evidence_hydrator, go_api_runtime;
