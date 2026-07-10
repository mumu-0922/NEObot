ALTER TABLE teams
  ADD COLUMN idempotency_key TEXT;

ALTER TABLE teams
  ADD CONSTRAINT teams_idempotency_key_optional_bounded_check CHECK (
    idempotency_key IS NULL
    OR (
      octet_length(idempotency_key) BETWEEN 1 AND 128
      AND length(trim(idempotency_key)) > 0
    )
  );

ALTER TABLE team_invites
  ADD COLUMN idempotency_key TEXT;

ALTER TABLE team_invites
  ADD CONSTRAINT team_invites_idempotency_key_optional_bounded_check CHECK (
    idempotency_key IS NULL
    OR (
      octet_length(idempotency_key) BETWEEN 1 AND 128
      AND length(trim(idempotency_key)) > 0
    )
  );

CREATE UNIQUE INDEX idx_teams_created_by_idempotency
  ON teams(created_by_user_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX idx_team_invites_team_inviter_idempotency
  ON team_invites(team_id, invited_by_user_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX idx_team_invites_pending_team_email
  ON team_invites(team_id, email)
  WHERE status = 'pending';

ALTER TABLE team_invites
  ADD CONSTRAINT team_invites_team_id_id_unique UNIQUE (team_id, id);

CREATE TABLE identity_mail_outbox (
  id UUID PRIMARY KEY,
  team_id UUID NOT NULL,
  invite_id UUID NOT NULL UNIQUE,
  key_id TEXT NOT NULL,
  payload_version INTEGER NOT NULL,
  nonce BYTEA NOT NULL,
  ciphertext BYTEA NOT NULL,
  message_id TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'pending',
  attempt_count INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 8,
  available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_owner UUID,
  lease_expires_at TIMESTAMPTZ,
  retry_at TIMESTAMPTZ,
  terminal_at TIMESTAMPTZ,
  error_code TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT identity_mail_outbox_invite_fk
    FOREIGN KEY (team_id, invite_id)
    REFERENCES team_invites(team_id, id)
    ON DELETE CASCADE,
  CONSTRAINT identity_mail_outbox_key_id_format
    CHECK (key_id ~ '^[A-Za-z0-9._-]{1,64}$'),
  CONSTRAINT identity_mail_outbox_payload_version_positive
    CHECK (payload_version BETWEEN 1 AND 32767),
  CONSTRAINT identity_mail_outbox_nonce_bytes
    CHECK (octet_length(nonce) = 12),
  CONSTRAINT identity_mail_outbox_key_nonce_unique
    UNIQUE (key_id, nonce),
  CONSTRAINT identity_mail_outbox_ciphertext_not_empty
    CHECK (octet_length(ciphertext) > 0),
  CONSTRAINT identity_mail_outbox_message_id_safe
    CHECK (
      octet_length(message_id) BETWEEN 1 AND 998
      AND position(chr(10) IN message_id) = 0
      AND position(chr(13) IN message_id) = 0
    ),
  CONSTRAINT identity_mail_outbox_status_check
    CHECK (status IN ('pending', 'processing', 'sent', 'failed', 'cancelled')),
  CONSTRAINT identity_mail_outbox_attempt_count_non_negative
    CHECK (attempt_count >= 0),
  CONSTRAINT identity_mail_outbox_max_attempts_range
    CHECK (max_attempts BETWEEN 1 AND 32),
  CONSTRAINT identity_mail_outbox_attempts_within_max
    CHECK (attempt_count <= max_attempts),
  CONSTRAINT identity_mail_outbox_error_code_sanitized
    CHECK (error_code IS NULL OR error_code ~ '^[A-Z0-9_]{1,64}$'),
  CONSTRAINT identity_mail_outbox_retry_alignment
    CHECK (retry_at IS NULL OR retry_at = available_at),
  CONSTRAINT identity_mail_outbox_status_timestamps_check CHECK (
    (
      status = 'pending'
      AND lease_owner IS NULL
      AND lease_expires_at IS NULL
      AND terminal_at IS NULL
      AND attempt_count < max_attempts
    )
    OR (
      status = 'processing'
      AND lease_owner IS NOT NULL
      AND lease_expires_at IS NOT NULL
      AND terminal_at IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'sent'
      AND lease_owner IS NULL
      AND lease_expires_at IS NULL
      AND retry_at IS NULL
      AND terminal_at IS NOT NULL
      AND error_code IS NULL
      AND attempt_count BETWEEN 1 AND max_attempts
    )
    OR (
      status = 'failed'
      AND lease_owner IS NULL
      AND lease_expires_at IS NULL
      AND retry_at IS NULL
      AND terminal_at IS NOT NULL
      AND error_code IS NOT NULL
      AND attempt_count = max_attempts
    )
    OR (
      status = 'cancelled'
      AND lease_owner IS NULL
      AND lease_expires_at IS NULL
      AND retry_at IS NULL
      AND terminal_at IS NOT NULL
      AND error_code IS NULL
    )
  ),
  CONSTRAINT identity_mail_outbox_available_after_created
    CHECK (available_at >= created_at),
  CONSTRAINT identity_mail_outbox_lease_after_created
    CHECK (lease_expires_at IS NULL OR lease_expires_at >= created_at),
  CONSTRAINT identity_mail_outbox_retry_after_created
    CHECK (retry_at IS NULL OR retry_at >= created_at),
  CONSTRAINT identity_mail_outbox_terminal_after_created
    CHECK (terminal_at IS NULL OR terminal_at >= created_at),
  CONSTRAINT identity_mail_outbox_timestamps_order
    CHECK (updated_at >= created_at)
);

CREATE INDEX idx_identity_mail_outbox_pending
  ON identity_mail_outbox(available_at, id)
  WHERE status = 'pending';

CREATE INDEX idx_identity_mail_outbox_processing
  ON identity_mail_outbox(lease_expires_at, id)
  WHERE status = 'processing';

ALTER TABLE team_memberships
  DROP CONSTRAINT team_memberships_user_id_fkey;

ALTER TABLE team_memberships
  ADD CONSTRAINT team_memberships_user_id_fkey
  FOREIGN KEY (user_id)
  REFERENCES users(id)
  ON DELETE RESTRICT;
