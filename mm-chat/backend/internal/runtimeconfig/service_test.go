package runtimeconfig

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/providersecrets"
)

func TestPublicConfigDoesNotUseEnvironmentProviderFallback(t *testing.T) {
	service := NewService(config.Config{
		Auth:  config.AuthConfig{Mode: config.AuthModeRequired},
		Redis: config.RedisConfig{RateLimitEnabled: true},
		BYOK:  config.BYOKConfig{AllowEphemeralKey: true},
	})

	cfg := service.PublicConfig()
	if cfg.ModelProvider.Available {
		t.Fatalf("model provider should be unavailable without Postgres activation")
	}
	if cfg.ModelProvider.Type != ProviderTypeOpenAICompatible {
		t.Fatalf("provider type = %q", cfg.ModelProvider.Type)
	}
	if len(cfg.ModelProvider.Models) != 0 {
		t.Fatalf("models = %#v, want empty", cfg.ModelProvider.Models)
	}
	if cfg.ModelProvider.Models == nil {
		t.Fatal("empty public model list must serialize as an array")
	}
	if cfg.Deployment.Mode != "hosted" {
		t.Fatalf("deployment mode = %q", cfg.Deployment.Mode)
	}
	if cfg.Deployment.RateLimitStore != "shared" {
		t.Fatalf("rate limit store = %q", cfg.Deployment.RateLimitStore)
	}
	if cfg.Deployment.PluginRegistryStore != "memory" {
		t.Fatalf("plugin registry store = %q", cfg.Deployment.PluginRegistryStore)
	}
	if !cfg.Deployment.BYOKEphemeralAllowed {
		t.Fatalf("expected BYOK ephemeral flag")
	}
}

func TestPublicConfigPublishesSharedPluginRegistryWhenDatabaseConfigured(t *testing.T) {
	service := NewService(config.Config{DatabaseURL: "postgres://mm-chat"})

	cfg := service.PublicConfig()

	if cfg.Deployment.PluginRegistryStore != "shared" {
		t.Fatalf("plugin registry store = %q, want shared", cfg.Deployment.PluginRegistryStore)
	}
}

func TestPublicConfigPublishesInjectedSearchAvailability(t *testing.T) {
	service := NewService(config.Config{}, WithSearchAvailable(true))

	if !service.PublicConfig().Search.Available {
		t.Fatalf("search should be available")
	}
}

func TestProviderModelsRejectsUnavailableServerAuthority(t *testing.T) {
	service := NewService(config.Config{})
	if _, err := service.ProviderModels(ProviderModelsRequest{Provider: ProviderRuntimeConfig{Source: "server-default"}}); err != ErrDatabaseRequired {
		t.Fatalf("server-default err = %v, want ErrDatabaseRequired", err)
	}

	if _, err := service.ProviderModels(ProviderModelsRequest{Provider: ProviderRuntimeConfig{APIKey: "secret"}}); err != ErrPlaintextProviderSecret {
		t.Fatalf("plaintext err = %v, want ErrPlaintextProviderSecret", err)
	}
	if _, err := service.ProviderModels(ProviderModelsRequest{Provider: ProviderRuntimeConfig{Source: "browser"}}); err != ErrProviderModelsUnsupported {
		t.Fatalf("source err = %v, want ErrProviderModelsUnsupported", err)
	}
}

func TestProviderModelsFetchesOpenAICompatibleModelsWithBYOK(t *testing.T) {
	const fixtureCredential = "runtime-provider-fixture"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+fixtureCredential {
			t.Fatalf("Authorization = %q, want bearer key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-a"},{"id":"gpt-b"},{"id":"gpt-a"}]}`))
	}))
	defer upstream.Close()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	service := NewService(config.Config{
		Provider: config.ProviderConfig{Timeout: 2 * time.Second},
		BYOK:     config.BYOKConfig{PrivateKeyPEM: pemValue},
	})

	response, err := service.ProviderModels(ProviderModelsRequest{
		Provider: ProviderRuntimeConfig{
			ID:           "CUSTOM",
			Type:         "OpenAI Compatible",
			BaseURL:      upstream.URL,
			APIKeySecret: encryptedSecretEnvelope(t, privateKey, fixtureCredential, "provider:OpenAI Compatible"),
		},
	})
	if err != nil {
		t.Fatalf("ProviderModels returned error: %v", err)
	}
	if got, want := response.Models, []string{"gpt-a", "gpt-b"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("models = %#v, want %#v", got, want)
	}
}

func TestBYOKPublicKeyFromConfiguredPEM(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	service := NewService(config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue, KeyID: "kid-test"}})

	response, err := service.BYOKPublicKey()
	if err != nil {
		t.Fatalf("BYOKPublicKey returned error: %v", err)
	}
	if response.KID != "kid-test" || response.Alg != byokAlgorithm {
		t.Fatalf("metadata = %#v", response)
	}
	if response.PublicKeyJWK["alg"] != "RSA-OAEP-256" {
		t.Fatalf("public JWK alg = %#v, want RSA-OAEP-256", response.PublicKeyJWK["alg"])
	}
	if response.PublicKeyJWK["kty"] != "RSA" || response.PublicKeyJWK["n"] == "" || response.PublicKeyJWK["e"] == "" {
		t.Fatalf("invalid jwk = %#v", response.PublicKeyJWK)
	}
}

func TestBYOKPublicKeyRequiresConfiguredOrEphemeralKey(t *testing.T) {
	service := NewService(config.Config{})
	if _, err := service.BYOKPublicKey(); err != ErrBYOKNotConfigured {
		t.Fatalf("err = %v, want ErrBYOKNotConfigured", err)
	}

	ephemeral := NewService(config.Config{BYOK: config.BYOKConfig{AllowEphemeralKey: true}})
	first, err := ephemeral.BYOKPublicKey()
	if err != nil {
		t.Fatalf("first BYOKPublicKey returned error: %v", err)
	}
	second, err := ephemeral.BYOKPublicKey()
	if err != nil {
		t.Fatalf("second BYOKPublicKey returned error: %v", err)
	}
	if first.KID != second.KID {
		t.Fatalf("ephemeral key was not reused: %q != %q", first.KID, second.KID)
	}
}

func encryptedSecretEnvelope(
	t *testing.T,
	privateKey *rsa.PrivateKey,
	plaintext string,
	context string,
) map[string]any {
	t.Helper()

	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	ciphertext := gcm.Seal(nil, iv, []byte(plaintext), []byte(context))
	wrappedKey, err := rsa.EncryptOAEP(
		sha256.New(),
		rand.Reader,
		&privateKey.PublicKey,
		aesKey,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	kid, err := deriveKeyID(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"v":          float64(1),
		"kid":        kid,
		"alg":        byokAlgorithm,
		"iv":         base64.RawURLEncoding.EncodeToString(iv),
		"wrappedKey": base64.RawURLEncoding.EncodeToString(wrappedKey),
		"ciphertext": base64.RawURLEncoding.EncodeToString(ciphertext),
		"context":    context,
	}
}

type fakeProviderConfigRepository struct {
	stored StoredProviderConfig
	ok     bool
	input  UpsertProviderConfigInput
}

func (r *fakeProviderConfigRepository) GetProviderConfig(_ context.Context, userID string, providerID string) (StoredProviderConfig, bool, error) {
	if !r.ok || r.stored.UserID != userID || r.stored.ProviderID != providerID {
		return StoredProviderConfig{}, false, nil
	}
	return r.stored, true, nil
}

func (r *fakeProviderConfigRepository) ListProviderConfigs(_ context.Context, userID string) ([]StoredProviderConfig, error) {
	if !r.ok || r.stored.UserID != userID {
		return []StoredProviderConfig{}, nil
	}
	return []StoredProviderConfig{r.stored}, nil
}

func (r *fakeProviderConfigRepository) UpsertProviderConfig(_ context.Context, input UpsertProviderConfigInput) (StoredProviderConfig, error) {
	r.input = input
	r.stored = StoredProviderConfig{
		ID:                 "provider-config-1",
		UserID:             input.UserID,
		ProviderID:         input.ProviderID,
		Label:              input.Label,
		EncryptedSecretRef: input.EncryptedSecretRef,
		Config:             input.Config,
	}
	r.ok = true
	return r.stored, nil
}

func (r *fakeProviderConfigRepository) CommitProviderConnection(
	_ context.Context,
	input CommitProviderConnectionInput,
) (StoredProviderConfig, error) {
	if !r.ok || r.stored.ID != input.ID || r.stored.UserID != input.UserID ||
		r.stored.ProviderID != input.ProviderID ||
		r.stored.EncryptedSecretRef != input.ExpectedEncryptedSecretRef ||
		r.stored.Config.Type != input.ExpectedType ||
		r.stored.Config.BaseURL != input.ExpectedBaseURL ||
		r.stored.Config.Enabled != input.ExpectedEnabled {
		return StoredProviderConfig{}, ErrProviderConfigChanged
	}
	r.stored.Config.ConnectionTestSHA256 = input.ConnectionTestSHA256
	r.stored.Config.ConnectionTestedAt = input.ConnectionTestedAt.UTC().Format(time.RFC3339Nano)
	r.stored.Config.Enabled = input.Enabled
	return r.stored, nil
}

func (r *fakeProviderConfigRepository) DeleteProviderConfig(_ context.Context, userID string, providerID string) error {
	if !r.ok || r.stored.UserID != userID || r.stored.ProviderID != providerID {
		return ErrProviderConfigNotFound
	}
	r.ok = false
	return nil
}

func TestAdminProviderConfigOverridesPublicServerDefault(t *testing.T) {
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: StoredProviderConfig{
			UserID:     "00000000-0000-0000-0000-000000000001",
			ProviderID: serverDefaultProviderID,
			Label:      "Admin Configured",
			Config: StoredProviderConfigPayload{
				Type:    ProviderTypeOpenAICompatible,
				BaseURL: "https://provider.example/v1",
				Models:  []string{"gpt-admin", "gpt-admin"},
				Enabled: true,
			},
		},
	}
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
	)

	public := service.PublicConfigForContext(context.Background())
	if public.ModelProvider.Name != "Admin Configured" {
		t.Fatalf("provider name = %q", public.ModelProvider.Name)
	}
	if got, want := public.ModelProvider.Models, []string{"gpt-admin"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("models = %#v, want %#v", got, want)
	}

	admin, err := service.AdminProviderConfig(context.Background())
	if err != nil {
		t.Fatalf("AdminProviderConfig returned error: %v", err)
	}
	if admin.BaseURL != "https://provider.example/v1" || admin.HasAPIKey {
		t.Fatalf("admin config = %#v", admin)
	}
}

func TestUpdateAdminProviderConfigStoresEncryptedSecretEnvelope(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "model-v1", 11)),
	)

	response, err := service.UpdateAdminProviderConfig(context.Background(), UpdateAdminProviderConfigRequest{
		Name:         "mumuapi",
		Type:         "OpenAI Compatible",
		BaseURL:      "https://sub.example/v1",
		Models:       []string{"gpt-5.5", "gpt-5.5"},
		APIKeySecret: encryptedSecretEnvelope(t, privateKey, "admin-provider-key", "provider:OpenAI Compatible"),
	})
	if err != nil {
		t.Fatalf("UpdateAdminProviderConfig returned error: %v", err)
	}
	if !response.HasAPIKey || response.Name != "mumuapi" {
		t.Fatalf("response = %#v", response)
	}
	if repo.input.EncryptedSecretRef == "" || strings.Contains(repo.input.EncryptedSecretRef, "admin-provider-key") {
		t.Fatalf("encrypted secret ref was not persisted safely: %q", repo.input.EncryptedSecretRef)
	}
	if !strings.Contains(repo.input.EncryptedSecretRef, `"alg":"A256GCM"`) ||
		strings.Contains(repo.input.EncryptedSecretRef, byokAlgorithm) {
		t.Fatalf("stored secret is not a vault envelope: %q", repo.input.EncryptedSecretRef)
	}
	if got := repo.input.Config.Models; len(got) != 1 || got[0] != "gpt-5.5" {
		t.Fatalf("stored models = %#v", got)
	}
}

func TestUpdateAdminProviderConfigRequiresAtRestVault(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(&fakeProviderConfigRepository{}),
	)

	_, err = service.UpdateAdminProviderConfig(
		context.Background(),
		UpdateAdminProviderConfigRequest{
			Type: "OpenAI Compatible",
			APIKeySecret: encryptedSecretEnvelope(
				t, privateKey, "fixture-key", "provider:OpenAI Compatible",
			),
		},
	)
	if err != ErrProviderSecretVaultUnavailable {
		t.Fatalf("UpdateAdminProviderConfig error = %v", err)
	}
}

func TestStoredVaultSecretSurvivesServiceRestart(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	repo := &fakeProviderConfigRepository{}
	first := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "model-v1", 13)),
	)
	_, err = first.UpsertAdminProviderConfig(
		context.Background(),
		"CUSTOM",
		UpdateAdminProviderConfigRequest{
			Name: "Restarted", Type: "OpenAI Compatible", Enabled: true,
			APIKeySecret: encryptedSecretEnvelope(
				t, privateKey, "restart-secret", "provider:OpenAI Compatible",
			),
		},
	)
	if err != nil {
		t.Fatalf("first UpsertAdminProviderConfig error = %v", err)
	}
	attestStoredProvider(&repo.stored, true)

	second := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "model-v1", 13)),
	)
	resolved, err := second.ResolveStoredProvider(context.Background(), "CUSTOM")
	if err != nil {
		t.Fatalf("second ResolveStoredProvider error = %v", err)
	}
	if resolved.APIKey != "restart-secret" {
		t.Fatalf("resolved secret = %q", resolved.APIKey)
	}

	repo.stored.ProviderID = "OTHER"
	attestStoredProvider(&repo.stored, true)
	if _, err := second.ResolveStoredProvider(context.Background(), "OTHER"); err != ErrProviderSecretInvalid {
		t.Fatalf("copied envelope error = %v", err)
	}
}

func TestServerDefaultMetadataSaveDoesNotInheritEnvironmentSecret(t *testing.T) {
	repo := &fakeProviderConfigRepository{}
	vault := testProviderSecretVault(t, "model-v1", 14)
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
	)
	_, err := service.UpdateAdminProviderConfig(
		context.Background(),
		UpdateAdminProviderConfigRequest{
			Name: "Imported", Type: "OpenAI Compatible", Models: []string{"gpt-env"},
		},
	)
	if err != nil {
		t.Fatalf("UpdateAdminProviderConfig error = %v", err)
	}
	if repo.stored.EncryptedSecretRef != "" {
		t.Fatalf("stored ref = %q, want empty", repo.stored.EncryptedSecretRef)
	}
	if _, err := service.ResolveServerDefaultProvider(context.Background()); err != ErrProviderDisabled {
		t.Fatalf("resolver error = %v, want ErrProviderDisabled", err)
	}
}

func TestCustomProviderStartsWithoutSecret(t *testing.T) {
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "model-v1", 15)),
	)
	created, err := service.UpsertAdminProviderConfig(
		context.Background(),
		"CUSTOM",
		UpdateAdminProviderConfigRequest{Name: "Custom", Type: "OpenAI Compatible"},
	)
	if err != nil {
		t.Fatalf("UpsertAdminProviderConfig error = %v", err)
	}
	if created.HasAPIKey || repo.stored.EncryptedSecretRef != "" {
		t.Fatalf("custom provider inherited default secret: %#v / %q", created, repo.stored.EncryptedSecretRef)
	}
}

func TestMetadataSaveMigratesLegacyIngressEnvelopeToVault(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	legacy, err := json.Marshal(encryptedSecretEnvelope(
		t, privateKey, "legacy-secret", "provider:OpenAI Compatible",
	))
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: StoredProviderConfig{
			UserID: "00000000-0000-0000-0000-000000000001", ProviderID: "CUSTOM",
			EncryptedSecretRef: string(legacy),
			Config:             StoredProviderConfigPayload{Type: ProviderTypeOpenAICompatible, Enabled: true},
		},
	}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "model-v1", 16)),
	)
	_, err = service.UpsertAdminProviderConfig(
		context.Background(),
		"CUSTOM",
		UpdateAdminProviderConfigRequest{Name: "Migrated", Type: "OpenAI Compatible", Enabled: true},
	)
	if err != nil {
		t.Fatalf("UpsertAdminProviderConfig error = %v", err)
	}
	if storedSecretAlgorithm(repo.stored.EncryptedSecretRef) != providersecrets.Algorithm {
		t.Fatalf("stored ref was not migrated: %q", repo.stored.EncryptedSecretRef)
	}
}

func TestAdminProviderConfigsManageCustomProviderOnBackend(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{
			BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue},
		},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "model-v1", 12)),
	)

	created, err := service.UpsertAdminProviderConfig(context.Background(), "CUSTOM", UpdateAdminProviderConfigRequest{
		Name:         "Custom Backend",
		Type:         "OpenAI Compatible",
		BaseURL:      "https://custom.example/v1",
		Models:       []string{"gpt-custom"},
		Enabled:      true,
		APIKeySecret: encryptedSecretEnvelope(t, privateKey, "custom-key", "provider:OpenAI Compatible"),
	})
	if err != nil {
		t.Fatalf("UpsertAdminProviderConfig returned error: %v", err)
	}
	if created.ID != "CUSTOM" || created.Source != "server-stored" || !created.HasAPIKey {
		t.Fatalf("created = %#v", created)
	}
	attestStoredProvider(&repo.stored, true)

	listed, err := service.AdminProviderConfigs(context.Background())
	if err != nil {
		t.Fatalf("AdminProviderConfigs returned error: %v", err)
	}
	if len(listed.Providers) != 2 {
		t.Fatalf("providers = %#v, want env default plus custom", listed.Providers)
	}
	if listed.Providers[0].ID != serverDefaultProviderID || listed.Providers[1].ID != "CUSTOM" {
		t.Fatalf("provider order = %#v", listed.Providers)
	}
	if listed.Providers[0].Models == nil {
		t.Fatal("empty Server Default models must serialize as an array")
	}

	resolved, err := service.ResolveStoredProvider(context.Background(), "CUSTOM")
	if err != nil {
		t.Fatalf("ResolveStoredProvider returned error: %v", err)
	}
	if resolved.APIKey != "custom-key" || resolved.BaseURL != "https://custom.example/v1" {
		t.Fatalf("resolved = %#v", resolved)
	}

	if err := service.DeleteAdminProviderConfig(context.Background(), "CUSTOM"); err != nil {
		t.Fatalf("DeleteAdminProviderConfig returned error: %v", err)
	}
	if _, err := service.ResolveStoredProvider(context.Background(), "CUSTOM"); err != ErrProviderConfigNotFound {
		t.Fatalf("resolve deleted err = %v, want ErrProviderConfigNotFound", err)
	}
}

func TestProviderModelsUsesStoredServerDefaultSecret(t *testing.T) {
	const fixtureCredential = "stored-admin-fixture"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+fixtureCredential {
			t.Fatalf("Authorization = %q, want stored key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-live"}]}`))
	}))
	defer upstream.Close()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	secret, err := json.Marshal(encryptedSecretEnvelope(t, privateKey, fixtureCredential, "provider:OpenAI Compatible"))
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: StoredProviderConfig{
			UserID:             "00000000-0000-0000-0000-000000000001",
			ProviderID:         serverDefaultProviderID,
			Label:              "Stored Default",
			EncryptedSecretRef: string(secret),
			Config: StoredProviderConfigPayload{
				Type:    ProviderTypeOpenAICompatible,
				BaseURL: upstream.URL,
				Models:  []string{},
				Enabled: true,
			},
		},
	}
	attestStoredProvider(&repo.stored, true)
	service := NewService(
		config.Config{Provider: config.ProviderConfig{Timeout: 2 * time.Second}, BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
	)

	response, err := service.ProviderModelsForContext(context.Background(), ProviderModelsRequest{Provider: ProviderRuntimeConfig{Source: "server-default"}})
	if err != nil {
		t.Fatalf("ProviderModelsForContext returned error: %v", err)
	}
	if got := response.Models; len(got) != 1 || got[0] != "gpt-live" {
		t.Fatalf("models = %#v", got)
	}
}

func TestAdminProviderConnectionTestAndActivationGateRuntime(t *testing.T) {
	const fixtureCredential = "connection-fixture"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer "+fixtureCredential {
			t.Fatalf("unexpected provider request path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-a"},{"id":"gpt-a"},{"id":"gpt-b"}]}`))
	}))
	defer upstream.Close()

	vault := testProviderSecretVault(t, "model-v1", 19)
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: testStoredVaultProvider(
			t,
			vault,
			"CUSTOM",
			ProviderTypeOpenAICompatible,
			upstream.URL,
			fixtureCredential,
		),
	}
	service := NewService(
		config.Config{Provider: config.ProviderConfig{Timeout: time.Second}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
	)

	if _, err := service.ResolveStoredProvider(context.Background(), "CUSTOM"); err != ErrProviderDisabled {
		t.Fatalf("pre-test ResolveStoredProvider error = %v", err)
	}
	tested, err := service.TestAdminProviderConnection(context.Background(), "CUSTOM")
	if err != nil {
		t.Fatalf("TestAdminProviderConnection error = %v", err)
	}
	if tested.Provider.Enabled || !tested.Provider.ConnectionTestValid ||
		len(tested.Models) != 2 || tested.Models[0] != "gpt-a" ||
		tested.Provider.ConnectionTestedAt == nil {
		t.Fatalf("tested response = %#v", tested)
	}
	if _, err := service.ResolveStoredProvider(context.Background(), "CUSTOM"); err != ErrProviderDisabled {
		t.Fatalf("tested-but-disabled resolver error = %v", err)
	}

	activated, err := service.ActivateAdminProvider(context.Background(), "CUSTOM")
	if err != nil {
		t.Fatalf("ActivateAdminProvider error = %v", err)
	}
	if !activated.Provider.Enabled || !activated.Provider.ConnectionTestValid {
		t.Fatalf("activated response = %#v", activated)
	}
	resolved, err := service.ResolveStoredProvider(context.Background(), "CUSTOM")
	if err != nil || resolved.APIKey != fixtureCredential {
		t.Fatalf("resolved provider = %#v, %v", resolved, err)
	}

	updated, err := service.UpsertAdminProviderConfig(
		context.Background(),
		"CUSTOM",
		UpdateAdminProviderConfigRequest{
			Name: "Changed", Type: "OpenAI Compatible",
			BaseURL: upstream.URL + "/changed", Models: []string{"gpt-a"}, Enabled: true,
		},
	)
	if err != nil {
		t.Fatalf("invalidating update error = %v", err)
	}
	if updated.Enabled || updated.ConnectionTestValid ||
		repo.stored.Config.ConnectionTestSHA256 != "" ||
		repo.stored.Config.ConnectionTestedAt != "" {
		t.Fatalf("invalidating update retained activation: %#v / %#v", updated, repo.stored.Config)
	}
	if _, err := service.ResolveStoredProvider(context.Background(), "CUSTOM"); err != ErrProviderDisabled {
		t.Fatalf("invalidated resolver error = %v", err)
	}
}

func TestAdminProviderConnectionFailureDoesNotAttestOrEnable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"must not escape"}}`))
	}))
	defer upstream.Close()

	vault := testProviderSecretVault(t, "model-v1", 20)
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: testStoredVaultProvider(
			t,
			vault,
			"CUSTOM",
			ProviderTypeOpenAICompatible,
			upstream.URL,
			"wrong-key",
		),
	}
	service := NewService(
		config.Config{Provider: config.ProviderConfig{Timeout: time.Second}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
	)

	_, err := service.ActivateAdminProvider(context.Background(), "CUSTOM")
	if err != ErrProviderConnectionTestFailed {
		t.Fatalf("ActivateAdminProvider error = %v", err)
	}
	if repo.stored.Config.Enabled || repo.stored.Config.ConnectionTestSHA256 != "" ||
		repo.stored.Config.ConnectionTestedAt != "" {
		t.Fatalf("failed activation mutated config = %#v", repo.stored.Config)
	}
}

func TestAdminProviderConnectionSupportsGeminiModelListing(t *testing.T) {
	const fixtureCredential = "gemini-fixture"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" || r.Header.Get("x-goog-api-key") != fixtureCredential ||
			r.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected Gemini request path=%q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-a"},{"name":"gemini-b"}]}`))
	}))
	defer upstream.Close()

	vault := testProviderSecretVault(t, "model-v1", 21)
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: testStoredVaultProvider(
			t,
			vault,
			"GEMINI",
			ProviderTypeGemini,
			upstream.URL,
			fixtureCredential,
		),
	}
	service := NewService(
		config.Config{Provider: config.ProviderConfig{Timeout: time.Second}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
	)

	response, err := service.TestAdminProviderConnection(context.Background(), "GEMINI")
	if err != nil {
		t.Fatalf("TestAdminProviderConnection error = %v", err)
	}
	if got := response.Models; len(got) != 2 || got[0] != "gemini-a" || got[1] != "gemini-b" {
		t.Fatalf("Gemini models = %#v", got)
	}
}

func testStoredVaultProvider(
	t *testing.T,
	vault *providersecrets.Vault,
	providerID string,
	providerType ProviderType,
	baseURL string,
	apiKey string,
) StoredProviderConfig {
	t.Helper()
	const userID = "00000000-0000-0000-0000-000000000001"
	plaintext := []byte(apiKey)
	envelope, err := vault.Encrypt(plaintext, modelProviderSecretContext(userID, providerID))
	clear(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return StoredProviderConfig{
		ID:                 "10000000-0000-4000-8000-000000000001",
		UserID:             userID,
		ProviderID:         providerID,
		Label:              providerID,
		EncryptedSecretRef: string(encoded),
		Config: StoredProviderConfigPayload{
			Type: providerType, BaseURL: baseURL, Enabled: false,
		},
	}
}

func attestStoredProvider(stored *StoredProviderConfig, enabled bool) {
	stored.Config.ConnectionTestSHA256 = providerConnectionFingerprint(
		stored.ProviderID,
		stored.Config.Type,
		stored.Config.BaseURL,
		stored.EncryptedSecretRef,
	)
	stored.Config.ConnectionTestedAt = time.Now().UTC().Format(time.RFC3339Nano)
	stored.Config.Enabled = enabled
}

func testProviderSecretVault(t *testing.T, kid string, fill byte) *providersecrets.Vault {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString(
		[]byte(strings.Repeat(string([]byte{fill}), 32)),
	)
	vault, err := providersecrets.NewVault(providersecrets.KeyringConfig{
		V:         providersecrets.KeyringVersion,
		ActiveKID: kid,
		Keys:      []providersecrets.KeyConfig{{KID: kid, Key: encoded}},
	})
	if err != nil {
		t.Fatalf("NewVault() error = %v", err)
	}
	return vault
}

func TestParseStoredLegacySecretRefIsClosedAndBounded(t *testing.T) {
	valid := `{"v":1,"kid":"fixture","alg":"RSA-OAEP-256+A256GCM",` +
		`"wrappedKey":"a","iv":"b","ciphertext":"c","context":"provider:OpenAI"}`
	if _, err := parseStoredLegacySecretRef(valid); err != nil {
		t.Fatalf("valid stored legacy envelope error = %v", err)
	}
	for name, encoded := range map[string]string{
		"unknown":  strings.TrimSuffix(valid, "}") + `,"extra":true}`,
		"trailing": valid + `{}`,
		"oversize": strings.Repeat("x", maxStoredProviderSecretRefBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseStoredLegacySecretRef(encoded); err != ErrProviderSecretInvalid {
				t.Fatalf("parseStoredLegacySecretRef() error = %v", err)
			}
		})
	}
}
