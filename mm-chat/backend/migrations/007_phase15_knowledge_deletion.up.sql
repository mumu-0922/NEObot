CREATE UNIQUE INDEX IF NOT EXISTS idx_knowledge_processing_jobs_purge_fence
  ON knowledge_processing_jobs(
    document_id,
    document_version_id,
    document_visibility_epoch
  )
  WHERE stage = 'purge' AND operation = 'purge';
