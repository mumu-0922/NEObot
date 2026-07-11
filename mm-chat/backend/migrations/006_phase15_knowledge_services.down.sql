DROP INDEX IF EXISTS idx_knowledge_processing_jobs_purge_fence;
DROP INDEX IF EXISTS idx_knowledge_processing_jobs_version;
DROP INDEX IF EXISTS idx_knowledge_processing_jobs_document;
DROP INDEX IF EXISTS idx_knowledge_processing_jobs_processing;
DROP INDEX IF EXISTS idx_knowledge_processing_jobs_pending;
DROP TABLE IF EXISTS knowledge_processing_jobs;

DROP INDEX IF EXISTS idx_processing_consents_current_lookup;
ALTER TABLE IF EXISTS processing_consents
  DROP CONSTRAINT IF EXISTS processing_consents_collection_job_binding_unique;

DROP INDEX IF EXISTS idx_knowledge_document_versions_one_nonterminal;
DROP INDEX IF EXISTS idx_knowledge_document_versions_document_creator_idempotency;
ALTER TABLE IF EXISTS knowledge_document_versions
  DROP CONSTRAINT IF EXISTS knowledge_document_versions_document_id_file_unique,
  DROP CONSTRAINT IF EXISTS knowledge_document_versions_error_code_check,
  DROP CONSTRAINT IF EXISTS knowledge_document_versions_idempotency_shape_check,
  DROP COLUMN IF EXISTS error_code,
  DROP COLUMN IF EXISTS request_hash,
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS created_by_user_id;

DROP INDEX IF EXISTS idx_knowledge_documents_collection_page;
DROP INDEX IF EXISTS idx_knowledge_documents_collection_creator_idempotency;
ALTER TABLE IF EXISTS knowledge_documents
  DROP CONSTRAINT IF EXISTS knowledge_documents_collection_id_unique,
  DROP CONSTRAINT IF EXISTS knowledge_documents_idempotency_shape_check,
  DROP CONSTRAINT IF EXISTS knowledge_documents_visibility_epoch_positive,
  DROP COLUMN IF EXISTS create_request_hash,
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS created_by_user_id,
  DROP COLUMN IF EXISTS visibility_epoch;

DROP INDEX IF EXISTS idx_knowledge_collections_team_page;
DROP INDEX IF EXISTS idx_knowledge_collections_personal_page;
DROP INDEX IF EXISTS idx_knowledge_collections_creator_idempotency;
ALTER TABLE IF EXISTS knowledge_collections
  DROP CONSTRAINT IF EXISTS knowledge_collections_idempotency_shape_check,
  DROP CONSTRAINT IF EXISTS knowledge_collections_color_check,
  DROP CONSTRAINT IF EXISTS knowledge_collections_icon_check,
  DROP CONSTRAINT IF EXISTS knowledge_collections_description_bounded_check,
  DROP CONSTRAINT IF EXISTS knowledge_collections_name_bounded_check,
  DROP COLUMN IF EXISTS create_request_hash,
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS created_by_user_id,
  DROP COLUMN IF EXISTS color,
  DROP COLUMN IF EXISTS icon,
  DROP COLUMN IF EXISTS description;
