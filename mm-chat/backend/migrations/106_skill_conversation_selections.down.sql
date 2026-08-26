DROP INDEX IF EXISTS idx_skill_conversation_installations_inventory;
DROP INDEX IF EXISTS idx_skill_conversation_selections_user;
DROP TABLE IF EXISTS skill_conversation_installations;
DROP TABLE IF EXISTS skill_conversation_selections;

ALTER TABLE skill_installations
  DROP CONSTRAINT IF EXISTS skill_installations_id_user_fingerprint_unique;
