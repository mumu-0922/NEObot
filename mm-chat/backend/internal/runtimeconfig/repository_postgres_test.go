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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/migration"
	"neo-chat/mm-chat/backend/internal/providersecrets"
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
