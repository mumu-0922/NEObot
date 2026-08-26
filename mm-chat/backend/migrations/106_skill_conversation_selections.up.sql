-- Durable per-conversation Skill selection. Inventory remains owned by
-- skill_installations; this layer only records which installed Skills a
-- conversation pins for future Runs.

ALTER TABLE skill_installations
  ADD CONSTRAINT skill_installations_id_user_fingerprint_unique
  UNIQUE (id, user_id, package_fingerprint);

CREATE TABLE skill_conversation_selections (
  conversation_id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT skill_conversation_selections_owner_unique
    UNIQUE (conversation_id, user_id),
  CONSTRAINT skill_conversation_selections_conversation_owner_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES conversations(id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT skill_conversation_selections_revision_positive CHECK (revision >= 1),
  CONSTRAINT skill_conversation_selections_timestamps_order
    CHECK (updated_at >= created_at)
);

CREATE TABLE skill_conversation_installations (
  conversation_id UUID NOT NULL,
  user_id UUID NOT NULL,
  installation_id UUID NOT NULL,
  package_fingerprint TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (conversation_id, installation_id),
  CONSTRAINT skill_conversation_installations_selection_fk
    FOREIGN KEY (conversation_id, user_id)
    REFERENCES skill_conversation_selections(conversation_id, user_id)
    ON DELETE CASCADE,
  CONSTRAINT skill_conversation_installations_inventory_fk
    FOREIGN KEY (installation_id, user_id, package_fingerprint)
    REFERENCES skill_installations(id, user_id, package_fingerprint)
    ON DELETE CASCADE,
  CONSTRAINT skill_conversation_installations_fingerprint_check
    CHECK (package_fingerprint ~ '^sha256:[0-9a-f]{64}$')
);

CREATE INDEX idx_skill_conversation_selections_user
  ON skill_conversation_selections(user_id, updated_at DESC);

CREATE INDEX idx_skill_conversation_installations_inventory
  ON skill_conversation_installations(user_id, installation_id);

GRANT SELECT, INSERT, UPDATE, DELETE
  ON TABLE skill_conversation_selections TO go_api_runtime;
GRANT SELECT, INSERT, DELETE
  ON TABLE skill_conversation_installations TO go_api_runtime;
