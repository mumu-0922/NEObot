DROP INDEX IF EXISTS idx_knowledge_outbox_aggregate;
DROP INDEX IF EXISTS idx_knowledge_outbox_pending;
DROP TABLE IF EXISTS knowledge_outbox;

DROP INDEX IF EXISTS idx_processing_consents_profile;
DROP INDEX IF EXISTS idx_processing_consents_query_revision;
DROP INDEX IF EXISTS idx_processing_consents_collection_revision;
DROP INDEX IF EXISTS idx_processing_consents_current_query_processor;
DROP INDEX IF EXISTS idx_processing_consents_current_collection_processor;
DROP TABLE IF EXISTS processing_consents;

DROP TABLE IF EXISTS processor_governance_heads;
DROP TABLE IF EXISTS processor_governance_profiles;

DROP TABLE IF EXISTS user_query_consent_state;

ALTER TABLE IF EXISTS knowledge_documents
  DROP CONSTRAINT IF EXISTS knowledge_documents_active_version_status_fk;

ALTER TABLE IF EXISTS knowledge_documents
  DROP CONSTRAINT IF EXISTS knowledge_documents_current_version_same_document_fk;

DROP INDEX IF EXISTS idx_knowledge_document_versions_document_status;
DROP INDEX IF EXISTS idx_knowledge_document_versions_file;
DROP TABLE IF EXISTS knowledge_document_versions;

DROP INDEX IF EXISTS idx_knowledge_documents_collection_status;
DROP TABLE IF EXISTS knowledge_documents;

DROP INDEX IF EXISTS idx_knowledge_collections_team_active;
DROP INDEX IF EXISTS idx_knowledge_collections_personal_active;
DROP TABLE IF EXISTS knowledge_collections;

DROP INDEX IF EXISTS idx_team_invites_pending_expiry;
DROP INDEX IF EXISTS idx_team_invites_team_created;
DROP TABLE IF EXISTS team_invites;

DROP INDEX IF EXISTS idx_team_memberships_team_active_role;
DROP INDEX IF EXISTS idx_team_memberships_user_active;
DROP TABLE IF EXISTS team_memberships;

DROP TABLE IF EXISTS teams;

DROP INDEX IF EXISTS idx_credential_recovery_tokens_active_expiry;
DROP INDEX IF EXISTS idx_credential_recovery_tokens_user_created;
DROP TABLE IF EXISTS credential_recovery_tokens;

DROP TABLE IF EXISTS user_credentials;

DROP INDEX IF EXISTS idx_users_email_case_insensitive;

ALTER TABLE IF EXISTS users
  DROP CONSTRAINT IF EXISTS users_account_status_check;

ALTER TABLE IF EXISTS users
  DROP COLUMN IF EXISTS account_status;
