CREATE TABLE model_tool_capability_cache (
  provider_config_hash TEXT NOT NULL,
  model_id TEXT NOT NULL,
  status TEXT NOT NULL,
  category TEXT NOT NULL DEFAULT '',
  checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (provider_config_hash, model_id),
  CONSTRAINT model_tool_capability_hash_shape CHECK (
    provider_config_hash ~ '^[0-9a-f]{64}$'
  ),
  CONSTRAINT model_tool_capability_model_bounded CHECK (
    length(trim(model_id)) BETWEEN 1 AND 512
  ),
  CONSTRAINT model_tool_capability_status_check CHECK (
    status IN ('supported', 'unsupported', 'unknown')
  ),
  CONSTRAINT model_tool_capability_category_bounded CHECK (
    char_length(category) <= 64
  ),
  CONSTRAINT model_tool_capability_expiry_order CHECK (
    expires_at > checked_at
  ),
  CONSTRAINT model_tool_capability_updated_order CHECK (
    updated_at >= checked_at
  )
);

CREATE INDEX idx_model_tool_capability_expiry
  ON model_tool_capability_cache(expires_at);

GRANT SELECT, INSERT, UPDATE, DELETE
  ON model_tool_capability_cache TO go_api_runtime;
