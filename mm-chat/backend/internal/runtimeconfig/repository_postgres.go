package runtimeconfig

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

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

func (r *PostgresProviderConfigRepository) ListProviderConfigs(
	ctx context.Context,
	userID string,
) ([]StoredProviderConfig, error) {
	if r == nil || r.db == nil {
		return nil, ErrDatabaseRequired
	}
	rows, err := r.db.QueryContext(ctx, `
SELECT id::text, user_id::text, provider_id, label, encrypted_secret_ref, config
FROM provider_configs
WHERE user_id = $1 AND deleted_at IS NULL
ORDER BY created_at ASC, id ASC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stored := make([]StoredProviderConfig, 0)
	for rows.Next() {
		item, err := scanProviderConfig(rows.Scan)
		if err != nil {
			return nil, err
		}
		stored = append(stored, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stored, nil
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

func (r *PostgresProviderConfigRepository) CommitProviderConnection(
	ctx context.Context,
	input CommitProviderConnectionInput,
) (StoredProviderConfig, error) {
	if r == nil || r.db == nil {
		return StoredProviderConfig{}, ErrDatabaseRequired
	}
	testedAt := input.ConnectionTestedAt.UTC().Format(time.RFC3339Nano)
	var stored StoredProviderConfig
	var encodedConfig []byte
	var secretRef sql.NullString
	err := r.db.QueryRowContext(ctx, `
UPDATE provider_configs
SET config = jsonb_set(
        jsonb_set(
          jsonb_set(config, '{connectionTestSha256}', to_jsonb($8::text), true),
          '{connectionTestedAt}', to_jsonb($9::text), true
        ),
        '{enabled}', to_jsonb($10::boolean), true
      ),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
  AND provider_id = $3
  AND deleted_at IS NULL
  AND encrypted_secret_ref IS NOT DISTINCT FROM NULLIF($4, '')
  AND COALESCE(config->>'type', '') = $5
  AND COALESCE(config->>'baseUrl', '') = $6
  AND COALESCE((config->>'enabled')::boolean, false) = $7
  AND $8 <> ''
RETURNING id::text, user_id::text, provider_id, label, encrypted_secret_ref, config
`,
		input.ID,
		input.UserID,
		input.ProviderID,
		input.ExpectedEncryptedSecretRef,
		string(input.ExpectedType),
		input.ExpectedBaseURL,
		input.ExpectedEnabled,
		input.ConnectionTestSHA256,
		testedAt,
		input.Enabled,
	).Scan(
		&stored.ID,
		&stored.UserID,
		&stored.ProviderID,
		&stored.Label,
		&secretRef,
		&encodedConfig,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return StoredProviderConfig{}, ErrProviderConfigChanged
		}
		return StoredProviderConfig{}, err
	}
	stored.EncryptedSecretRef = secretRef.String
	if err := json.Unmarshal(encodedConfig, &stored.Config); err != nil {
		return StoredProviderConfig{}, ErrProviderConfigUnsupported
	}
	return stored, nil
}

func (r *PostgresProviderConfigRepository) CommitSearchProviderConnection(
	ctx context.Context,
	input CommitSearchProviderConnectionInput,
) (StoredProviderConfig, error) {
	if r == nil || r.db == nil {
		return StoredProviderConfig{}, ErrDatabaseRequired
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return StoredProviderConfig{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(
		ctx,
		`LOCK TABLE provider_configs IN SHARE ROW EXCLUSIVE MODE`,
	); err != nil {
		return StoredProviderConfig{}, err
	}
	testedAt := input.ConnectionTestedAt.UTC().Format(time.RFC3339Nano)
	var stored StoredProviderConfig
	var encodedConfig []byte
	var secretRef sql.NullString
	err = tx.QueryRowContext(ctx, `
UPDATE provider_configs
SET config = jsonb_set(
        jsonb_set(
          jsonb_set(config, '{connectionTestSha256}', to_jsonb($8::text), true),
          '{connectionTestedAt}', to_jsonb($9::text), true
        ),
        '{enabled}', to_jsonb($10::boolean), true
      ),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
  AND provider_id = $3
  AND deleted_at IS NULL
  AND encrypted_secret_ref IS NOT DISTINCT FROM NULLIF($4, '')
  AND COALESCE(config->>'kind', '') = 'search'
  AND COALESCE(config->>'searchProvider', '') = $5
  AND COALESCE(config->>'baseUrl', '') = $6
  AND COALESCE((config->>'enabled')::boolean, false) = $7
  AND $8 <> ''
RETURNING id::text, user_id::text, provider_id, label, encrypted_secret_ref, config
`,
		input.ID,
		input.UserID,
		input.ProviderID,
		input.ExpectedEncryptedSecretRef,
		input.ExpectedSearchProvider,
		input.ExpectedBaseURL,
		input.ExpectedEnabled,
		input.ConnectionTestSHA256,
		testedAt,
		input.Enabled,
	).Scan(
		&stored.ID,
		&stored.UserID,
		&stored.ProviderID,
		&stored.Label,
		&secretRef,
		&encodedConfig,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return StoredProviderConfig{}, ErrProviderConfigChanged
		}
		return StoredProviderConfig{}, err
	}
	if input.Enabled {
		if _, err := tx.ExecContext(ctx, `
UPDATE provider_configs
SET config = jsonb_set(config, '{enabled}', 'false'::jsonb, true),
    updated_at = now()
WHERE user_id = $1
  AND id <> $2
  AND deleted_at IS NULL
  AND COALESCE(config->>'kind', '') = 'search'
  AND COALESCE((config->>'enabled')::boolean, false) = true
`, input.UserID, input.ID); err != nil {
			return StoredProviderConfig{}, err
		}
	}
	stored.EncryptedSecretRef = secretRef.String
	if err := json.Unmarshal(encodedConfig, &stored.Config); err != nil {
		return StoredProviderConfig{}, ErrProviderConfigUnsupported
	}
	if err := tx.Commit(); err != nil {
		return StoredProviderConfig{}, err
	}
	return stored, nil
}

func (r *PostgresProviderConfigRepository) CommitModelBuiltInSearchConnection(
	ctx context.Context,
	input CommitModelBuiltInSearchConnectionInput,
) (StoredProviderConfig, error) {
	if r == nil || r.db == nil {
		return StoredProviderConfig{}, ErrDatabaseRequired
	}
	testedAt := input.ConnectionTestedAt.UTC().Format(time.RFC3339Nano)
	var stored StoredProviderConfig
	var encodedConfig []byte
	var secretRef sql.NullString
	err := r.db.QueryRowContext(ctx, `
UPDATE provider_configs
SET config = jsonb_set(
        jsonb_set(
          config,
          '{modelBuiltInSearchTestSha256}',
          to_jsonb($9::text),
          true
        ),
        '{modelBuiltInSearchTestedAt}',
        to_jsonb($10::text),
        true
      ),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
  AND provider_id = $3
  AND deleted_at IS NULL
  AND encrypted_secret_ref IS NOT DISTINCT FROM NULLIF($4, '')
  AND COALESCE(config->>'kind', '') IN ('', 'model')
  AND COALESCE(config->>'type', '') = $5
  AND COALESCE(config->>'baseUrl', '') = $6
  AND COALESCE(config->>'modelBuiltInSearchProtocol', '') = $7
  AND COALESCE(config->>'modelBuiltInSearchModel', '') = $8
  AND $9 <> ''
RETURNING id::text, user_id::text, provider_id, label, encrypted_secret_ref, config
`,
		input.ID,
		input.UserID,
		input.ProviderID,
		input.ExpectedEncryptedSecretRef,
		string(input.ExpectedType),
		input.ExpectedBaseURL,
		input.ExpectedProtocol,
		input.ExpectedModel,
		input.ConnectionTestSHA256,
		testedAt,
	).Scan(
		&stored.ID,
		&stored.UserID,
		&stored.ProviderID,
		&stored.Label,
		&secretRef,
		&encodedConfig,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return StoredProviderConfig{}, ErrProviderConfigChanged
		}
		return StoredProviderConfig{}, err
	}
	stored.EncryptedSecretRef = secretRef.String
	if err := json.Unmarshal(encodedConfig, &stored.Config); err != nil {
		return StoredProviderConfig{}, ErrProviderConfigUnsupported
	}
	return stored, nil
}

func (r *PostgresProviderConfigRepository) DeleteProviderConfig(
	ctx context.Context,
	userID string,
	providerID string,
) error {
	if r == nil || r.db == nil {
		return ErrDatabaseRequired
	}
	result, err := r.db.ExecContext(ctx, `
UPDATE provider_configs
SET deleted_at = now(), updated_at = now()
WHERE user_id = $1 AND provider_id = $2 AND deleted_at IS NULL
`, userID, providerID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrProviderConfigNotFound
	}
	return nil
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

type rowScanner func(dest ...any) error

func scanProviderConfig(scan rowScanner) (StoredProviderConfig, error) {
	var stored StoredProviderConfig
	var encodedConfig []byte
	var secretRef sql.NullString
	if err := scan(
		&stored.ID,
		&stored.UserID,
		&stored.ProviderID,
		&stored.Label,
		&secretRef,
		&encodedConfig,
	); err != nil {
		return StoredProviderConfig{}, err
	}
	stored.EncryptedSecretRef = secretRef.String
	if len(encodedConfig) > 0 {
		if err := json.Unmarshal(encodedConfig, &stored.Config); err != nil {
			return StoredProviderConfig{}, ErrProviderConfigUnsupported
		}
	}
	return stored, nil
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
