package runtimeconfig

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestResolveRAGProviderCredentialUsesOnlyExactActiveAttestedVaultRecord(t *testing.T) {
	const credential = "runtime-jina-vault-credential"
	vault := testProviderSecretVault(t, "rag-runtime-v1", 47)
	writer := NewService(config.Config{}, WithProviderSecretVault(vault))
	recordID := ragProviderRecordID(RAGProviderJina)
	secretRef, err := writer.encryptRAGProviderSecretAtRest(
		auth.DevelopmentUserID,
		recordID,
		credential,
	)
	if err != nil {
		t.Fatal(err)
	}
	stored := StoredProviderConfig{
		ID:                 "rag-runtime-config",
		UserID:             auth.DevelopmentUserID,
		ProviderID:         recordID,
		Label:              "Jina AI",
		EncryptedSecretRef: secretRef,
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindRAG, RAGProvider: string(RAGProviderJina),
			Enabled: true,
		},
	}
	stored.Config.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
		stored.ProviderID,
		RAGProviderJina,
		stored.EncryptedSecretRef,
	)
	stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	repo := &fakeProviderConfigRepository{ok: true, stored: stored}
	service := NewService(
		config.Config{RAG: config.RAGConfig{JinaAPIKey: "environment-must-not-win"}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
	)

	resolved, err := service.ResolveRAGProviderCredential(context.Background(), "jina")
	if err != nil || resolved != credential {
		t.Fatalf("ResolveRAGProviderCredential() = %q, %v", resolved, err)
	}

	repo.stored.Config.Enabled = false
	_, err = service.ResolveRAGProviderCredential(context.Background(), "jina")
	if !errors.Is(err, ragproviders.ErrProviderGatewayActivationRequired) {
		t.Fatalf("disabled provider error = %v", err)
	}
	repo.stored = stored
	repo.stored.Config.ConnectionTestSHA256 = strings.Repeat("0", 64)
	_, err = service.ResolveRAGProviderCredential(context.Background(), "jina")
	if !errors.Is(err, ragproviders.ErrProviderGatewayActivationRequired) {
		t.Fatalf("unattested provider error = %v", err)
	}
	repo.stored = stored
	repo.stored.Config.Kind = providerConfigKindSearch
	_, err = service.ResolveRAGProviderCredential(context.Background(), "jina")
	if !errors.Is(err, ragproviders.ErrProviderGatewayUnavailable) {
		t.Fatalf("cross-kind provider error = %v", err)
	}
}

func TestResolveRAGProviderCredentialRejectsMissingAndCopiedContextsWithoutEnvFallback(t *testing.T) {
	vault := testProviderSecretVault(t, "rag-runtime-v1", 48)
	service := NewService(
		config.Config{RAG: config.RAGConfig{
			MinerUAPIKey: "environment-mineru-must-not-win",
			JinaAPIKey:   "environment-jina-must-not-win",
		}},
		WithProviderConfigRepository(&fakeProviderConfigRepository{}),
		WithProviderSecretVault(vault),
	)
	_, err := service.ResolveRAGProviderCredential(context.Background(), "jina")
	if !errors.Is(err, ragproviders.ErrProviderGatewayNotFound) {
		t.Fatalf("missing provider error = %v", err)
	}

	writer := NewService(config.Config{}, WithProviderSecretVault(vault))
	minerURef, err := writer.encryptRAGProviderSecretAtRest(
		auth.DevelopmentUserID,
		ragProviderRecordID(RAGProviderMinerU),
		"copied-mineru-credential",
	)
	if err != nil {
		t.Fatal(err)
	}
	stored := StoredProviderConfig{
		ID:                 "copied-context",
		UserID:             auth.DevelopmentUserID,
		ProviderID:         ragProviderRecordID(RAGProviderJina),
		EncryptedSecretRef: minerURef,
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindRAG, RAGProvider: string(RAGProviderJina),
			Enabled: true,
		},
	}
	stored.Config.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
		stored.ProviderID,
		RAGProviderJina,
		stored.EncryptedSecretRef,
	)
	stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	service = NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{ok: true, stored: stored}),
		WithProviderSecretVault(vault),
	)
	_, err = service.ResolveRAGProviderCredential(context.Background(), "jina")
	if !errors.Is(err, ragproviders.ErrProviderGatewayUnavailable) ||
		strings.Contains(err.Error(), "copied-mineru-credential") {
		t.Fatalf("copied context error = %v", err)
	}
}
