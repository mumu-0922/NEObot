package runtimeconfig

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/ragproviders"
)

func TestConfigureAdminRAGProviderTestsThenAtomicallyActivates(t *testing.T) {
	privateKey, pemValue := ragProviderTestBYOKKey(t)
	const fixtureCredential = "mineru-atomic-config-fixture"
	requests := 0
	client := searchHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.String() != minerUAllocateURL ||
			request.Header.Get("Authorization") != "Bearer "+fixtureCredential {
			t.Fatalf("unexpected MinerU request %s", request.URL.Redacted())
		}
		return ragProviderJSONResponse(t, map[string]any{
			"code": 0,
			"data": map[string]any{
				"batch_id": "mm-chat-atomic-config",
				"file_urls": []string{
					"https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/atomic.pdf?signature=test",
				},
			},
		}), nil
	})
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "rag-atomic-v1", 61)),
		WithRAGProviderHTTPClient(client),
	)

	configured, err := service.ConfigureAdminRAGProvider(
		context.Background(),
		"mineru",
		ConfigureAdminRAGProviderRequest{APIKeySecret: encryptedSecretEnvelope(
			t,
			privateKey,
			fixtureCredential,
			ragProviderIngressContext(RAGProviderMinerU),
		)},
	)
	if err != nil {
		t.Fatalf("ConfigureAdminRAGProvider() error = %v", err)
	}
	if requests != 1 || repo.configureCalls != 1 ||
		!configured.Provider.Enabled || !configured.Provider.ConnectionTestValid ||
		!configured.Provider.HasAPIKey || strings.Join(configured.Checks, ",") != "allocate" ||
		storedSecretAlgorithm(repo.stored.EncryptedSecretRef) != "A256GCM" ||
		strings.Contains(repo.stored.EncryptedSecretRef, fixtureCredential) {
		t.Fatalf(
			"configured MinerU = %#v requests=%d commits=%d stored=%#v",
			configured,
			requests,
			repo.configureCalls,
			repo.stored,
		)
	}
	status, err := service.RAGProviderStatus(context.Background())
	if err != nil || status.Ready ||
		status.Status != ragproviders.ServiceStatusUnavailable ||
		!status.Capabilities.PDFParsing || status.Capabilities.NativeIndexing ||
		status.Capabilities.Retrieval {
		t.Fatalf("MinerU-only status = %#v, %v", status, err)
	}
}

func TestConfigureAdminRAGProviderFailedReplacementPreservesActiveRecord(t *testing.T) {
	privateKey, pemValue := ragProviderTestBYOKKey(t)
	vault := testProviderSecretVault(t, "rag-atomic-v1", 62)
	const userID = "00000000-0000-0000-0000-000000000001"
	recordID := ragProviderRecordID(RAGProviderMinerU)
	seed := NewService(config.Config{}, WithProviderSecretVault(vault))
	oldSecretRef, err := seed.encryptRAGProviderSecretAtRest(
		userID,
		recordID,
		"working-mineru-fixture",
	)
	if err != nil {
		t.Fatal(err)
	}
	testedAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	before := StoredProviderConfig{
		ID: "active-mineru-config", UserID: userID, ProviderID: recordID,
		Label: "MinerU", EncryptedSecretRef: oldSecretRef,
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindRAG, RAGProvider: string(RAGProviderMinerU),
			Enabled: true, ConnectionTestSHA256: ragProviderConnectionFingerprint(
				recordID,
				RAGProviderMinerU,
				oldSecretRef,
			),
			ConnectionTestedAt: testedAt,
		},
	}
	repo := &fakeProviderConfigRepository{stored: before, ok: true}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
		WithRAGProviderHTTPClient(searchHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":"invalid key"}`)),
			}, nil
		})),
	)

	_, err = service.ConfigureAdminRAGProvider(
		context.Background(),
		"mineru",
		ConfigureAdminRAGProviderRequest{APIKeySecret: encryptedSecretEnvelope(
			t,
			privateKey,
			"invalid-mineru-replacement",
			ragProviderIngressContext(RAGProviderMinerU),
		)},
	)
	if !errors.Is(err, ErrRAGProviderConnectionFailed) {
		t.Fatalf("ConfigureAdminRAGProvider() error = %v", err)
	}
	if repo.configureCalls != 0 || !reflect.DeepEqual(repo.stored, before) {
		t.Fatalf(
			"failed replacement mutated active record: calls=%d\nbefore=%#v\nafter=%#v",
			repo.configureCalls,
			before,
			repo.stored,
		)
	}
	status, statusErr := service.RAGProviderStatus(context.Background())
	if statusErr != nil ||
		status.Providers.MinerU.Status != ragproviders.ProviderStatusReady {
		t.Fatalf("preserved provider status = %#v, %v", status, statusErr)
	}
}

func TestConfigureAdminRAGProviderMapsAtomicConcurrencyConflict(t *testing.T) {
	privateKey, pemValue := ragProviderTestBYOKKey(t)
	repo := &fakeProviderConfigRepository{configureErr: ErrProviderConfigChanged}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "rag-atomic-v1", 63)),
		WithRAGProviderHTTPClient(searchHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return ragProviderJSONResponse(t, map[string]any{
				"code": 0,
				"data": map[string]any{
					"batch_id": "mm-chat-conflict",
					"file_urls": []string{
						"https://mineru.oss-cn-shanghai.aliyuncs.com/api-upload/conflict.pdf?signature=test",
					},
				},
			}), nil
		})),
	)

	_, err := service.ConfigureAdminRAGProvider(
		context.Background(),
		"mineru",
		ConfigureAdminRAGProviderRequest{APIKeySecret: encryptedSecretEnvelope(
			t,
			privateKey,
			"conflicting-mineru-key",
			ragProviderIngressContext(RAGProviderMinerU),
		)},
	)
	if !errors.Is(err, ErrRAGProviderConfigChanged) || repo.configureCalls != 1 || repo.ok {
		t.Fatalf("concurrent configure error = %v repo=%#v", err, repo)
	}
}

func TestConfigureAdminRAGProviderMissingVaultSkipsProviderCall(t *testing.T) {
	privateKey, pemValue := ragProviderTestBYOKKey(t)
	requests := 0
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(&fakeProviderConfigRepository{}),
		WithRAGProviderHTTPClient(searchHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			requests++
			return nil, errors.New("provider call must not run")
		})),
	)

	_, err := service.ConfigureAdminRAGProvider(
		context.Background(),
		"mineru",
		ConfigureAdminRAGProviderRequest{APIKeySecret: encryptedSecretEnvelope(
			t,
			privateKey,
			"vault-missing-mineru-key",
			ragProviderIngressContext(RAGProviderMinerU),
		)},
	)
	if !errors.Is(err, ErrRAGProviderSecretVaultUnavailable) || requests != 0 {
		t.Fatalf("missing vault error = %v requests=%d", err, requests)
	}
}
