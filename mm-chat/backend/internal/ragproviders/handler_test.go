package ragproviders

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestProviderStatusReportsMissingSecretsWithoutLeakingValues(t *testing.T) {
	handler := NewHandler(StaticStatusResolver(config.RAGConfig{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/rag/provider-status", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Ready {
		t.Fatal("ready = true, want false without provider secrets")
	}
	if body.Providers.MinerU.Configured || body.Providers.MinerU.Status != ProviderStatusMissingSecret {
		t.Fatalf("mineru status = %#v, want missing secret", body.Providers.MinerU)
	}
	if body.Providers.Jina.Configured || body.Providers.Jina.Status != ProviderStatusMissingSecret {
		t.Fatalf("jina status = %#v, want missing secret", body.Providers.Jina)
	}
	if body.Providers.Jina.EmbeddingDimensions != config.DefaultRAGJinaDimensions {
		t.Fatalf(
			"jina dimensions = %d, want %d",
			body.Providers.Jina.EmbeddingDimensions,
			config.DefaultRAGJinaDimensions,
		)
	}
}

func TestProviderStatusReportsReadyAndRedactsSecrets(t *testing.T) {
	minerUSecret := "fake-mineru-secret"
	jinaSecret := "fake-jina-secret"
	handler := NewHandler(StaticStatusResolver(config.RAGConfig{
		MinerUAPIKey:            minerUSecret,
		JinaAPIKey:              jinaSecret,
		JinaEmbeddingDimensions: config.DefaultRAGJinaDimensions,
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/rag/provider-status", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	payload := rec.Body.String()
	if strings.Contains(payload, minerUSecret) || strings.Contains(payload, jinaSecret) {
		t.Fatalf("provider status leaked secret value: %s", payload)
	}
	var body StatusResponse
	if err := json.NewDecoder(strings.NewReader(payload)).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Ready {
		t.Fatal("ready = false, want true")
	}
	if !body.Providers.MinerU.Configured || body.Providers.MinerU.Status != ProviderStatusReady {
		t.Fatalf("mineru status = %#v, want ready", body.Providers.MinerU)
	}
	if !body.Providers.Jina.Configured || body.Providers.Jina.Status != ProviderStatusReady {
		t.Fatalf("jina status = %#v, want ready", body.Providers.Jina)
	}
	if body.Providers.Jina.EmbeddingDimensions != config.DefaultRAGJinaDimensions {
		t.Fatalf(
			"jina dimensions = %d, want %d",
			body.Providers.Jina.EmbeddingDimensions,
			config.DefaultRAGJinaDimensions,
		)
	}
}

func TestProviderStatusRejectsNonGETWithJSONError(t *testing.T) {
	handler := NewHandler(StaticStatusResolver(config.RAGConfig{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/rag/provider-status", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", got)
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("error code = %q, want METHOD_NOT_ALLOWED", body.Error.Code)
	}
}

func TestProviderStatusReturnsJSONNotFound(t *testing.T) {
	handler := NewHandler(StaticStatusResolver(config.RAGConfig{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/rag/missing", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Fatalf("error code = %q, want NOT_FOUND", body.Error.Code)
	}
}
