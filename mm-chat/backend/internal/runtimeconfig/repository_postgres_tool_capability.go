package runtimeconfig

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (r *PostgresProviderConfigRepository) GetToolCapabilityCache(
	ctx context.Context,
	providerConfigHash string,
	modelID string,
	now time.Time,
) (ToolCapabilityCacheEntry, bool, error) {
	if r == nil || r.db == nil {
		return ToolCapabilityCacheEntry{}, false, ErrDatabaseRequired
	}
	var entry ToolCapabilityCacheEntry
	err := r.db.QueryRowContext(ctx, `
SELECT provider_config_hash, model_id, status, category, checked_at, expires_at
FROM model_tool_capability_cache
WHERE provider_config_hash = $1
  AND model_id = $2
  AND expires_at > $3
`, providerConfigHash, modelID, now.UTC()).Scan(
		&entry.ProviderConfigHash,
		&entry.ModelID,
		&entry.Status,
		&entry.Category,
		&entry.CheckedAt,
		&entry.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ToolCapabilityCacheEntry{}, false, nil
	}
	if err != nil {
		return ToolCapabilityCacheEntry{}, false, fmt.Errorf("get tool capability cache: %w", err)
	}
	entry.CheckedAt = entry.CheckedAt.UTC()
	entry.ExpiresAt = entry.ExpiresAt.UTC()
	return entry, true, nil
}

func (r *PostgresProviderConfigRepository) UpsertToolCapabilityCache(
	ctx context.Context,
	entry ToolCapabilityCacheEntry,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO model_tool_capability_cache (
  provider_config_hash,
  model_id,
  status,
  category,
  checked_at,
  expires_at,
  updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $5)
ON CONFLICT (provider_config_hash, model_id) DO UPDATE SET
  status = EXCLUDED.status,
  category = EXCLUDED.category,
  checked_at = EXCLUDED.checked_at,
  expires_at = EXCLUDED.expires_at,
  updated_at = EXCLUDED.updated_at
`,
		entry.ProviderConfigHash,
		entry.ModelID,
		entry.Status,
		entry.Category,
		entry.CheckedAt.UTC(),
		entry.ExpiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("upsert tool capability cache: %w", err)
	}
	return nil
}
