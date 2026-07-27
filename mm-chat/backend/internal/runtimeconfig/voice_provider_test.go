package runtimeconfig

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/config"
)

type voiceRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f voiceRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAdminSiliconFlowVoiceSaveTestActivateResolveAndInvalidate(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	const fixtureCredential = "siliconflow-production-fixture"
	client := &http.Client{Transport: voiceRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost ||
			request.URL.String() != SiliconFlowVoiceBaseURL+"/audio/speech" ||
			request.Header.Get("Authorization") != "Bearer "+fixtureCredential {
			t.Fatalf("unexpected Voice request %s %s", request.Method, request.URL.Redacted())
		}
		var payload struct {
			Model string `json:"model"`
			Voice string `json:"voice"`
			Input string `json:"input"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != SiliconFlowVoiceModelID || payload.Voice != SiliconFlowVoiceID ||
			strings.TrimSpace(payload.Input) == "" {
			t.Fatalf("unexpected Voice tuple %#v", payload)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       io.NopCloser(strings.NewReader("ID3-audio-fixture")),
		}, nil
	})}
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "voice-v1", 51)),
		WithVoiceProviderHTTPClient(client),
	)

	saved, err := service.UpsertAdminVoiceProviderConfig(
		context.Background(),
		"siliconflow",
		UpdateAdminVoiceProviderConfigRequest{APIKeySecret: encryptedSecretEnvelope(
			t,
			privateKey,
			fixtureCredential,
			voiceProviderIngressContext(voiceProviderSiliconFlow),
		)},
	)
	if err != nil {
		t.Fatalf("UpsertAdminVoiceProviderConfig() error = %v", err)
	}
	if saved.Enabled || saved.ConnectionTestValid || !saved.HasAPIKey ||
		storedSecretAlgorithm(repo.stored.EncryptedSecretRef) == byokAlgorithm {
		t.Fatalf("saved Voice provider = %#v", saved)
	}
	if repo.stored.Config.BaseURL != SiliconFlowVoiceBaseURL ||
		repo.stored.Config.VoiceModel != SiliconFlowVoiceModelID ||
		repo.stored.Config.VoiceID != SiliconFlowVoiceID {
		t.Fatalf("stored Voice tuple = %#v", repo.stored.Config)
	}

	tested, err := service.TestAdminVoiceProviderConnection(context.Background(), "siliconflow")
	if err != nil {
		t.Fatalf("TestAdminVoiceProviderConnection() error = %v", err)
	}
	if tested.Provider.Enabled || !tested.Provider.ConnectionTestValid ||
		tested.ContentType != "audio/mpeg" || tested.Size <= 0 {
		t.Fatalf("tested Voice provider = %#v", tested)
	}
	if _, err := service.ResolveVoiceProvider(context.Background()); !errors.Is(err, ErrVoiceProviderNotConfigured) {
		t.Fatalf("tested-only ResolveVoiceProvider() error = %v", err)
	}

	activated, err := service.ActivateAdminVoiceProvider(context.Background(), "siliconflow")
	if err != nil {
		t.Fatalf("ActivateAdminVoiceProvider() error = %v", err)
	}
	if !activated.Provider.Enabled || !activated.Provider.ConnectionTestValid {
		t.Fatalf("activated Voice provider = %#v", activated)
	}
	resolved, err := service.ResolveVoiceProvider(context.Background())
	if err != nil || resolved.ProviderID != "siliconflow" ||
		resolved.BaseURL != SiliconFlowVoiceBaseURL ||
		resolved.ModelID != SiliconFlowVoiceModelID || resolved.VoiceID != SiliconFlowVoiceID ||
		resolved.APIKey != fixtureCredential {
		t.Fatalf("ResolveVoiceProvider() = %#v, %v", resolved, err)
	}
	public := service.PublicConfigForContext(context.Background())
	if !public.Voice.DefaultTTSAvailable || public.Voice.DefaultSTTAvailable ||
		public.Voice.DefaultProvider != "siliconflow" {
		t.Fatalf("public Voice config = %#v", public.Voice)
	}

	updated, err := service.UpsertAdminVoiceProviderConfig(
		context.Background(),
		"siliconflow",
		UpdateAdminVoiceProviderConfigRequest{
			Enabled: true,
			APIKeySecret: encryptedSecretEnvelope(
				t,
				privateKey,
				"rotated-siliconflow-fixture",
				voiceProviderIngressContext(voiceProviderSiliconFlow),
			),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Enabled || updated.ConnectionTestValid ||
		repo.stored.Config.ConnectionTestSHA256 != "" ||
		repo.stored.Config.ConnectionTestedAt != "" {
		t.Fatalf("credential rotation retained Voice activation: %#v", updated)
	}
}

func TestAdminSiliconFlowVoiceFailedTestDoesNotEnableOrLeakProviderBody(t *testing.T) {
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
		WithProviderSecretVault(testProviderSecretVault(t, "voice-v1", 52)),
		WithVoiceProviderHTTPClient(&http.Client{Transport: voiceRoundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader(`{"secret":"must not escape"}`)),
			}, nil
		})}),
	)
	_, err = service.UpsertAdminVoiceProviderConfig(
		context.Background(),
		"siliconflow",
		UpdateAdminVoiceProviderConfigRequest{APIKeySecret: encryptedSecretEnvelope(
			t,
			privateKey,
			"wrong-siliconflow-fixture",
			voiceProviderIngressContext(voiceProviderSiliconFlow),
		)},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ActivateAdminVoiceProvider(context.Background(), "siliconflow")
	if !errors.Is(err, ErrVoiceProviderConnectionFailed) || strings.Contains(err.Error(), "must not escape") {
		t.Fatalf("ActivateAdminVoiceProvider() error = %v", err)
	}
	if repo.stored.Config.Enabled || repo.stored.Config.ConnectionTestSHA256 != "" {
		t.Fatalf("failed Voice activation mutated config: %#v", repo.stored.Config)
	}
}

func TestVoiceResolverFailsClosedOnInvalidOrAmbiguousAuthority(t *testing.T) {
	invalid := StoredProviderConfig{
		ID: "voice-invalid", UserID: "00000000-0000-0000-0000-000000000001",
		ProviderID: voiceProviderRecordID(voiceProviderSiliconFlow),
		Config: StoredProviderConfigPayload{
			Kind: providerConfigKindVoice, VoiceProvider: string(voiceProviderSiliconFlow),
			BaseURL: SiliconFlowVoiceBaseURL, VoiceModel: "other", VoiceID: SiliconFlowVoiceID,
			Enabled: true,
		},
	}
	service := NewService(config.Config{}, WithProviderConfigRepository(&fakeProviderConfigRepository{
		listed: []StoredProviderConfig{invalid},
	}))
	if _, err := service.ResolveVoiceProvider(context.Background()); !errors.Is(err, ErrVoiceProviderResolutionFailed) {
		t.Fatalf("invalid ResolveVoiceProvider() error = %v", err)
	}

	second := invalid
	second.ID = "voice-second"
	second.ProviderID = voiceProviderRecordID(voiceProviderMimo)
	second.Config.VoiceProvider = string(voiceProviderMimo)
	service = NewService(config.Config{}, WithProviderConfigRepository(&fakeProviderConfigRepository{
		listed: []StoredProviderConfig{invalid, second},
	}))
	if _, err := service.ResolveVoiceProvider(context.Background()); !errors.Is(err, ErrVoiceProviderResolutionFailed) {
		t.Fatalf("ambiguous ResolveVoiceProvider() error = %v", err)
	}
}
