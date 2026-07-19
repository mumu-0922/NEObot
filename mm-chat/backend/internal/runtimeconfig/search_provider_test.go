package runtimeconfig

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/websearch"
)

type searchHTTPDoerFunc func(*http.Request) (*http.Response, error)

func (f searchHTTPDoerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAdminSearchProviderSaveTestActivateAndInvalidate(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	const fixtureCredential = "tavily-connection-fixture"
	client := searchHTTPDoerFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.tavily.com/search" ||
			request.Header.Get("Authorization") != "Bearer "+fixtureCredential {
			t.Fatalf("unexpected Search request %s", request.URL.Redacted())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"results":[{"title":"OpenAI","url":"https://openai.com/","content":"docs"}],"images":[]}`,
			)),
		}, nil
	})
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "search-v1", 31)),
		WithSearchProviderHTTPClient(client),
	)

	saved, err := service.UpsertAdminSearchProviderConfig(
		context.Background(),
		"tavily",
		UpdateAdminSearchProviderConfigRequest{
			Name: "Tavily Search",
			APIKeySecret: encryptedSecretEnvelope(
				t,
				privateKey,
				fixtureCredential,
				searchProviderIngressContext(websearch.ProviderTavily),
			),
		},
	)
	if err != nil {
		t.Fatalf("UpsertAdminSearchProviderConfig() error = %v", err)
	}
	if saved.Enabled || saved.ConnectionTestValid || !saved.HasAPIKey ||
		storedSecretAlgorithm(repo.stored.EncryptedSecretRef) == byokAlgorithm {
		t.Fatalf("saved Search provider = %#v", saved)
	}

	tested, err := service.TestAdminSearchProviderConnection(context.Background(), "tavily")
	if err != nil {
		t.Fatalf("TestAdminSearchProviderConnection() error = %v", err)
	}
	if tested.Provider.Enabled || !tested.Provider.ConnectionTestValid || tested.SourceCount != 1 {
		t.Fatalf("tested Search provider = %#v", tested)
	}
	if _, err := service.ResolveActive(context.Background()); !errors.Is(err, websearch.ErrNotConfigured) {
		t.Fatalf("tested-only ResolveActive() error = %v", err)
	}

	activated, err := service.ActivateAdminSearchProvider(context.Background(), "tavily")
	if err != nil {
		t.Fatalf("ActivateAdminSearchProvider() error = %v", err)
	}
	if !activated.Provider.Enabled || !activated.Provider.ConnectionTestValid {
		t.Fatalf("activated Search provider = %#v", activated)
	}
	execution, err := service.ResolveActive(context.Background())
	if err != nil || execution.Mode != websearch.ExecutionExternal || execution.External == nil ||
		execution.External.ID() != websearch.ProviderTavily {
		t.Fatalf("ResolveActive() = %#v, %v", execution, err)
	}

	updated, err := service.UpsertAdminSearchProviderConfig(
		context.Background(),
		"tavily",
		UpdateAdminSearchProviderConfigRequest{
			Name: "Tavily Search", BaseURL: "https://search.example.com", Enabled: true,
		},
	)
	if err != nil {
		t.Fatalf("Search endpoint update error = %v", err)
	}
	if updated.Enabled || updated.ConnectionTestValid ||
		repo.stored.Config.ConnectionTestSHA256 != "" ||
		repo.stored.Config.ConnectionTestedAt != "" {
		t.Fatalf("endpoint update retained activation: %#v", updated)
	}

	modelProviders, err := service.AdminProviderConfigs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(modelProviders.Providers) != 1 || modelProviders.Providers[0].ID != serverDefaultProviderID {
		t.Fatalf("Search row leaked into model providers: %#v", modelProviders)
	}
	if err := service.DeleteAdminSearchProviderConfig(context.Background(), "tavily"); err != nil {
		t.Fatalf("DeleteAdminSearchProviderConfig() error = %v", err)
	}
}

func TestAdminSearchProviderFailedTestDoesNotEnable(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "search-v1", 32)),
		WithSearchProviderHTTPClient(searchHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader(`{"secret":"must not escape"}`)),
			}, nil
		})),
	)
	_, err = service.UpsertAdminSearchProviderConfig(
		context.Background(),
		"exa",
		UpdateAdminSearchProviderConfigRequest{APIKeySecret: encryptedSecretEnvelope(
			t, privateKey, "wrong-fixture", searchProviderIngressContext(websearch.ProviderExa),
		)},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ActivateAdminSearchProvider(context.Background(), "exa")
	if !errors.Is(err, ErrSearchProviderConnectionFailed) {
		t.Fatalf("ActivateAdminSearchProvider() error = %v", err)
	}
	if repo.stored.Config.Enabled || repo.stored.Config.ConnectionTestSHA256 != "" ||
		repo.stored.Config.ConnectionTestedAt != "" {
		t.Fatalf("failed Search activation mutated config: %#v", repo.stored.Config)
	}
}

func TestSearchResolverUsesEligibleOpenAIModelBuiltInOnlyWithoutExternal(t *testing.T) {
	vault := testProviderSecretVault(t, "search-v1", 33)
	stored := testStoredVaultProvider(
		t,
		vault,
		"OPENAI_MODEL",
		ProviderTypeOpenAI,
		"https://api.openai.com/v1",
		"openai-model-fixture",
	)
	stored.Config.Models = []string{"gpt-5"}
	attestStoredProvider(&stored, true)
	repo := &fakeProviderConfigRepository{ok: true, stored: stored}
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
	)

	execution, err := service.ResolveActive(context.Background())
	if err != nil || execution.Mode != websearch.ExecutionModelBuiltIn ||
		execution.ModelBuiltIn != websearch.ModelBuiltInOpenAI {
		t.Fatalf("ResolveActive() = %#v, %v", execution, err)
	}

	repo.stored.Config.Type = ProviderTypeOpenAICompatible
	repo.stored.Config.ConnectionTestSHA256 = ProviderConnectionTestFingerprint(repo.stored)
	repo.stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := service.ResolveActive(context.Background()); !errors.Is(err, websearch.ErrNotConfigured) {
		t.Fatalf("OpenAI Compatible ResolveActive() error = %v", err)
	}
}

func TestSearchResolverDoesNotFallbackPastCorruptOrMultipleActiveExternalRows(t *testing.T) {
	const userID = "00000000-0000-0000-0000-000000000001"
	modelVault := testProviderSecretVault(t, "search-v1", 35)
	model := testStoredVaultProvider(
		t,
		modelVault,
		"OPENAI_MODEL",
		ProviderTypeOpenAI,
		"https://api.openai.com/v1",
		"openai-model-fixture",
	)
	model.Config.Models = []string{"gpt-5"}
	attestStoredProvider(&model, true)
	corrupt := StoredProviderConfig{
		ID: "search-corrupt", UserID: userID,
		ProviderID: searchProviderRecordID(websearch.ProviderExa),
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindSearch, SearchProvider: string(websearch.ProviderTavily),
			Enabled: true,
		},
	}
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{
			listed: []StoredProviderConfig{corrupt, model},
		}),
		WithProviderSecretVault(modelVault),
	)
	if _, err := service.ResolveActive(context.Background()); !errors.Is(err, websearch.ErrResolutionFailed) {
		t.Fatalf("corrupt active external ResolveActive() error = %v", err)
	}

	second := corrupt
	second.ID = "search-second"
	second.ProviderID = searchProviderRecordID(websearch.ProviderBocha)
	second.Config.SearchProvider = string(websearch.ProviderBocha)
	service = NewService(
		config.Config{},
		WithProviderConfigRepository(&fakeProviderConfigRepository{
			listed: []StoredProviderConfig{corrupt, second, model},
		}),
		WithProviderSecretVault(modelVault),
	)
	if _, err := service.ResolveActive(context.Background()); !errors.Is(err, websearch.ErrResolutionFailed) {
		t.Fatalf("multiple active external ResolveActive() error = %v", err)
	}
}
