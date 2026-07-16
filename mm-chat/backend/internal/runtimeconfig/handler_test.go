package runtimeconfig

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestHandlerRoutesRuntimeConfig(t *testing.T) {
	handler := NewHandler(NewService(config.Config{Provider: config.ProviderConfig{Model: "gpt-test"}}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response PublicConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := response.ModelProvider.Models; len(got) != 1 || got[0] != "gpt-test" {
		t.Fatalf("models = %#v", got)
	}
}

func TestHandlerRoutesProviderModels(t *testing.T) {
	handler := NewHandler(NewService(config.Config{Provider: config.ProviderConfig{Model: "gpt-test"}}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost,
		"/v1/providers/models",
		strings.NewReader(`{"provider":{"source":"server-default"}}`),
	))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response ProviderModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := response.Models; len(got) != 1 || got[0] != "gpt-test" {
		t.Fatalf("models = %#v", got)
	}
}

func TestHandlerRejectsPlaintextProviderSecret(t *testing.T) {
	handler := NewHandler(NewService(config.Config{}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost,
		"/v1/providers/models",
		strings.NewReader(`{"provider":{"apiKey":"secret"}}`),
	))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PLAINTEXT_PROVIDER_SECRET_REJECTED") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHandlerRoutesBYOKPublicKey(t *testing.T) {
	handler := NewHandler(NewService(config.Config{BYOK: config.BYOKConfig{AllowEphemeralKey: true}}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/byok/public-key", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var response BYOKPublicKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.KID == "" || response.PublicKeyJWK["kty"] != "RSA" {
		t.Fatalf("response = %#v", response)
	}
}

func TestHandlerRoutesAdminProviderConfig(t *testing.T) {
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: StoredProviderConfig{
			UserID:     "00000000-0000-0000-0000-000000000001",
			ProviderID: serverDefaultProviderID,
			Label:      "Admin Default",
			Config: StoredProviderConfigPayload{
				Type:    ProviderTypeOpenAICompatible,
				BaseURL: "https://admin.example/v1",
				Models:  []string{"gpt-admin"},
				Enabled: true,
			},
		},
	}
	handler := NewHandler(NewService(config.Config{}, WithProviderConfigRepository(repo)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/provider-config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response AdminProviderConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Name != "Admin Default" || response.BaseURL != "https://admin.example/v1" {
		t.Fatalf("response = %#v", response)
	}
}

func TestHandlerUpdatesAdminProviderConfig(t *testing.T) {
	handler := NewHandler(NewService(config.Config{}, WithProviderConfigRepository(&fakeProviderConfigRepository{})))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPut,
		"/v1/admin/provider-config",
		strings.NewReader(`{"name":"Saved Default","type":"OpenAI Compatible","baseUrl":"https://saved.example/v1","models":["gpt-saved","gpt-saved"],"enabled":true}`),
	))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var response AdminProviderConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Name != "Saved Default" || len(response.Models) != 1 || response.Models[0] != "gpt-saved" {
		t.Fatalf("response = %#v", response)
	}
}

func TestHandlerAdminProviderConfigRequiresDatabaseForUpdate(t *testing.T) {
	handler := NewHandler(NewService(config.Config{}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPut,
		"/v1/admin/provider-config",
		strings.NewReader(`{"name":"No DB","type":"OpenAI Compatible","models":["gpt"]}`),
	))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "DATABASE_REQUIRED") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
