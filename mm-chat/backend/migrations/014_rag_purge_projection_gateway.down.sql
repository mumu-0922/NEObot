DROP FUNCTION IF EXISTS knowledge_assert_purge_complete(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID, UUID, INTEGER
);
DROP FUNCTION IF EXISTS knowledge_purge_search_projection(
  UUID, UUID, UUID, UUID, UUID, UUID, UUID, UUID
);
DROP FUNCTION IF EXISTS knowledge_mark_purge_invisible(
  UUID, UUID, UUID, UUID, UUID, UUID, BIGINT, BIGINT
);
