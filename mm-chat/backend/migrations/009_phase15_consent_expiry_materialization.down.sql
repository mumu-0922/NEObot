DROP INDEX IF EXISTS idx_processing_consents_expiry_due;
ALTER TABLE processing_consents
  DROP CONSTRAINT IF EXISTS processing_consents_expiry_materialized_shape;
ALTER TABLE processing_consents
  DROP COLUMN IF EXISTS expiry_materialized_at;
