-- Revert G7.5N parse completion finalizer.

REVOKE EXECUTE ON FUNCTION knowledge_complete_parse_and_enqueue_embedding(
  UUID, UUID, UUID, UUID, UUID
) FROM rag_worker_executor;
DROP FUNCTION IF EXISTS knowledge_complete_parse_and_enqueue_embedding(
  UUID, UUID, UUID, UUID, UUID
);
REVOKE SELECT ON
  processor_governance_profiles,
  processor_governance_heads,
  processing_consents
FROM rag_projection_owner;
