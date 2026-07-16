package runtimeconfig

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"

	"neo-chat/mm-chat/backend/internal/auth"
)

type PostgresProviderConfigRepository struct {
	db    *sql.DB
	newID func() (string, error)
}

func NewPostgresProviderConfigRepository(db *sql.DB) *PostgresProviderConfigRepository {
	return &PostgresProviderConfigRepository{db: db, newID: newUUID}
}

func (r *PostgresProviderConfigRepository) GetProviderConfig(
	ctx context.Context,
	userID string,
	providerID string,
) (StoredProviderConfig, bool, error) {
	if r == nil || r.db == nil {
		return StoredProviderConfig{}, false, ErrDatabaseRequired
	}
	var stored StoredProviderConfig
	var encodedConfig []byte
	var secretRef sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT id::text, user_id::text, provider_id, label, encrypted_secret_ref, config
FROM provider_configs
WHERE user_id = $1 AND provider_id = $2 AND deleted_at IS NULL
ORDER BY updated_at DESC, created_at DESC
LIMIT 1
`, userID, providerID).Scan(
		&stored.ID,
		&stored.UserID,
		&stored.ProviderID,
		&stored.Label,
		&secretRef,
		&encodedConfig,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return StoredProviderConfig{}, false, nil
		}
		return StoredProviderConfig{}, false, err
	}
	stored.EncryptedSecretRef = secretRef.String
	if len(encodedConfig) > 0 {
		if err := json.Unmarshal(encodedConfig, &stored.Config); err != nil {
			return StoredProviderConfig{}, false, ErrProviderConfigUnsupported
		}
	}
	return stored, true, nil
}

func (r *PostgresProviderConfigRepository) UpsertProviderConfig(
	ctx context.Context,
	input UpsertProviderConfigInput,
) (StoredProviderConfig, error) {
	if r == nil || r.db == nil {
		return StoredProviderConfig{}, ErrDatabaseRequired
	}
	providerID := input.ProviderID
	if providerID == "" {
		providerID = serverDefaultProviderID
	}
	idGenerator := r.newID
	if idGenerator == nil {
		idGenerator = newUUID
	}
	encodedConfig, err := json.Marshal(input.Config)
	if err != nil {
		return StoredProviderConfig{}, ErrProviderConfigUnsupported
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return StoredProviderConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()

	user := auth.UserOrDevelopment(auth.WithUser(context.Background(), auth.User{ID: input.UserID}))
	if _, err := tx.ExecContext(ctx, `
INSERT INTO users (id, display_name)
VALUES ($1, $2)
ON CONFLICT (id) DO NOTHING
`, user.ID, user.DisplayName); err != nil {
		return StoredProviderConfig{}, err
	}
	if _, err := tx.ExecContext(ctx, `LOCK TABLE provider_configs IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return StoredProviderConfig{}, err
	}

	var existingID string
	err = tx.QueryRowContext(ctx, `
SELECT id::text
FROM provider_configs
WHERE user_id = $1 AND provider_id = $2 AND deleted_at IS NULL
ORDER BY updated_at DESC, created_at DESC
LIMIT 1
`, user.ID, providerID).Scan(&existingID)
	if err != nil && err != sql.ErrNoRows {
		return StoredProviderConfig{}, err
	}
	if err == sql.ErrNoRows {
		existingID, err = idGenerator()
		if err != nil {
			return StoredProviderConfig{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO provider_configs (id, user_id, provider_id, label, encrypted_secret_ref, config)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6::jsonb)
`, existingID, user.ID, providerID, input.Label, input.EncryptedSecretRef, string(encodedConfig)); err != nil {
			return StoredProviderConfig{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
UPDATE provider_configs
SET label = $2,
    encrypted_secret_ref = NULLIF($3, ''),
    config = $4::jsonb,
    updated_at = now()
WHERE id = $1
`, existingID, input.Label, input.EncryptedSecretRef, string(encodedConfig)); err != nil {
			return StoredProviderConfig{}, err
		}
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

func scanProviderConfigTx(ctx context.Context, tx *sql.Tx, id string) (StoredProviderConfig, bool, error) {
	var stored StoredProviderConfig
	var encodedConfig []byte
	var secretRef sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT id::text, user_id::text, provider_id, label, encrypted_secret_ref, config
FROM provider_configs
WHERE id = $1 AND deleted_at IS NULL
`, id).Scan(
		&stored.ID,
		&stored.UserID,
		&stored.ProviderID,
		&stored.Label,
		&secretRef,
		&encodedConfig,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return StoredProviderConfig{}, false, nil
		}
		return StoredProviderConfig{}, false, err
	}
	stored.EncryptedSecretRef = secretRef.String
	if len(encodedConfig) > 0 {
		if err := json.Unmarshal(encodedConfig, &stored.Config); err != nil {
			return StoredProviderConfig{}, false, ErrProviderConfigUnsupported
		}
	}
	return stored, true, nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}
