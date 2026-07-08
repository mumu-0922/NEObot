CREATE TABLE import_batches (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  idempotency_key TEXT NOT NULL,
  package_hash TEXT NOT NULL,
  manifest_hash TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  response JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ,
  rolled_back_at TIMESTAMPTZ,
  CONSTRAINT import_batches_idempotency_key_not_blank CHECK (length(trim(idempotency_key)) > 0),
  CONSTRAINT import_batches_package_hash_hex_check CHECK (package_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT import_batches_manifest_hash_hex_check CHECK (manifest_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT import_batches_status_check CHECK (status IN ('pending', 'completed', 'rolled_back')),
  CONSTRAINT import_batches_response_object CHECK (jsonb_typeof(response) = 'object'),
  CONSTRAINT import_batches_timestamps_order CHECK (updated_at >= created_at),
  CONSTRAINT import_batches_completed_after_created CHECK (completed_at IS NULL OR completed_at >= created_at),
  CONSTRAINT import_batches_rolled_back_after_created CHECK (rolled_back_at IS NULL OR rolled_back_at >= created_at)
);

CREATE UNIQUE INDEX idx_import_batches_user_idempotency ON import_batches(user_id, idempotency_key);
CREATE INDEX idx_import_batches_user_created ON import_batches(user_id, created_at DESC);
CREATE INDEX idx_import_batches_status ON import_batches(status);
