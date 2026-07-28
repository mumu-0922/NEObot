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
	const credential = "runtime-siliconflow-vault-credential"
	vault := testProviderSecretVault(t, "rag-runtime-v1", 47)
	writer := NewService(config.Config{}, WithProviderSecretVault(vault))
	recordID := ragProviderRecordID(RAGProviderSiliconFlow)
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
		Label:              "SiliconFlow AI",
		EncryptedSecretRef: secretRef,
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindRAG, RAGProvider: string(RAGProviderSiliconFlow),
			Enabled: true,
		},
	}
	stored.Config.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
		stored.ProviderID,
		RAGProviderSiliconFlow,
		stored.EncryptedSecretRef,
	)
	stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	repo := &fakeProviderConfigRepository{ok: true, stored: stored}
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
	)

	resolved, err := service.ResolveRAGProviderCredential(context.Background(), "siliconflow")
	if err != nil || resolved != credential {
		t.Fatalf("ResolveRAGProviderCredential() = %q, %v", resolved, err)
	}

	repo.stored.Config.Enabled = false
	_, err = service.ResolveRAGProviderCredential(context.Background(), "siliconflow")
	if !errors.Is(err, ragproviders.ErrProviderGatewayActivationRequired) {
		t.Fatalf("disabled provider error = %v", err)
	}
	repo.stored = stored
	repo.stored.Config.ConnectionTestSHA256 = strings.Repeat("0", 64)
	_, err = service.ResolveRAGProviderCredential(context.Background(), "siliconflow")
	if !errors.Is(err, ragproviders.ErrProviderGatewayActivationRequired) {
		t.Fatalf("unattested provider error = %v", err)
	}
	repo.stored = stored
	repo.stored.Config.Kind = providerConfigKindSearch
	_, err = service.ResolveRAGProviderCredential(context.Background(), "siliconflow")
	if !errors.Is(err, ragproviders.ErrProviderGatewayUnavailable) {
		t.Fatalf("cross-kind provider error = %v", err)
	}
}

func TestResolveRAGProviderCredentialRejectsMissingAndCopiedContextsWithoutEnvFallback(t *testing.T) {
	vault := testProviderSecretVault(t, "rag-runtime-v1", 48)
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{}),
		WithProviderSecretVault(vault),
	)
	_, err := service.ResolveRAGProviderCredential(context.Background(), "siliconflow")
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
		ProviderID:         ragProviderRecordID(RAGProviderSiliconFlow),
		EncryptedSecretRef: minerURef,
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindRAG, RAGProvider: string(RAGProviderSiliconFlow),
			Enabled: true,
		},
	}
	stored.Config.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
		stored.ProviderID,
		RAGProviderSiliconFlow,
		stored.EncryptedSecretRef,
	)
	stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	service = NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{ok: true, stored: stored}),
		WithProviderSecretVault(vault),
	)
	_, err = service.ResolveRAGProviderCredential(context.Background(), "siliconflow")
	if !errors.Is(err, ragproviders.ErrProviderGatewayUnavailable) ||
		strings.Contains(err.Error(), "copied-mineru-credential") {
		t.Fatalf("copied context error = %v", err)
	}
}

func TestResolveRAGProviderCredentialPermanentlyRejectsRetiredJina(t *testing.T) {
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: StoredProviderConfig{
			ProviderID: ragProviderRecordPrefix + "JINA",
			Config: StoredProviderConfigPayload{
				Kind: providerConfigKindRAG, RAGProvider: retiredRAGProviderJina,
				Enabled: true,
			},
		},
	}
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
	)

	_, err := service.ResolveRAGProviderCredential(context.Background(), "jina")
	if !errors.Is(err, ragproviders.ErrProviderGatewayUnavailable) {
		t.Fatalf("retired Jina credential error = %v", err)
	}
	if repo.getCalls != 0 {
		t.Fatalf("retired Jina reached repository %d time(s)", repo.getCalls)
	}
}

func TestResolveHydratedStoredRAGProviderCredentialRevalidatesWorkerCapabilityRow(t *testing.T) {
	const credential = "hydrated-worker-siliconflow-credential"
	vault := testProviderSecretVault(t, "rag-worker-runtime-v1", 49)
	service := NewService(config.Config{}, WithProviderSecretVault(vault))
	recordID := ragProviderRecordID(RAGProviderSiliconFlow)
	secretRef, err := service.encryptRAGProviderSecretAtRest(
		auth.DevelopmentUserID,
		recordID,
		credential,
	)
	if err != nil {
		t.Fatal(err)
	}
	stored := StoredProviderConfig{
		ID: "77777777-7777-4777-8777-777777777777", UserID: auth.DevelopmentUserID,
		ProviderID: recordID, Label: "SiliconFlow",
		EncryptedSecretRef: secretRef,
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindRAG, RAGProvider: string(RAGProviderSiliconFlow),
			Enabled: true,
		},
	}
	stored.Config.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
		stored.ProviderID,
		RAGProviderSiliconFlow,
		stored.EncryptedSecretRef,
	)
	stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)

	resolved, err := service.ResolveHydratedStoredRAGProviderCredential(stored)
	if err != nil || resolved != credential {
		t.Fatalf("hydrated credential = %q/%v", resolved, err)
	}

	disabled := stored
	disabled.Config.Enabled = false
	_, err = service.ResolveHydratedStoredRAGProviderCredential(disabled)
	if !errors.Is(err, ragproviders.ErrProviderGatewayActivationRequired) {
		t.Fatalf("disabled hydrated provider error = %v", err)
	}

	drifted := stored
	drifted.Config.ConnectionTestSHA256 = strings.Repeat("0", 64)
	_, err = service.ResolveHydratedStoredRAGProviderCredential(drifted)
	if !errors.Is(err, ragproviders.ErrProviderGatewayActivationRequired) {
		t.Fatalf("drifted hydrated provider error = %v", err)
	}

	crossUser := stored
	crossUser.UserID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	_, err = service.ResolveHydratedStoredRAGProviderCredential(crossUser)
	if !errors.Is(err, ragproviders.ErrProviderGatewayUnavailable) ||
		strings.Contains(err.Error(), credential) {
		t.Fatalf("cross-user hydrated provider error = %v", err)
	}

	wrongKind := stored
	wrongKind.Config.Kind = providerConfigKindSearch
	_, err = service.ResolveHydratedStoredRAGProviderCredential(wrongKind)
	if !errors.Is(err, ragproviders.ErrProviderGatewayUnavailable) {
		t.Fatalf("cross-kind hydrated provider error = %v", err)
	}
}
