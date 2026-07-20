CREATE TABLE conversation_context_summaries (
  conversation_id UUID PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE,
  version INTEGER NOT NULL DEFAULT 1,
  model_provider TEXT NOT NULL DEFAULT '',
  model_id TEXT NOT NULL DEFAULT '',
  source_first_message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  source_last_message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  source_message_count INTEGER NOT NULL,
  source_digest TEXT NOT NULL,
  summary TEXT NOT NULL,
  estimated_source_tokens INTEGER NOT NULL DEFAULT 0,
  estimated_summary_tokens INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT conversation_context_summaries_version_positive CHECK (version > 0),
  CONSTRAINT conversation_context_summaries_source_count_positive CHECK (source_message_count > 0),
  CONSTRAINT conversation_context_summaries_source_digest_sha256 CHECK (source_digest ~ '^[0-9a-f]{64}$'),
  CONSTRAINT conversation_context_summaries_summary_bounded CHECK (
    length(trim(summary)) > 0 AND octet_length(summary) <= 65536
  ),
  CONSTRAINT conversation_context_summaries_token_estimates_non_negative CHECK (
    estimated_source_tokens >= 0 AND estimated_summary_tokens >= 0
  ),
  CONSTRAINT conversation_context_summaries_timestamps_order CHECK (updated_at >= created_at)
);

CREATE INDEX idx_conversation_context_summaries_last_message
  ON conversation_context_summaries(source_last_message_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON conversation_context_summaries
TO go_api_runtime;
