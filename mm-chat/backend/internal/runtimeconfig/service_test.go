package runtimeconfig

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

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
	if !cfg.Deployment.BYOKEphemeralAllowed {
		t.Fatalf("expected BYOK ephemeral flag")
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
	if _, err := service.ProviderModels(ProviderModelsRequest{Provider: ProviderRuntimeConfig{APIKeySecret: map[string]any{"v": 1}}}); err != ErrProviderModelsUnsupported {
		t.Fatalf("BYOK err = %v, want ErrProviderModelsUnsupported", err)
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
