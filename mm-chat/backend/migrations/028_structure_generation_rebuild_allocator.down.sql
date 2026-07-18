DROP FUNCTION knowledge_begin_structure_generation_rebuild(
  UUID,UUID,UUID,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB
);
REVOKE SELECT, INSERT ON knowledge_index_profiles FROM rag_projection_owner;
