package knowledge

import (
	"context"
	"database/sql"
	"fmt"
)

type runtimeSchemaCapabilities struct {
	exactModelIdentity      bool
	legacyProjectionUnbound bool
	jobLeaseToken           bool
}

func loadRuntimeSchemaCapabilities(ctx context.Context, tx *sql.Tx) (runtimeSchemaCapabilities, error) {
	var capabilities runtimeSchemaCapabilities
	err := tx.QueryRowContext(ctx, `
SELECT
  EXISTS (
    SELECT 1 FROM pg_attribute
    WHERE attrelid='processor_governance_profiles'::regclass
      AND attname='model_id' AND NOT attisdropped
  ) AND EXISTS (
    SELECT 1 FROM pg_attribute
    WHERE attrelid='processor_governance_profiles'::regclass
      AND attname='profile_contract_hash' AND NOT attisdropped
  ) AND EXISTS (
    SELECT 1 FROM pg_attribute
    WHERE attrelid='processor_governance_heads'::regclass
      AND attname='model_id' AND NOT attisdropped
  ) AND EXISTS (
    SELECT 1 FROM pg_attribute
    WHERE attrelid='processing_consents'::regclass
      AND attname='model_id' AND NOT attisdropped
  ),
  EXISTS (
    SELECT 1 FROM pg_attribute
    WHERE attrelid='knowledge_processing_jobs'::regclass
      AND attname='legacy_projection_unbound' AND NOT attisdropped
  ),
  EXISTS (
    SELECT 1 FROM pg_attribute
    WHERE attrelid='knowledge_processing_jobs'::regclass
      AND attname='lease_token' AND NOT attisdropped
  )
`).Scan(&capabilities.exactModelIdentity, &capabilities.legacyProjectionUnbound, &capabilities.jobLeaseToken)
	if err != nil {
		return runtimeSchemaCapabilities{}, fmt.Errorf("detect knowledge schema capabilities: %w", err)
	}
	return capabilities, nil
}
