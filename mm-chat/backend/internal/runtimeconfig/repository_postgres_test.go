package runtimeconfig

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/migration"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/websearch"
	migrationfiles "neo-chat/mm-chat/backend/migrations"
)

func TestPostgresProviderVaultCiphertextSurvivesReload(t *testing.T) {
	db := openRuntimeConfigPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DELETE FROM provider_configs`); err != nil {
		t.Fatalf("clear provider configs: %v", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	keyringPath := filepath.Join(t.TempDir(), "provider-keyring.json")
	encodedKey := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{17}, 32))
	keyringJSON, err := json.Marshal(providersecrets.KeyringConfig{
		V:         providersecrets.KeyringVersion,
		ActiveKID: "integration-v1",
		Keys: []providersecrets.KeyConfig{{
			KID: "integration-v1", Key: encodedKey,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyringPath, keyringJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	firstVault, err := providersecrets.LoadVaultFile(keyringPath)
	if err != nil {
		t.Fatalf("load first vault: %v", err)
	}
	repo := NewPostgresProviderConfigRepository(db)
	first := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(firstVault),
	)
	const plaintext = "postgres-vault-fixture-secret"
	_, err = first.UpsertAdminProviderConfig(
		ctx,
		"INTEGRATION",
		UpdateAdminProviderConfigRequest{
			Name: "Integration", Type: "OpenAI Compatible", Enabled: true,
			APIKeySecret: encryptedSecretEnvelope(
				t, privateKey, plaintext, "provider:OpenAI Compatible",
			),
		},
	)
	if err != nil {
		t.Fatalf("UpsertAdminProviderConfig() error = %v", err)
	}
	stored, ok, err := repo.GetProviderConfig(ctx, authDevelopmentUserID(), "INTEGRATION")
	if err != nil || !ok {
		t.Fatalf("GetProviderConfig() = %#v/%v/%v", stored, ok, err)
	}
	_, err = repo.CommitProviderConnection(ctx, CommitProviderConnectionInput{
		ID:                         stored.ID,
		UserID:                     stored.UserID,
		ProviderID:                 stored.ProviderID,
		ExpectedEncryptedSecretRef: stored.EncryptedSecretRef,
		ExpectedType:               stored.Config.Type,
		ExpectedBaseURL:            stored.Config.BaseURL,
		ExpectedEnabled:            stored.Config.Enabled,
		ConnectionTestSHA256:       ProviderConnectionTestFingerprint(stored),
		ConnectionTestedAt:         time.Now().UTC(),
		Enabled:                    true,
	})
	if err != nil {
		t.Fatalf("CommitProviderConnection() error = %v", err)
	}

	var secretRef string
	var storedConfig string
	if err := db.QueryRowContext(ctx, `
SELECT encrypted_secret_ref, config::text
FROM provider_configs
WHERE provider_id = 'INTEGRATION' AND deleted_at IS NULL
`).Scan(&secretRef, &storedConfig); err != nil {
		t.Fatalf("query stored provider: %v", err)
	}
	if storedSecretAlgorithm(secretRef) != providersecrets.Algorithm ||
		strings.Contains(secretRef, plaintext) || strings.Contains(storedConfig, plaintext) ||
		strings.Contains(secretRef, byokAlgorithm) {
		t.Fatalf("database provider secret is not vault-only")
	}

	secondVault, err := providersecrets.LoadVaultFile(keyringPath)
	if err != nil {
		t.Fatalf("load restarted vault: %v", err)
	}
	restarted := NewService(
		config.Config{},
		WithProviderConfigRepository(NewPostgresProviderConfigRepository(db)),
		WithProviderSecretVault(secondVault),
	)
	resolved, err := restarted.ResolveStoredProvider(ctx, "INTEGRATION")
	if err != nil {
		t.Fatalf("ResolveStoredProvider() error = %v", err)
	}
	if resolved.APIKey != plaintext {
		t.Fatalf("reloaded provider secret does not match")
	}
}

func TestPostgresSearchProviderActivationKeepsExactlyOneActive(t *testing.T) {
	db := openRuntimeConfigPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DELETE FROM provider_configs`); err != nil {
		t.Fatalf("clear provider configs: %v", err)
	}
	repo := NewPostgresProviderConfigRepository(db)
	vault := testProviderSecretVault(t, "search-v1", 34)
	secretWriter := NewService(config.Config{}, WithProviderSecretVault(vault))
	create := func(providerID websearch.ProviderID) StoredProviderConfig {
		recordID := searchProviderRecordID(providerID)
		secretRef, err := secretWriter.encryptSearchProviderSecretAtRest(
			authDevelopmentUserID(), recordID, string(providerID)+"-fixture-secret",
		)
		if err != nil {
			t.Fatalf("encrypt %s Search secret: %v", providerID, err)
		}
		stored, err := repo.UpsertProviderConfig(ctx, UpsertProviderConfigInput{
			UserID: authDevelopmentUserID(), ProviderID: recordID,
			Label:              searchProviderDefaultName(providerID),
			EncryptedSecretRef: secretRef,
			Config: StoredProviderConfigPayload{
				Kind: providerConfigKindSearch, SearchProvider: string(providerID),
			},
		})
		if err != nil {
			t.Fatalf("upsert %s: %v", providerID, err)
		}
		return stored
	}
	tavily := create(websearch.ProviderTavily)
	exa := create(websearch.ProviderExa)
	activate := func(stored StoredProviderConfig) StoredProviderConfig {
		providerID, err := normalizeSearchProviderID(stored.Config.SearchProvider)
		if err != nil {
			t.Fatal(err)
		}
		committed, err := repo.CommitSearchProviderConnection(ctx, CommitSearchProviderConnectionInput{
			ID: stored.ID, UserID: stored.UserID, ProviderID: stored.ProviderID,
			ExpectedEncryptedSecretRef: stored.EncryptedSecretRef,
			ExpectedSearchProvider:     stored.Config.SearchProvider,
			ExpectedBaseURL:            stored.Config.BaseURL,
			ExpectedEnabled:            stored.Config.Enabled,
			ConnectionTestSHA256: searchProviderConnectionFingerprint(
				stored.ProviderID, providerID, stored.Config.BaseURL, stored.EncryptedSecretRef,
			),
			ConnectionTestedAt: time.Now().UTC(), Enabled: true,
		})
		if err != nil {
			t.Fatalf("activate %s: %v", stored.ProviderID, err)
		}
		return committed
	}
	tavily = activate(tavily)
	if !tavily.Config.Enabled {
		t.Fatal("Tavily was not activated")
	}
	exa = activate(exa)
	if !exa.Config.Enabled {
		t.Fatal("Exa was not activated")
	}

	listed, err := repo.ListProviderConfigs(ctx, authDevelopmentUserID())
	if err != nil {
		t.Fatal(err)
	}
	active := make([]string, 0, 1)
	for _, stored := range listed {
		if isSearchProviderConfig(stored) && stored.Config.Enabled {
			active = append(active, stored.ProviderID)
		}
	}
	if len(active) != 1 || active[0] != searchProviderRecordID(websearch.ProviderExa) {
		t.Fatalf("active Search providers = %#v", active)
	}
	restarted := NewService(
		config.Config{},
		WithProviderConfigRepository(NewPostgresProviderConfigRepository(db)),
		WithProviderSecretVault(testProviderSecretVault(t, "search-v1", 34)),
	)
	execution, err := restarted.ResolveActive(ctx)
	if err != nil || execution.Mode != websearch.ExecutionExternal ||
		execution.External == nil || execution.External.ID() != websearch.ProviderExa {
		t.Fatalf("restarted ResolveActive() = %#v, %v", execution, err)
	}

	_, err = repo.CommitSearchProviderConnection(ctx, CommitSearchProviderConnectionInput{
		ID: tavily.ID, UserID: tavily.UserID, ProviderID: tavily.ProviderID,
		ExpectedEncryptedSecretRef: tavily.EncryptedSecretRef,
		ExpectedSearchProvider:     tavily.Config.SearchProvider,
		ExpectedBaseURL:            "https://changed.example",
		ExpectedEnabled:            false,
		ConnectionTestSHA256:       "stale-fingerprint",
		ConnectionTestedAt:         time.Now().UTC(), Enabled: true,
	})
	if !errors.Is(err, ErrProviderConfigChanged) {
		t.Fatalf("stale activation error = %v", err)
	}
}

func authDevelopmentUserID() string {
	return "00000000-0000-0000-0000-000000000001"
}

func TestPostgresProviderSecretRewriteBackfillsAndRotatesEveryCiphertextRow(t *testing.T) {
	db := openRuntimeConfigPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DELETE FROM provider_configs`); err != nil {
		t.Fatalf("clear provider configs: %v", err)
	}

	privateKey, cfg := providerSecretRewriteBYOKConfig(t)
	oldVault := providerSecretRewriteVault(t, "old", map[string]byte{"old": 31})
	rotatingVault := providerSecretRewriteVault(t, "new", map[string]byte{
		"new": 32,
		"old": 31,
	})
	newOnlyVault := providerSecretRewriteVault(t, "new", map[string]byte{"new": 32})
	const userID = "00000000-0000-0000-0000-000000000001"
	oldAttested := providerSecretVaultRewriteRow(
		t, oldVault, "10000000-0000-0000-0000-000000000001",
		userID, "OLD", "old-secret", false,
	)
	oldAttested.configJSON = providerSecretRewriteConfigJSON(t, oldAttested, func(
		payload *StoredProviderConfigPayload,
	) {
		payload.Type = ProviderTypeOpenAICompatible
		payload.BaseURL = "https://model.example.test/v1"
		payload.Enabled = true
		payload.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
		payload.ConnectionTestSHA256 = providerConnectionFingerprint(
			oldAttested.providerID,
			payload.Type,
			payload.BaseURL,
			oldAttested.encryptedSecretRef,
		)
	})
	providerRows := []providerSecretRewriteRow{
		oldAttested,
		providerSecretLegacyRewriteRow(
			t, privateKey, "10000000-0000-0000-0000-000000000002",
			userID, "LEGACY", "legacy-secret",
		),
		providerSecretVaultRewriteRow(
			t, rotatingVault, "10000000-0000-0000-0000-000000000003",
			userID, "CURRENT", "current-secret", false,
		),
		{
			id: "10000000-0000-0000-0000-000000000004", userID: userID,
			providerID: "EMPTY", configJSON: `{}`,
		},
		providerSecretVaultRewriteRow(
			t, oldVault, "10000000-0000-0000-0000-000000000005",
			userID, "DELETED", "deleted-secret", true,
		),
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO users (id, display_name)
VALUES ($1, 'Rewrite Test')
ON CONFLICT (id) DO NOTHING
`, userID); err != nil {
		t.Fatalf("insert rewrite user: %v", err)
	}
	for _, row := range providerRows {
		if _, err := db.ExecContext(ctx, `
INSERT INTO provider_configs (
  id, user_id, provider_id, label, encrypted_secret_ref, config, deleted_at
) VALUES ($1, $2, $3, $3, NULLIF($4, ''), $5::jsonb, CASE WHEN $6 THEN now() ELSE NULL END)
`, row.id, row.userID, row.providerID, row.encryptedSecretRef, row.configJSON, row.deleted); err != nil {
			t.Fatalf("insert provider %s: %v", row.providerID, err)
		}
	}

	rewriter := NewPostgresProviderSecretRewriter(db, cfg, rotatingVault)
	dryRun, err := rewriter.Rewrite(ctx, ProviderSecretRewriteOptions{})
	if err != nil {
		t.Fatalf("dry-run Rewrite() error = %v", err)
	}
	if dryRun.TotalRows != 5 || dryRun.SecretRows != 4 || dryRun.ChangedRows != 3 ||
		dryRun.LegacyRows != 1 || dryRun.EnvRows != 0 || dryRun.RotatedRows != 2 ||
		dryRun.CurrentRows != 1 || dryRun.EmptyRows != 1 || dryRun.BlockedRows != 0 ||
		dryRun.Executed {
		t.Fatalf("dry-run = %#v", dryRun)
	}

	beforeMismatch := providerSecretRefsByID(t, ctx, db)
	_, err = rewriter.Rewrite(ctx, ProviderSecretRewriteOptions{
		Execute: true, ExpectedPlanSHA256: strings.Repeat("0", 64),
	})
	if !errors.Is(err, ErrProviderSecretRewritePlanMismatch) {
		t.Fatalf("mismatched Rewrite() error = %v", err)
	}
	if afterMismatch := providerSecretRefsByID(t, ctx, db); !reflect.DeepEqual(afterMismatch, beforeMismatch) {
		t.Fatal("plan mismatch changed provider ciphertext")
	}

	executed, err := rewriter.Rewrite(ctx, ProviderSecretRewriteOptions{
		Execute: true, ExpectedPlanSHA256: dryRun.PlanSHA256,
	})
	if err != nil || !executed.Executed || executed.ChangedRows != 3 {
		t.Fatalf("executed Rewrite() = %#v, %v", executed, err)
	}
	wantPlaintext := map[string]string{
		"OLD": "old-secret", "LEGACY": "legacy-secret",
		"CURRENT": "current-secret", "DELETED": "deleted-secret",
	}
	rows, err := db.QueryContext(ctx, `
SELECT user_id::text, provider_id, encrypted_secret_ref, config
FROM provider_configs
WHERE encrypted_secret_ref IS NOT NULL
ORDER BY provider_id
`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var storedUserID, providerID, encoded string
		var encodedConfig []byte
		if err := rows.Scan(
			&storedUserID,
			&providerID,
			&encoded,
			&encodedConfig,
		); err != nil {
			t.Fatal(err)
		}
		envelope, err := providersecrets.ParseEnvelope(encoded)
		if err != nil || envelope.KID != "new" || strings.Contains(encoded, byokAlgorithm) {
			t.Fatalf("rewritten %s envelope = %#v, %v", providerID, envelope, err)
		}
		plaintext, err := newOnlyVault.Decrypt(
			envelope,
			modelProviderSecretContext(storedUserID, providerID),
		)
		if err != nil || string(plaintext) != wantPlaintext[providerID] {
			t.Fatalf("rewritten %s plaintext = %q, %v", providerID, plaintext, err)
		}
		clear(plaintext)
		if providerID == "OLD" {
			var payload StoredProviderConfigPayload
			if err := json.Unmarshal(encodedConfig, &payload); err != nil {
				t.Fatal(err)
			}
			stored := StoredProviderConfig{
				UserID: storedUserID, ProviderID: providerID,
				EncryptedSecretRef: encoded, Config: payload,
			}
			if !ProviderConnectionTestValid(stored) {
				t.Fatal("rotated provider lost its valid connection attestation")
			}
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 4 {
		t.Fatalf("rewritten ciphertext rows = %d, want 4", seen)
	}

	restarted := NewPostgresProviderSecretRewriter(db, config.Config{}, newOnlyVault)
	audit, err := restarted.Rewrite(ctx, ProviderSecretRewriteOptions{})
	if err != nil || audit.ChangedRows != 0 ||
		audit.CurrentRows != 4 || audit.EmptyRows != 1 {
		t.Fatalf("new-only restart audit = %#v, %v", audit, err)
	}

	stalePrivateKey, _ := providerSecretRewriteBYOKConfig(t)
	blocked := providerSecretLegacyRewriteRow(
		t,
		stalePrivateKey,
		"10000000-0000-0000-0000-000000000006",
		userID,
		"BLOCKED",
		"blocked-secret",
	)
	if _, err := db.ExecContext(ctx, `
INSERT INTO provider_configs (
  id, user_id, provider_id, label, encrypted_secret_ref, config
) VALUES ($1, $2, $3, $3, $4, $5::jsonb)
`, blocked.id, blocked.userID, blocked.providerID, blocked.encryptedSecretRef, blocked.configJSON); err != nil {
		t.Fatalf("insert blocked provider: %v", err)
	}
	blockedRewriter := NewPostgresProviderSecretRewriter(db, cfg, newOnlyVault)
	blockedPlan, err := blockedRewriter.Rewrite(ctx, ProviderSecretRewriteOptions{})
	if err != nil || blockedPlan.BlockedRows != 1 || blockedPlan.ChangedRows != 0 {
		t.Fatalf("blocked dry-run = %#v, %v", blockedPlan, err)
	}
	beforeBlocked := providerSecretRefsByID(t, ctx, db)
	_, err = blockedRewriter.Rewrite(ctx, ProviderSecretRewriteOptions{
		Execute: true, ExpectedPlanSHA256: blockedPlan.PlanSHA256,
	})
	if !errors.Is(err, ErrProviderSecretRewriteBlocked) {
		t.Fatalf("blocked execute error = %v", err)
	}
	if afterBlocked := providerSecretRefsByID(t, ctx, db); !reflect.DeepEqual(afterBlocked, beforeBlocked) {
		t.Fatal("blocked execute changed provider ciphertext")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM provider_configs WHERE id = $1`, blocked.id); err != nil {
		t.Fatalf("remove blocked fixture: %v", err)
	}
}

func providerSecretRefsByID(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
) map[string]string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
SELECT id::text, COALESCE(encrypted_secret_ref, '')
FROM provider_configs
ORDER BY id
`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var id, secretRef string
		if err := rows.Scan(&id, &secretRef); err != nil {
			t.Fatal(err)
		}
		result[id] = secretRef
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func openRuntimeConfigPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("MM_CHAT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set MM_CHAT_TEST_DATABASE_URL to run Postgres integration tests")
	}
	pgxConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse MM_CHAT_TEST_DATABASE_URL: %v", err)
	}
	if !isRuntimeConfigIntegrationDatabase(pgxConfig.Database) {
		t.Fatal("MM_CHAT_TEST_DATABASE_URL must name an isolated mm_chat_*_test database")
	}
	pgxConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	db := stdlib.OpenDB(*pgxConfig)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping integration database: %v", err)
	}
	if os.Getenv("MM_CHAT_TEST_DATABASE_PREMIGRATED") != "true" {
		if _, err := migration.NewRunner(db, migrationfiles.FS).Up(ctx); err != nil {
			t.Fatalf("apply migrations: %v", err)
		}
	}
	return db
}

func isRuntimeConfigIntegrationDatabase(name string) bool {
	return strings.HasPrefix(name, "mm_chat_") &&
		strings.HasSuffix(name, "_test") && len(name) > len("mm_chat__test")
}

func TestRuntimeConfigIntegrationDatabaseNameGuard(t *testing.T) {
	for name, want := range map[string]bool{
		"mm_chat_g119f21_test": true,
		"mm_chat_a_test":       true,
		"neo_chat":             false,
		"mm_chat_test":         false,
		"mm_chat_prod":         false,
	} {
		if got := isRuntimeConfigIntegrationDatabase(name); got != want {
			t.Errorf("isRuntimeConfigIntegrationDatabase(%q) = %t, want %t", name, got, want)
		}
	}
}
