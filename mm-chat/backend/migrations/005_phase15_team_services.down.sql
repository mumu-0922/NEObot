DROP INDEX IF EXISTS idx_identity_mail_outbox_processing;
DROP INDEX IF EXISTS idx_identity_mail_outbox_pending;
DROP TABLE IF EXISTS identity_mail_outbox;

DROP INDEX IF EXISTS idx_team_invites_pending_team_email;
DROP INDEX IF EXISTS idx_team_invites_team_inviter_idempotency;
DROP INDEX IF EXISTS idx_teams_created_by_idempotency;

ALTER TABLE IF EXISTS team_invites
  DROP CONSTRAINT IF EXISTS team_invites_team_id_id_unique;

ALTER TABLE IF EXISTS team_invites
  DROP CONSTRAINT IF EXISTS team_invites_idempotency_key_optional_bounded_check;

ALTER TABLE IF EXISTS team_invites
  DROP COLUMN IF EXISTS idempotency_key;

ALTER TABLE IF EXISTS teams
  DROP CONSTRAINT IF EXISTS teams_idempotency_key_optional_bounded_check;

ALTER TABLE IF EXISTS teams
  DROP COLUMN IF EXISTS idempotency_key;

ALTER TABLE team_memberships
  DROP CONSTRAINT team_memberships_user_id_fkey;

ALTER TABLE team_memberships
  ADD CONSTRAINT team_memberships_user_id_fkey
  FOREIGN KEY (user_id)
  REFERENCES users(id)
  ON DELETE CASCADE;
