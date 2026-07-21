package runtimeconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func (r *PostgresProviderConfigRepository) ConfigureRAGProviderConnection(
	ctx context.Context,
	input ConfigureRAGProviderConnectionInput,
) (StoredProviderConfig, error) {
	if r == nil || r.db == nil {
		return StoredProviderConfig{}, ErrDatabaseRequired
	}
	if input.UserID == "" || input.ProviderID == "" || input.Label == "" ||
		input.EncryptedSecretRef == "" || input.RAGProvider == "" ||
		input.ConnectionTestSHA256 == "" || input.ConnectionTestedAt.IsZero() {
		return StoredProviderConfig{}, ErrProviderConfigUnsupported
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return StoredProviderConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()

	user := auth.UserOrDevelopment(auth.WithUser(
		context.Background(),
		auth.User{ID: input.UserID},
	))
	if _, err := tx.ExecContext(ctx, `
INSERT INTO users (id, display_name)
VALUES ($1, $2)
ON CONFLICT (id) DO NOTHING
`, user.ID, user.DisplayName); err != nil {
		return StoredProviderConfig{}, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`LOCK TABLE provider_configs IN SHARE ROW EXCLUSIVE MODE`,
	); err != nil {
		return StoredProviderConfig{}, err
	}

	var existingID string
	err = tx.QueryRowContext(ctx, `
SELECT id::text
FROM provider_configs
WHERE user_id = $1 AND provider_id = $2 AND deleted_at IS NULL
ORDER BY updated_at DESC, created_at DESC
LIMIT 1
`, input.UserID, input.ProviderID).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return StoredProviderConfig{}, err
	}
	exists := err == nil
	if input.ExpectedExists != exists {
		return StoredProviderConfig{}, ErrProviderConfigChanged
	}
	if exists {
		current, ok, err := scanProviderConfigTx(ctx, tx, existingID)
		if err != nil {
			return StoredProviderConfig{}, err
		}
		if !ok || current.ID != input.ExpectedID ||
			current.EncryptedSecretRef != input.ExpectedEncryptedSecretRef ||
			current.Config.Kind != providerConfigKindRAG ||
			current.Config.RAGProvider != input.ExpectedRAGProvider ||
			current.Config.Enabled != input.ExpectedEnabled {
			return StoredProviderConfig{}, ErrProviderConfigChanged
		}
	}

	configuredPayload := StoredProviderConfigPayload{
		Kind:                 providerConfigKindRAG,
		RAGProvider:          input.RAGProvider,
		Enabled:              true,
		ConnectionTestSHA256: input.ConnectionTestSHA256,
		ConnectionTestedAt:   input.ConnectionTestedAt.UTC().Format(time.RFC3339Nano),
	}
	encodedConfig, err := json.Marshal(configuredPayload)
	if err != nil {
		return StoredProviderConfig{}, ErrProviderConfigUnsupported
	}

	if !exists {
		idGenerator := r.newID
		if idGenerator == nil {
			idGenerator = newUUID
		}
		existingID, err = idGenerator()
		if err != nil {
			return StoredProviderConfig{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO provider_configs (
  id, user_id, provider_id, label, encrypted_secret_ref, config
)
VALUES ($1, $2, $3, $4, $5, $6::jsonb)
`, existingID, input.UserID, input.ProviderID, input.Label,
			input.EncryptedSecretRef, string(encodedConfig)); err != nil {
			return StoredProviderConfig{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `
UPDATE provider_configs
SET label = $2,
    encrypted_secret_ref = $3,
    config = $4::jsonb,
    updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
`, existingID, input.Label, input.EncryptedSecretRef, string(encodedConfig)); err != nil {
		return StoredProviderConfig{}, err
	}

	stored, ok, err := scanProviderConfigTx(ctx, tx, existingID)
	if err != nil {
		return StoredProviderConfig{}, err
	}
	if !ok {
		return StoredProviderConfig{}, ErrProviderConfigUnsupported
	}
	if err := tx.Commit(); err != nil {
		return StoredProviderConfig{}, err
	}
	return stored, nil
}
