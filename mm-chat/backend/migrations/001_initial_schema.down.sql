-- Drop application-created indexes before dependent tables. Unique constraint
-- backing indexes are removed automatically with their owning tables.
DROP INDEX IF EXISTS idx_audit_logs_request_id;
DROP INDEX IF EXISTS idx_audit_logs_outcome_created;
DROP INDEX IF EXISTS idx_audit_logs_resource;
DROP INDEX IF EXISTS idx_audit_logs_session_created;
DROP INDEX IF EXISTS idx_audit_logs_actor_created;
DROP INDEX IF EXISTS idx_audit_logs_created_at;

DROP INDEX IF EXISTS idx_message_attachments_user_id;
DROP INDEX IF EXISTS idx_message_attachments_file_id;

DROP INDEX IF EXISTS idx_messages_conversation_idempotency;
DROP INDEX IF EXISTS idx_messages_parent_message_id;
DROP INDEX IF EXISTS idx_messages_user_id;
DROP INDEX IF EXISTS idx_messages_status;
DROP INDEX IF EXISTS idx_messages_conversation_role;
DROP INDEX IF EXISTS idx_messages_conversation_created;

DROP INDEX IF EXISTS idx_files_user_active;
DROP INDEX IF EXISTS idx_files_upload_status;
DROP INDEX IF EXISTS idx_files_sha256;
DROP INDEX IF EXISTS idx_files_user_created;

DROP INDEX IF EXISTS idx_conversations_user_idempotency;
DROP INDEX IF EXISTS idx_conversations_status;
DROP INDEX IF EXISTS idx_conversations_user_active;
DROP INDEX IF EXISTS idx_conversations_user_updated;

DROP INDEX IF EXISTS idx_provider_configs_user_active;
DROP INDEX IF EXISTS idx_provider_configs_user_provider;

DROP INDEX IF EXISTS idx_sessions_active_user;
DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_user_id;

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS message_attachments;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS provider_configs;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;

-- No custom enum/type objects are created by 001_initial_schema.up.sql.
