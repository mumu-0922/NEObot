package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestPostgresRAGProviderAtomicConfigureCreatesReplacesAndFences(t *testing.T) {
	db := openRuntimeConfigPostgresIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DELETE FROM provider_configs`); err != nil {
		t.Fatalf("clear provider configs: %v", err)
	}
	repo := NewPostgresProviderConfigRepository(db)
	vault := testProviderSecretVault(t, "rag-atomic-postgres-v1", 64)
	writer := NewService(config.Config{}, WithProviderSecretVault(vault))
	recordID := ragProviderRecordID(RAGProviderMinerU)
	firstSecretRef, err := writer.encryptRAGProviderSecretAtRest(
		authDevelopmentUserID(),
		recordID,
		"first-postgres-rag-key",
	)
	if err != nil {
		t.Fatal(err)
	}
	firstTestedAt := time.Now().UTC().Add(-time.Minute)
	created, err := repo.ConfigureRAGProviderConnection(
		ctx,
		ConfigureRAGProviderConnectionInput{
			UserID: authDevelopmentUserID(), ProviderID: recordID, Label: "MinerU",
			EncryptedSecretRef: firstSecretRef,
			RAGProvider:        string(RAGProviderMinerU),
			ConnectionTestSHA256: ragProviderConnectionFingerprint(
				recordID,
				RAGProviderMinerU,
				firstSecretRef,
			),
			ConnectionTestedAt: firstTestedAt,
		},
	)
	if err != nil || !created.Config.Enabled || !RAGProviderConnectionTestValid(created) {
		t.Fatalf("create configured provider = %#v, %v", created, err)
	}

	secondSecretRef, err := writer.encryptRAGProviderSecretAtRest(
		authDevelopmentUserID(),
		recordID,
		"second-postgres-rag-key",
	)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := repo.ConfigureRAGProviderConnection(
		ctx,
		ConfigureRAGProviderConnectionInput{
			UserID: authDevelopmentUserID(), ProviderID: recordID, Label: "MinerU",
			EncryptedSecretRef: secondSecretRef,
			RAGProvider:        string(RAGProviderMinerU),
			ConnectionTestSHA256: ragProviderConnectionFingerprint(
				recordID,
				RAGProviderMinerU,
				secondSecretRef,
			),
			ConnectionTestedAt:         time.Now().UTC(),
			ExpectedExists:             true,
			ExpectedID:                 created.ID,
			ExpectedEncryptedSecretRef: created.EncryptedSecretRef,
			ExpectedRAGProvider:        created.Config.RAGProvider,
			ExpectedEnabled:            created.Config.Enabled,
		},
	)
	if err != nil || replaced.ID != created.ID ||
		replaced.EncryptedSecretRef != secondSecretRef ||
		!replaced.Config.Enabled || !RAGProviderConnectionTestValid(replaced) {
		t.Fatalf("replace configured provider = %#v, %v", replaced, err)
	}

	_, err = repo.ConfigureRAGProviderConnection(
		ctx,
		ConfigureRAGProviderConnectionInput{
			UserID: authDevelopmentUserID(), ProviderID: recordID, Label: "MinerU",
			EncryptedSecretRef: firstSecretRef,
			RAGProvider:        string(RAGProviderMinerU),
			ConnectionTestSHA256: ragProviderConnectionFingerprint(
				recordID,
				RAGProviderMinerU,
				firstSecretRef,
			),
			ConnectionTestedAt:         time.Now().UTC(),
			ExpectedExists:             true,
			ExpectedID:                 created.ID,
			ExpectedEncryptedSecretRef: created.EncryptedSecretRef,
			ExpectedRAGProvider:        created.Config.RAGProvider,
			ExpectedEnabled:            created.Config.Enabled,
		},
	)
	if !errors.Is(err, ErrProviderConfigChanged) {
		t.Fatalf("stale atomic replacement error = %v", err)
	}
	stored, ok, err := repo.GetProviderConfig(ctx, authDevelopmentUserID(), recordID)
	if err != nil || !ok || stored.EncryptedSecretRef != secondSecretRef ||
		!RAGProviderConnectionTestValid(stored) {
		t.Fatalf("post-conflict provider = %#v, ok=%t err=%v", stored, ok, err)
	}
}
