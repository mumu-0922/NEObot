-- Server-owned Assistant library and explicit Marketplace admission snapshots.
-- Assistants are bounded prompt presets only; no model, credential, Tool, or
-- executable capability is stored here.

CREATE TABLE assistant_library_entries (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source TEXT NOT NULL,
  source_identifier TEXT,
  avatar TEXT NOT NULL DEFAULT '🤖',
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT 'general',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  system_prompt TEXT NOT NULL,
  author TEXT NOT NULL DEFAULT '',
  homepage TEXT NOT NULL DEFAULT '',
  source_version TEXT NOT NULL DEFAULT '',
  source_updated_at TEXT NOT NULL DEFAULT '',
  required_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
  content_fingerprint TEXT NOT NULL,
  revision BIGINT NOT NULL DEFAULT 1,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT assistant_library_source_check
    CHECK (source IN ('custom', 'lobehub')),
  CONSTRAINT assistant_library_source_shape CHECK (
    (source = 'custom' AND source_identifier IS NULL)
    OR (source = 'lobehub' AND length(trim(source_identifier)) > 0)
  ),
  CONSTRAINT assistant_library_title_not_blank CHECK (length(trim(title)) > 0),
  CONSTRAINT assistant_library_prompt_not_blank CHECK (length(trim(system_prompt)) > 0),
  CONSTRAINT assistant_library_tags_array CHECK (jsonb_typeof(tags) = 'array'),
  CONSTRAINT assistant_library_required_tools_array
    CHECK (jsonb_typeof(required_tools) = 'array'),
  CONSTRAINT assistant_library_fingerprint_check
    CHECK (content_fingerprint ~ '^[0-9a-f]{64}$'),
  CONSTRAINT assistant_library_revision_positive CHECK (revision >= 1),
  CONSTRAINT assistant_library_timestamps_order CHECK (updated_at >= created_at)
);

CREATE INDEX idx_assistant_library_user_updated
  ON assistant_library_entries(user_id, updated_at DESC, id DESC);

-- Custom Assistants intentionally have no source identifier and may coexist.
-- Store snapshots remain unique per account and upstream identifier.
CREATE UNIQUE INDEX idx_assistant_library_user_store_unique
  ON assistant_library_entries(user_id, source, source_identifier)
  WHERE source = 'lobehub';

CREATE TABLE assistant_market_admissions (
  source TEXT NOT NULL DEFAULT 'lobehub',
  source_identifier TEXT NOT NULL,
  status TEXT NOT NULL,
  avatar TEXT NOT NULL DEFAULT '🤖',
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT 'general',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  system_prompt TEXT NOT NULL,
  author TEXT NOT NULL DEFAULT '',
  homepage TEXT NOT NULL DEFAULT '',
  source_version TEXT NOT NULL DEFAULT '',
  source_updated_at TEXT NOT NULL DEFAULT '',
  required_tools JSONB NOT NULL DEFAULT '[]'::jsonb,
  content_fingerprint TEXT NOT NULL,
  reviewed_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (source, source_identifier),
  CONSTRAINT assistant_admission_source_check CHECK (source = 'lobehub'),
  CONSTRAINT assistant_admission_identifier_not_blank
    CHECK (length(trim(source_identifier)) > 0),
  CONSTRAINT assistant_admission_status_check
    CHECK (status IN ('admitted', 'rejected')),
  CONSTRAINT assistant_admission_title_not_blank CHECK (length(trim(title)) > 0),
  CONSTRAINT assistant_admission_prompt_not_blank CHECK (length(trim(system_prompt)) > 0),
  CONSTRAINT assistant_admission_tags_array CHECK (jsonb_typeof(tags) = 'array'),
  CONSTRAINT assistant_admission_required_tools_array
    CHECK (jsonb_typeof(required_tools) = 'array'),
  CONSTRAINT assistant_admission_fingerprint_check
    CHECK (content_fingerprint ~ '^[0-9a-f]{64}$'),
  CONSTRAINT assistant_admission_timestamps_order CHECK (updated_at >= created_at)
);

CREATE INDEX idx_assistant_admission_browse
  ON assistant_market_admissions(status, category, updated_at DESC, source_identifier);

GRANT SELECT, INSERT, UPDATE, DELETE
  ON TABLE assistant_library_entries, assistant_market_admissions
  TO go_api_runtime;
