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
)

func TestPublicConfigPublishesServerDefaultProviderWithoutSecret(t *testing.T) {
	service := NewService(config.Config{
		Provider: config.ProviderConfig{
			Type:   "openai_compatible",
			Name:   "Server Default",
			Model:  "gpt-5.5, gpt-5.5-mini, gpt-5.5",
			APIKey: "secret-value",
		},
		Auth:  config.AuthConfig{Mode: config.AuthModeRequired},
		Redis: config.RedisConfig{RateLimitEnabled: true},
		BYOK:  config.BYOKConfig{AllowEphemeralKey: true},
	})

	cfg := service.PublicConfig()
	if !cfg.ModelProvider.Available {
		t.Fatalf("model provider should be available")
	}
	if cfg.ModelProvider.Type != ProviderTypeOpenAICompatible {
		t.Fatalf("provider type = %q", cfg.ModelProvider.Type)
	}
	if got, want := cfg.ModelProvider.Models, []string{"gpt-5.5", "gpt-5.5-mini"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("models = %#v, want %#v", got, want)
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

func TestProviderModelsSupportsOnlyServerDefault(t *testing.T) {
	service := NewService(config.Config{Provider: config.ProviderConfig{Model: "gpt-a,gpt-b"}})

	response, err := service.ProviderModels(ProviderModelsRequest{Provider: ProviderRuntimeConfig{Source: "server-default"}})
	if err != nil {
		t.Fatalf("ProviderModels returned error: %v", err)
	}
	if got, want := response.Models, []string{"gpt-a", "gpt-b"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("models = %#v, want %#v", got, want)
	}

	if _, err := service.ProviderModels(ProviderModelsRequest{Provider: ProviderRuntimeConfig{APIKey: "secret"}}); err != ErrPlaintextProviderSecret {
		t.Fatalf("plaintext err = %v, want ErrPlaintextProviderSecret", err)
	}
	if _, err := service.ProviderModels(ProviderModelsRequest{Provider: ProviderRuntimeConfig{Source: "browser"}}); err != ErrProviderModelsUnsupported {
		t.Fatalf("source err = %v, want ErrProviderModelsUnsupported", err)
	}
}

func TestProviderModelsFetchesOpenAICompatibleModelsWithBYOK(t *testing.T) {
	const apiKey = "test-runtime-provider-key"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
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
			APIKeySecret: encryptedSecretEnvelope(t, privateKey, apiKey, "provider:OpenAI Compatible"),
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
		config.Config{Provider: config.ProviderConfig{Name: "Env", Model: "gpt-env"}},
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
	if got := repo.input.Config.Models; len(got) != 1 || got[0] != "gpt-5.5" {
		t.Fatalf("stored models = %#v", got)
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
			Provider: config.ProviderConfig{Name: "Env Default", Model: "gpt-env"},
			BYOK:     config.BYOKConfig{PrivateKeyPEM: pemValue},
		},
		WithProviderConfigRepository(repo),
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
	const apiKey = "stored-admin-key"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
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
	secret, err := json.Marshal(encryptedSecretEnvelope(t, privateKey, apiKey, "provider:OpenAI Compatible"))
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
