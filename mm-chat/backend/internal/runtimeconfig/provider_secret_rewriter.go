package runtimeconfig

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"regexp"
	"strings"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/providersecrets"
)

const (
	maxProviderSecretRewriteRows       = 10_000
	maxProviderSecretRewriteConfigSize = 64 << 10
)

var (
	ErrProviderSecretRewriteUnavailable  = errors.New("provider secret rewrite is unavailable")
	ErrProviderSecretRewriteInvalid      = errors.New("provider secret rewrite input is invalid")
	ErrProviderSecretRewritePlanMismatch = errors.New("provider secret rewrite plan does not match")
	ErrProviderSecretRewriteBlocked      = errors.New("provider secret rewrite has unrecoverable legacy rows")
)

var providerSecretRewritePlanPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProviderSecretRewriteOptions struct {
	Execute            bool
	ExpectedPlanSHA256 string
}

type ProviderSecretRewriteResult struct {
	TotalRows   int
	SecretRows  int
	ChangedRows int
	LegacyRows  int
	EnvRows     int
	RotatedRows int
	CurrentRows int
	EmptyRows   int
	BlockedRows int
	PlanSHA256  string
	Executed    bool
}

type PostgresProviderSecretRewriter struct {
	db      *sql.DB
	cfg     config.Config
	vault   *providersecrets.Vault
	service *Service
}

func NewPostgresProviderSecretRewriter(
	db *sql.DB,
	cfg config.Config,
	vault *providersecrets.Vault,
) *PostgresProviderSecretRewriter {
	// Bulk legacy rewrites must never generate an ephemeral ingress key. Either
	// the configured stable BYOK key decrypts every legacy row or the entire
	// transaction fails before the first update.
	cfg.BYOK.AllowEphemeralKey = false
	return &PostgresProviderSecretRewriter{
		db: db, cfg: cfg, vault: vault,
		service: NewService(cfg, WithProviderSecretVault(vault)),
	}
}

func (r *PostgresProviderSecretRewriter) Rewrite(
	ctx context.Context,
	options ProviderSecretRewriteOptions,
) (ProviderSecretRewriteResult, error) {
	if r == nil || r.db == nil || r.vault == nil || r.service == nil {
		return ProviderSecretRewriteResult{}, ErrProviderSecretRewriteUnavailable
	}
	expectedPlan := strings.TrimSpace(options.ExpectedPlanSHA256)
	if options.Execute {
		if !providerSecretRewritePlanPattern.MatchString(expectedPlan) {
			return ProviderSecretRewriteResult{}, ErrProviderSecretRewritePlanMismatch
		}
	} else if expectedPlan != "" {
		return ProviderSecretRewriteResult{}, ErrProviderSecretRewriteInvalid
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ProviderSecretRewriteResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `LOCK TABLE provider_configs IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return ProviderSecretRewriteResult{}, err
	}

	providerRows, err := loadProviderSecretRewriteRows(ctx, tx)
	if err != nil {
		return ProviderSecretRewriteResult{}, err
	}
	plan, err := r.buildProviderSecretRewritePlan(providerRows)
	if err != nil {
		return ProviderSecretRewriteResult{}, err
	}
	if !options.Execute {
		return plan.result, nil
	}
	if plan.result.PlanSHA256 != expectedPlan {
		return ProviderSecretRewriteResult{}, ErrProviderSecretRewritePlanMismatch
	}
	if plan.result.BlockedRows != 0 {
		return ProviderSecretRewriteResult{}, ErrProviderSecretRewriteBlocked
	}

	for _, item := range plan.rows {
		if item.action != providerSecretRewriteLegacy &&
			item.action != providerSecretRewriteRotate {
			continue
		}
		secretRef, err := r.rewriteProviderSecret(item)
		if err != nil {
			return ProviderSecretRewriteResult{}, err
		}
		connectionTestSHA256, preserveAttestation, err :=
			r.rewrittenProviderConnectionAttestation(item, secretRef)
		if err != nil {
			return ProviderSecretRewriteResult{}, err
		}
		result, err := tx.ExecContext(ctx, `
UPDATE provider_configs
SET encrypted_secret_ref = $2,
    config = CASE
      WHEN $4 THEN jsonb_set(
        config,
        '{connectionTestSha256}',
        to_jsonb($5::text),
        true
      )
      ELSE config
    END,
    updated_at = now()
WHERE id = $1
  AND encrypted_secret_ref = $3
  AND config = $6::jsonb
`,
			item.id,
			secretRef,
			item.encryptedSecretRef,
			preserveAttestation,
			connectionTestSHA256,
			item.configJSON,
		)
		if err != nil {
			return ProviderSecretRewriteResult{}, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return ProviderSecretRewriteResult{}, err
		}
		if changed != 1 {
			return ProviderSecretRewriteResult{}, ErrProviderSecretRewritePlanMismatch
		}
	}
	if err := tx.Commit(); err != nil {
		return ProviderSecretRewriteResult{}, err
	}
	plan.result.Executed = true
	return plan.result, nil
}

type providerSecretRewriteAction string

const (
	providerSecretRewriteEmpty   providerSecretRewriteAction = "empty"
	providerSecretRewriteCurrent providerSecretRewriteAction = "current"
	providerSecretRewriteRotate  providerSecretRewriteAction = "rotate"
	providerSecretRewriteLegacy  providerSecretRewriteAction = "legacy"
	providerSecretRewriteBlocked providerSecretRewriteAction = "blocked-legacy"
)

type providerSecretRewriteRow struct {
	id                 string
	userID             string
	providerID         string
	encryptedSecretRef string
	configJSON         string
	deleted            bool
	action             providerSecretRewriteAction
}

type providerSecretRewritePlan struct {
	rows   []providerSecretRewriteRow
	result ProviderSecretRewriteResult
}

func loadProviderSecretRewriteRows(
	ctx context.Context,
	tx *sql.Tx,
) ([]providerSecretRewriteRow, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id::text,
       user_id::text,
       provider_id,
       COALESCE(encrypted_secret_ref, ''),
       config::text,
       deleted_at IS NOT NULL
FROM provider_configs
ORDER BY id
FOR UPDATE
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]providerSecretRewriteRow, 0)
	for rows.Next() {
		if len(result) >= maxProviderSecretRewriteRows {
			return nil, ErrProviderSecretRewriteInvalid
		}
		var row providerSecretRewriteRow
		if err := rows.Scan(
			&row.id,
			&row.userID,
			&row.providerID,
			&row.encryptedSecretRef,
			&row.configJSON,
			&row.deleted,
		); err != nil {
			return nil, err
		}
		if len(row.configJSON) > maxProviderSecretRewriteConfigSize ||
			len(row.encryptedSecretRef) > maxStoredProviderSecretRefBytes {
			return nil, ErrProviderSecretRewriteInvalid
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *PostgresProviderSecretRewriter) buildProviderSecretRewritePlan(
	rows []providerSecretRewriteRow,
) (providerSecretRewritePlan, error) {
	if len(rows) > maxProviderSecretRewriteRows {
		return providerSecretRewritePlan{}, ErrProviderSecretRewriteInvalid
	}
	result := ProviderSecretRewriteResult{TotalRows: len(rows)}
	active := make(map[string]struct{}, len(rows))
	for index := range rows {
		row := &rows[index]
		if row.id != strings.TrimSpace(row.id) || row.id == "" ||
			row.userID != strings.TrimSpace(row.userID) || row.userID == "" ||
			row.providerID != strings.TrimSpace(row.providerID) || row.providerID == "" ||
			len(row.providerID) > 128 ||
			len(row.configJSON) > maxProviderSecretRewriteConfigSize ||
			len(row.encryptedSecretRef) > maxStoredProviderSecretRefBytes {
			return providerSecretRewritePlan{}, ErrProviderSecretRewriteInvalid
		}
		if !row.deleted {
			key := row.userID + "\x00" + row.providerID
			if _, duplicate := active[key]; duplicate {
				return providerSecretRewritePlan{}, ErrProviderSecretRewriteInvalid
			}
			active[key] = struct{}{}
		}

		action, err := r.inspectProviderSecret(*row)
		if err != nil {
			return providerSecretRewritePlan{}, err
		}
		row.action = action
		switch action {
		case providerSecretRewriteEmpty:
			result.EmptyRows++
		case providerSecretRewriteCurrent:
			result.SecretRows++
			result.CurrentRows++
		case providerSecretRewriteRotate:
			result.SecretRows++
			result.RotatedRows++
			result.ChangedRows++
		case providerSecretRewriteLegacy:
			result.SecretRows++
			result.LegacyRows++
			result.ChangedRows++
		case providerSecretRewriteBlocked:
			result.SecretRows++
			result.BlockedRows++
		default:
			return providerSecretRewritePlan{}, ErrProviderSecretRewriteInvalid
		}
	}
	activeKeyBinding, err := r.vault.ActiveKeyBinding("provider:model:rewrite:v1")
	if err != nil {
		return providerSecretRewritePlan{}, ErrProviderSecretRewriteInvalid
	}
	result.PlanSHA256 = providerSecretRewritePlanSHA256(
		r.vault.ActiveKID(), activeKeyBinding, rows,
	)
	return providerSecretRewritePlan{rows: rows, result: result}, nil
}

func (r *PostgresProviderSecretRewriter) inspectProviderSecret(
	row providerSecretRewriteRow,
) (providerSecretRewriteAction, error) {
	encoded := strings.TrimSpace(row.encryptedSecretRef)
	if encoded == "" {
		return providerSecretRewriteEmpty, nil
	}
	switch storedSecretAlgorithm(encoded) {
	case providersecrets.Algorithm:
		envelope, err := providersecrets.ParseEnvelope(encoded)
		if err != nil {
			return "", ErrProviderSecretRewriteInvalid
		}
		secretContext, err := r.providerSecretContext(row)
		if err != nil {
			return "", err
		}
		plaintext, err := r.vault.Decrypt(envelope, secretContext)
		validPlaintext := strings.TrimSpace(string(plaintext)) != ""
		clear(plaintext)
		if err != nil || !validPlaintext {
			return "", ErrProviderSecretRewriteInvalid
		}
		if r.vault.NeedsRotation(envelope) {
			return providerSecretRewriteRotate, nil
		}
		return providerSecretRewriteCurrent, nil
	case byokAlgorithm:
		if !r.isLegacyModelProvider(row) {
			return providerSecretRewriteBlocked, nil
		}
		envelope, err := parseStoredLegacySecretRef(encoded)
		if err != nil {
			return "", ErrProviderSecretRewriteInvalid
		}
		providerType, err := r.providerType(row)
		if err != nil {
			return "", err
		}
		plaintext, err := r.service.DecryptSecretEnvelope(
			envelope,
			"provider:"+string(providerType),
		)
		validPlaintext := strings.TrimSpace(plaintext) != ""
		plaintext = ""
		if err != nil || !validPlaintext {
			return providerSecretRewriteBlocked, nil
		}
		return providerSecretRewriteLegacy, nil
	default:
		return "", ErrProviderSecretRewriteInvalid
	}
}

func (r *PostgresProviderSecretRewriter) rewriteProviderSecret(
	row providerSecretRewriteRow,
) (string, error) {
	switch row.action {
	case providerSecretRewriteRotate:
		envelope, err := providersecrets.ParseEnvelope(row.encryptedSecretRef)
		if err != nil {
			return "", ErrProviderSecretRewriteInvalid
		}
		secretContext, err := r.providerSecretContext(row)
		if err != nil {
			return "", err
		}
		rotated, changed, err := r.vault.Rotate(envelope, secretContext)
		if err != nil || !changed {
			return "", ErrProviderSecretRewriteInvalid
		}
		return marshalProviderSecretEnvelope(rotated)
	case providerSecretRewriteLegacy:
		if !r.isLegacyModelProvider(row) {
			return "", ErrProviderSecretRewriteInvalid
		}
		envelope, err := parseStoredLegacySecretRef(row.encryptedSecretRef)
		if err != nil {
			return "", ErrProviderSecretRewriteInvalid
		}
		providerType, err := r.providerType(row)
		if err != nil {
			return "", err
		}
		plaintext, err := r.service.DecryptSecretEnvelope(
			envelope,
			"provider:"+string(providerType),
		)
		if err != nil || strings.TrimSpace(plaintext) == "" {
			return "", ErrProviderSecretRewriteInvalid
		}
		secretBytes := []byte(strings.TrimSpace(plaintext))
		plaintext = ""
		secretContext, err := r.providerSecretContext(row)
		if err != nil {
			clear(secretBytes)
			return "", err
		}
		vaultEnvelope, err := r.vault.Encrypt(secretBytes, secretContext)
		clear(secretBytes)
		if err != nil {
			return "", ErrProviderSecretRewriteInvalid
		}
		return marshalProviderSecretEnvelope(vaultEnvelope)
	default:
		return "", ErrProviderSecretRewriteInvalid
	}
}

func (r *PostgresProviderSecretRewriter) providerType(
	row providerSecretRewriteRow,
) (ProviderType, error) {
	var payload StoredProviderConfigPayload
	if err := json.Unmarshal([]byte(row.configJSON), &payload); err != nil {
		return "", ErrProviderSecretRewriteInvalid
	}
	if payload.Type != "" {
		return normalizeProviderType(string(payload.Type)), nil
	}
	return ProviderTypeOpenAICompatible, nil
}

func (r *PostgresProviderSecretRewriter) providerSecretContext(
	row providerSecretRewriteRow,
) (string, error) {
	var payload StoredProviderConfigPayload
	if err := json.Unmarshal([]byte(row.configJSON), &payload); err != nil {
		return "", ErrProviderSecretRewriteInvalid
	}
	secretContext, ok := storedProviderSecretContext(row.userID, row.providerID, payload)
	if !ok {
		return "", ErrProviderSecretRewriteInvalid
	}
	return secretContext, nil
}

func (r *PostgresProviderSecretRewriter) isLegacyModelProvider(
	row providerSecretRewriteRow,
) bool {
	var payload StoredProviderConfigPayload
	if json.Unmarshal([]byte(row.configJSON), &payload) != nil {
		return false
	}
	return IsModelProviderConfig(StoredProviderConfig{
		ProviderID: row.providerID,
		Config:     payload,
	})
}

func marshalProviderSecretEnvelope(envelope providersecrets.Envelope) (string, error) {
	encoded, err := json.Marshal(envelope)
	if err != nil || len(encoded) > maxStoredProviderSecretRefBytes {
		return "", ErrProviderSecretRewriteInvalid
	}
	return string(encoded), nil
}

func providerSecretRewritePlanSHA256(
	activeKID string,
	activeKeyBinding string,
	rows []providerSecretRewriteRow,
) string {
	digest := sha256.New()
	writeProviderSecretPlanField(digest, "provider-secret-rewrite-v1")
	writeProviderSecretPlanField(digest, activeKID)
	writeProviderSecretPlanField(digest, activeKeyBinding)
	for _, row := range rows {
		writeProviderSecretPlanField(digest, row.id)
		writeProviderSecretPlanField(digest, row.userID)
		writeProviderSecretPlanField(digest, row.providerID)
		writeProviderSecretPlanField(digest, row.encryptedSecretRef)
		writeProviderSecretPlanField(digest, row.configJSON)
		if row.deleted {
			writeProviderSecretPlanField(digest, "deleted")
		} else {
			writeProviderSecretPlanField(digest, "active")
		}
		writeProviderSecretPlanField(digest, string(row.action))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func writeProviderSecretPlanField(digest hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = digest.Write(size[:])
	_, _ = digest.Write([]byte(value))
}
