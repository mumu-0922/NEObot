ALTER TABLE processing_consents
  ADD COLUMN expiry_materialized_at TIMESTAMPTZ;

ALTER TABLE processing_consents
  ADD CONSTRAINT processing_consents_expiry_materialized_shape CHECK (
    expiry_materialized_at IS NULL
    OR (
      decision = 'granted'
      AND expires_at IS NOT NULL
      AND expiry_materialized_at >= expires_at
    )
  );

CREATE INDEX idx_processing_consents_expiry_due
  ON processing_consents(expires_at, id)
  WHERE superseded_at IS NULL
    AND decision = 'granted'
    AND expires_at IS NOT NULL
    AND expiry_materialized_at IS NULL;
