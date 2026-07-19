package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestPostgresRAGProviderActivationPersistsVaultState(t *testing.T) {
	db := openRuntimeConfigPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DELETE FROM provider_configs`); err != nil {
		t.Fatalf("clear provider configs: %v", err)
	}
	repo := NewPostgresProviderConfigRepository(db)
	vault := testProviderSecretVault(t, "rag-v1", 44)
	writer := NewService(config.Config{}, WithProviderSecretVault(vault))
	recordID := ragProviderRecordID(RAGProviderJina)
	secretRef, err := writer.encryptRAGProviderSecretAtRest(
		authDevelopmentUserID(), recordID, "postgres-rag-fixture",
	)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.UpsertProviderConfig(ctx, UpsertProviderConfigInput{
		UserID: authDevelopmentUserID(), ProviderID: recordID, Label: "Jina AI",
		EncryptedSecretRef: secretRef,
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindRAG, RAGProvider: string(RAGProviderJina),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	committed, err := repo.CommitRAGProviderConnection(ctx, CommitRAGProviderConnectionInput{
		ID: stored.ID, UserID: stored.UserID, ProviderID: stored.ProviderID,
		ExpectedEncryptedSecretRef: stored.EncryptedSecretRef,
		ExpectedRAGProvider:        stored.Config.RAGProvider,
		ExpectedEnabled:            stored.Config.Enabled,
		ConnectionTestSHA256: ragProviderConnectionFingerprint(
			stored.ProviderID, RAGProviderJina, stored.EncryptedSecretRef,
		),
		ConnectionTestedAt: time.Now().UTC(), Enabled: true,
	})
	if err != nil || !committed.Config.Enabled || !RAGProviderConnectionTestValid(committed) {
		t.Fatalf("CommitRAGProviderConnection() = %#v, %v", committed, err)
	}

	restarted := NewService(
		config.Config{},
		WithProviderConfigRepository(NewPostgresProviderConfigRepository(db)),
		WithProviderSecretVault(testProviderSecretVault(t, "rag-v1", 44)),
	)
	status, err := restarted.RAGProviderStatus(ctx)
	if err != nil || status.Ready ||
		status.Providers.Jina.Status != "ready" ||
		status.Providers.MinerU.Status != "missing_secret" {
		t.Fatalf("restarted RAG status = %#v, %v", status, err)
	}

	_, err = repo.CommitRAGProviderConnection(ctx, CommitRAGProviderConnectionInput{
		ID: committed.ID, UserID: committed.UserID, ProviderID: committed.ProviderID,
		ExpectedEncryptedSecretRef: "stale-secret-ref",
		ExpectedRAGProvider:        string(RAGProviderJina),
		ExpectedEnabled:            true,
		ConnectionTestSHA256:       "stale-fingerprint",
		ConnectionTestedAt:         time.Now().UTC(), Enabled: true,
	})
	if !errors.Is(err, ErrProviderConfigChanged) {
		t.Fatalf("stale RAG activation error = %v", err)
	}
}
