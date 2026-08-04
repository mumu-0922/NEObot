package httpserver

import (
	"bytes"
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
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/imagejobs"
	"neo-chat/mm-chat/backend/internal/jobartifacts"
	"neo-chat/mm-chat/backend/internal/jobaudit"
	"neo-chat/mm-chat/backend/internal/memoryjudge"
	"neo-chat/mm-chat/backend/internal/ragproviders"
	"neo-chat/mm-chat/backend/internal/ragsource"
	"neo-chat/mm-chat/backend/internal/ratelimit"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
	"neo-chat/mm-chat/backend/internal/usermemory"
	"neo-chat/mm-chat/backend/internal/voicejobs"
	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestRuntimeProviderResolverAdmitsBuiltInSearchOnlyForOpenAI(t *testing.T) {
	tests := []struct {
		providerType string
		wantBuiltIn  bool
	}{
		{providerType: "OpenAI", wantBuiltIn: true},
		{providerType: "OpenAI Compatible", wantBuiltIn: false},
		{providerType: "Gemini", wantBuiltIn: true},
		{providerType: "Anthropic", wantBuiltIn: true},
	}
	for _, tt := range tests {
		t.Run(tt.providerType, func(t *testing.T) {
			privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}
			service := runtimeconfig.NewService(config.Config{BYOK: config.BYOKConfig{
				PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{
					Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
				})),
			}})
			resolution, err := (runtimeChatProviderResolver{service: service}).ResolveRuntimeProvider(
				context.Background(),
				runtimeconfig.ProviderRuntimeConfig{
					Type: tt.providerType, BaseURL: "https://provider.example",
					APIKeySecret: runtimeProviderSecretEnvelope(
						t, privateKey, "fixture-key", "provider:"+tt.providerType,
					),
				},
			)
			if err != nil {
				t.Fatalf("ResolveRuntimeProvider() error = %v", err)
			}
			_, builtIn := resolution.Provider.(chat.ModelBuiltInSearchProvider)
			if builtIn != tt.wantBuiltIn {
				t.Fatalf("built-in capability = %v, want %v", builtIn, tt.wantBuiltIn)
			}
			if tt.providerType == "Gemini" {
				if _, ok := resolution.Provider.(*chat.GeminiProvider); !ok {
					t.Fatalf("provider = %T, want *chat.GeminiProvider", resolution.Provider)
				}
			}
			if tt.providerType == "Anthropic" {
				if _, ok := resolution.Provider.(*chat.AnthropicProvider); !ok {
					t.Fatalf("provider = %T, want *chat.AnthropicProvider", resolution.Provider)
				}
			}
		})
	}
}

type memoryHybridProfileFactoryFixture struct {
	gateway   *ragproviders.RetrievalProfileGateway
	requested ragproviders.RetrievalProfileID
}

func (fixture *memoryHybridProfileFactoryFixture) EmbedQuery(
	context.Context,
	string,
) (ragproviders.QueryEmbedding, error) {
	return ragproviders.QueryEmbedding{}, ragproviders.ErrQueryEmbeddingUnavailable
}

func (fixture *memoryHybridProfileFactoryFixture) ForRetrievalProfile(
	profileID ragproviders.RetrievalProfileID,
) (*ragproviders.RetrievalProfileGateway, error) {
	fixture.requested = profileID
	return fixture.gateway, nil
}

func TestResolveMemoryHybridProviderRejectsArbitraryEmbedder(t *testing.T) {
	if provider := resolveMemoryHybridProvider(fakeRAGQueryEmbedder{}); provider != nil {
		t.Fatalf("provider = %T, want nil", provider)
	}
}

func TestResolveMemoryHybridProviderBindsProviderGatewayToSiliconFlow(t *testing.T) {
	provider := resolveMemoryHybridProvider(ragproviders.NewProviderGateway(nil))
	profileGateway, ok := provider.(*ragproviders.RetrievalProfileGateway)
	if !ok || profileGateway.Profile().ID != ragproviders.RetrievalProfileSiliconFlow {
		t.Fatalf("provider = %#v, want SiliconFlow retrieval profile gateway", provider)
	}
}

func TestResolveMemoryHybridProviderRejectsWrongFactoryProfile(t *testing.T) {
	factory := &memoryHybridProfileFactoryFixture{
		gateway: &ragproviders.RetrievalProfileGateway{},
	}
	if provider := resolveMemoryHybridProvider(factory); provider != nil {
		t.Fatalf("provider = %T, want nil", provider)
	}
	if factory.requested != ragproviders.RetrievalProfileSiliconFlow {
		t.Fatalf("requested profile = %q, want %q",
			factory.requested, ragproviders.RetrievalProfileSiliconFlow)
	}
}

func TestMemoryHybridProviderRequiredForEveryProviderBackedReader(t *testing.T) {
	tests := []struct {
		name   string
		memory config.MemoryConfig
		want   bool
	}{
		{name: "all disabled"},
		{
			name: "L1 hybrid shadow",
			memory: config.MemoryConfig{
				HybridShadowEnabled: true,
			},
			want: true,
		},
		{
			name: "L1 Memory Tool Loop",
			memory: config.MemoryConfig{
				ToolLoopEnabled: true,
			},
			want: true,
		},
		{
			name: "L2 Scene shadow",
			memory: config.MemoryConfig{
				L2SceneShadowEnabled: true,
			},
			want: true,
		},
		{
			name: "L3 Persona shadow",
			memory: config.MemoryConfig{
				L3PersonaShadowEnabled: true,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := memoryHybridProviderRequired(tt.memory); got != tt.want {
				t.Fatalf("memoryHybridProviderRequired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFixedMemoryJudgeAuthorityPinsProductionTuple(t *testing.T) {
	valid := runtimeconfig.ResolvedProvider{
		ID:      fixedMemoryJudgeProviderID,
		Type:    runtimeconfig.ProviderTypeOpenAICompatible,
		BaseURL: "https://sub.mumubuku.top/v1/",
		APIKey:  "example-fixture-provider-credential",
		Models:  []string{"gpt-5.6-sol", usermemory.HybridFixedMemoryJudgeModelID},
	}
	if !fixedMemoryJudgeAuthorityValid(valid) {
		t.Fatal("exact fixed Memory judge authority was rejected")
	}
	mutations := []struct {
		name   string
		mutate func(*runtimeconfig.ResolvedProvider)
	}{
		{name: "provider", mutate: func(value *runtimeconfig.ResolvedProvider) { value.ID = "CUSTOM" }},
		{name: "type", mutate: func(value *runtimeconfig.ResolvedProvider) { value.Type = runtimeconfig.ProviderTypeOpenAI }},
		{name: "endpoint", mutate: func(value *runtimeconfig.ResolvedProvider) { value.BaseURL = "https://other.example/v1" }},
		{name: "secret", mutate: func(value *runtimeconfig.ResolvedProvider) { value.APIKey = "" }},
		{name: "model", mutate: func(value *runtimeconfig.ResolvedProvider) { value.Models = []string{"gpt-5.6-sol"} }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			candidate := valid
			candidate.Models = append([]string(nil), valid.Models...)
			mutation.mutate(&candidate)
			if fixedMemoryJudgeAuthorityValid(candidate) {
				t.Fatalf("drifted authority accepted: %#v", candidate)
			}
		})
	}
}

func TestRuntimeMemoryCandidateJudgeFailsClosedOnAuthorityDrift(t *testing.T) {
	judge := newRuntimeMemoryCandidateJudge(
		fixedMemoryJudgeResolverFixture{provider: runtimeconfig.ResolvedProvider{
			ID:      fixedMemoryJudgeProviderID,
			Type:    runtimeconfig.ProviderTypeOpenAICompatible,
			BaseURL: "https://other.example/v1",
			APIKey:  "example-fixture-provider-credential",
			Models:  []string{usermemory.HybridFixedMemoryJudgeModelID},
		}},
		time.Second,
	)
	if judge == nil {
		t.Fatal("runtime fixed Memory judge was not constructed")
	}
	_, err := judge.JudgeHybridCandidates(context.Background(), usermemory.HybridCandidateJudgeInput{
		Query: "Which school?",
		Candidates: []usermemory.HybridCandidateJudgeCandidate{
			{Ordinal: 0, Content: "Northwestern Polytechnical University"},
		},
	})
	if category := memoryjudge.FailureCategory(err); category != memoryjudge.FailureProvenanceDrift {
		t.Fatalf("failure category=%q err=%v", category, err)
	}
}

type fixedMemoryJudgeResolverFixture struct {
	provider runtimeconfig.ResolvedProvider
	err      error
}

func (fixture fixedMemoryJudgeResolverFixture) ResolveServerDefaultProvider(
	context.Context,
) (runtimeconfig.ResolvedProvider, error) {
	return fixture.provider, fixture.err
}

func TestRuntimeChatProviderResolutionUsesServerOwnedAnswerProcessor(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		config    runtimeconfig.ProviderRuntimeConfig
		processor string
	}{
		{
			name:      "server default type",
			source:    "server-default",
			config:    runtimeconfig.ProviderRuntimeConfig{ID: "SERVER_DEFAULT", Type: "OpenAI Compatible"},
			processor: "openai_compatible",
		},
		{
			name:      "stored provider id",
			source:    "server-stored",
			config:    runtimeconfig.ProviderRuntimeConfig{ID: "FOHWSU", Type: "OpenAI Compatible"},
			processor: "fohwsu",
		},
		{
			name:      "browser supplied provider has no server consent identity",
			source:    "",
			config:    runtimeconfig.ProviderRuntimeConfig{ID: "CUSTOM", Type: "OpenAI Compatible"},
			processor: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolution := runtimeChatProviderResolution(nil, tt.source, tt.config)
			if resolution.RAGAnswerProcessor != tt.processor {
				t.Fatalf("processor = %q, want %q", resolution.RAGAnswerProcessor, tt.processor)
			}
		})
	}
}

func runtimeProviderSecretEnvelope(
	t *testing.T,
	privateKey *rsa.PrivateKey,
	plaintext string,
	secretContext string,
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
	ciphertext := gcm.Seal(nil, iv, []byte(plaintext), []byte(secretContext))
	wrappedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &privateKey.PublicKey, aesKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	kid := sha256.Sum256(der)
	return map[string]any{
		"v": 1, "kid": base64.RawURLEncoding.EncodeToString(kid[:]),
		"alg": "RSA-OAEP-256+A256GCM", "context": secretContext,
		"iv":         base64.RawURLEncoding.EncodeToString(iv),
		"wrappedKey": base64.RawURLEncoding.EncodeToString(wrappedKey),
		"ciphertext": base64.RawURLEncoding.EncodeToString(ciphertext),
	}
}

func TestMapRuntimeProviderErrorRedactsVaultFailures(t *testing.T) {
	for _, source := range []error{
		runtimeconfig.ErrProviderSecretVaultUnavailable,
		runtimeconfig.ErrProviderSecretInvalid,
	} {
		mapped, ok := mapRuntimeProviderError(source).(chat.ValidationError)
		if !ok || mapped.Code != "PROVIDER_SECRET_UNAVAILABLE" ||
			mapped.Message != "stored provider secret is unavailable" {
			t.Fatalf("mapRuntimeProviderError(%v) = %#v", source, mapped)
		}
	}
}

func TestNewHandlerRoutesHealthReadyAndVersion(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})

	tests := []struct {
		name string
		path string
		want map[string]string
	}{
		{name: "health", path: "/health", want: map[string]string{"status": "healthy"}},
		{name: "ready", path: "/ready", want: map[string]string{"status": "ready"}},
		{name: "version", path: "/v1/version", want: map[string]string{"version": "route-test"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", contentType)
			}

			var got map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode response body: %v", err)
			}
			for key, value := range tt.want {
				if got[key] != value {
					t.Fatalf("body[%q] = %q, want %q; body=%v", key, got[key], value, got)
				}
			}
		})
	}
}

func TestNewHandlerRegistersMemoryPortabilityRoutes(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	tests := []struct {
		name        string
		path        string
		body        string
		contentType string
		wantStatus  int
		wantCode    string
	}{
		{
			name: "export", path: "/v1/memory-export",
			body:        `{"passphrase":"fixture-passphrase","includeHistory":false}`,
			contentType: "application/json", wantStatus: http.StatusServiceUnavailable,
			wantCode: "DATABASE_REQUIRED",
		},
		{
			name: "import dry-run", path: "/v1/memory-import/dry-run",
			contentType: "application/json", wantStatus: http.StatusBadRequest,
			wantCode: "MEMORY_IMPORT_MULTIPART_INVALID",
		},
		{
			name: "import confirm", path: "/v1/memory-import/confirm",
			contentType: "application/json", wantStatus: http.StatusBadRequest,
			wantCode: "MEMORY_IMPORT_MULTIPART_INVALID",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			handler.ServeHTTP(response, request)

			var payload ErrorResponse
			if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
				t.Fatalf("decode response: %v body=%s", err, response.Body.String())
			}
			if response.Code != test.wantStatus || payload.Error.Code != test.wantCode {
				t.Fatalf(
					"response = %d/%s, want %d/%s",
					response.Code, payload.Error.Code, test.wantStatus, test.wantCode,
				)
			}
		})
	}
}

func TestNewHandlerRoutesAuthenticatedWebSearchBoundary(t *testing.T) {
	provider := &httpserverSearchProvider{}
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "route-test"},
		WithWebSearchResolver(httpserverSearchResolver{provider: provider}),
	)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		websearch.SearchPath,
		strings.NewReader(`{"query":"fixture","maxResults":2}`),
	)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if provider.calls != 1 || !strings.Contains(recorder.Body.String(), "https://search.example/result") {
		t.Fatalf("provider calls/body = %d/%s", provider.calls, recorder.Body.String())
	}
}

type httpserverSearchResolver struct {
	provider websearch.Provider
}

func (r httpserverSearchResolver) ResolveActive(context.Context) (websearch.ActiveExecution, error) {
	return websearch.ActiveExecution{
		Mode: websearch.ExecutionExternal, External: r.provider,
	}, nil
}

type httpserverSearchProvider struct {
	calls int
}

func (p *httpserverSearchProvider) ID() websearch.ProviderID { return websearch.ProviderTavily }

func (p *httpserverSearchProvider) Search(
	_ context.Context,
	_ websearch.Request,
) (websearch.Result, error) {
	p.calls++
	return websearch.Result{Sources: []websearch.Source{{
		Title: "Fixture", URL: "https://search.example/result", Content: "result",
	}}}, nil
}

func TestMiddlewareSetsSecurityHeaders(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestMiddlewareSetsAndPropagatesRequestID(t *testing.T) {
	var contextRequestID string
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextRequestID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}), withRequestID)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/request-id", nil)
	req.Header.Set(requestIDHeader, "client-request-1")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := rec.Header().Get(requestIDHeader); got != "client-request-1" {
		t.Fatalf("%s = %q, want client-request-1", requestIDHeader, got)
	}
	if contextRequestID != "client-request-1" {
		t.Fatalf("context request id = %q, want client-request-1", contextRequestID)
	}
}

func TestMiddlewareGeneratesRequestIDWhenMissingOrInvalid(t *testing.T) {
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Fatal("request id missing from context")
		}
		w.WriteHeader(http.StatusNoContent)
	}), withRequestID)
	for _, headerValue := range []string{"", "bad request id"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/request-id", nil)
		req.Header.Set(requestIDHeader, headerValue)

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
		if got := rec.Header().Get(requestIDHeader); got == "" || got == headerValue {
			t.Fatalf("%s = %q, want generated value", requestIDHeader, got)
		}
	}
}

func TestMiddlewareLogsStructuredRequest(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	}), withRequestID, withRequestLogging(logger))
	rec := httptest.NewRecorder()
	teamID := "33333333-3333-4333-8333-333333333333"
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/teams/"+teamID+"/invites?token=hidden",
		nil,
	)
	req.Header.Set(requestIDHeader, "log-request-1")
	req.RemoteAddr = "127.0.0.1:12345"

	handler.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry); err != nil {
		t.Fatalf("decode structured log: %v; log=%s", err, logs.String())
	}
	if entry["msg"] != "http_request" ||
		entry["request_id"] != "log-request-1" ||
		entry["method"] != http.MethodPost ||
		entry["path"] != "/v1/teams/{teamId}/invites" {
		t.Fatalf("structured log = %#v", entry)
	}
	if entry["status"] != float64(http.StatusCreated) {
		t.Fatalf("log status = %#v, want %d", entry["status"], http.StatusCreated)
	}
	if strings.Contains(logs.String(), "token=hidden") || strings.Contains(logs.String(), teamID) {
		t.Fatalf("structured log includes query or raw Team id: %s", logs.String())
	}
}

func TestRequestLoggingPreservesFlusher(t *testing.T) {
	flushAvailable := false
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		flushAvailable = ok
		if ok {
			flusher.Flush()
		}
		w.WriteHeader(http.StatusNoContent)
	}), withRequestID, withRequestLogging(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)

	handler.ServeHTTP(rec, req)

	if !flushAvailable {
		t.Fatal("logging response writer does not preserve http.Flusher")
	}
}

func TestMiddlewareRecoversPanicsWithJSON(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom-secret")
	}), withRequestID, withRecover(logger), withSecurityHeaders)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	req.Header.Set(requestIDHeader, "panic-request-1")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "INTERNAL_ERROR")
	}
	if !strings.Contains(logs.String(), "panic-request-1") {
		t.Fatalf("panic log missing request id: %s", logs.String())
	}
	if strings.Contains(logs.String(), "boom-secret") {
		t.Fatalf("panic log leaks panic payload: %s", logs.String())
	}
}

func TestMiddlewareChainLogsPanicAndRequestWithRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("chain-secret")
	}), withRequestID, withRequestLogging(logger), withRecover(logger), withSecurityHeaders)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic-chain?token=hidden", nil)
	req.Header.Set(requestIDHeader, "chain-request-1")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if got := rec.Header().Get(requestIDHeader); got != "chain-request-1" {
		t.Fatalf("%s = %q, want chain-request-1", requestIDHeader, got)
	}

	entries := decodeJSONLogLines(t, logs.Bytes())
	if len(entries) != 2 {
		t.Fatalf("log entries = %#v, want panic and request entries; raw=%s", entries, logs.String())
	}
	if entries[0]["msg"] != "http_panic" || entries[0]["request_id"] != "chain-request-1" {
		t.Fatalf("panic log = %#v", entries[0])
	}
	if entries[1]["msg"] != "http_request" ||
		entries[1]["request_id"] != "chain-request-1" ||
		entries[1]["status"] != float64(http.StatusInternalServerError) ||
		entries[1]["path"] != "/__unknown__" {
		t.Fatalf("request log = %#v", entries[1])
	}
	if strings.Contains(logs.String(), "chain-secret") || strings.Contains(logs.String(), "token=hidden") {
		t.Fatalf("chain logs leak secret payload or query: %s", logs.String())
	}
}

func TestSessionIdentityMiddlewareSetsRequestUser(t *testing.T) {
	resolver := &fakeSessionResolver{
		session: auth.Session{
			ID:          "session-1",
			UserID:      "77777777-7777-4777-8777-777777777777",
			DisplayName: "User Seven",
			Role:        "owner",
			ExpiresAt:   time.Now().Add(time.Hour),
		},
	}
	var gotUser auth.User
	var gotSession auth.Session
	var gotSessionOK bool
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = auth.UserOrDevelopment(r.Context())
		gotSession, gotSessionOK = auth.SessionFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}), withSessionIdentity(resolver, nil, true))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer raw-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if resolver.tokenHash != auth.HashSessionToken("raw-token") {
		t.Fatalf("token hash = %q, want %q", resolver.tokenHash, auth.HashSessionToken("raw-token"))
	}
	if gotUser.ID != resolver.session.UserID || gotUser.DisplayName != "User Seven" || gotUser.Role != "owner" {
		t.Fatalf("context user = %#v", gotUser)
	}
	if !gotSessionOK || gotSession.ID != resolver.session.ID || gotSession.UserID != resolver.session.UserID {
		t.Fatalf("context session = %#v, ok=%v", gotSession, gotSessionOK)
	}
}

func TestSessionIdentityMiddlewareRejectsInvalidSession(t *testing.T) {
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run for invalid bearer token")
	}), withSessionIdentity(&fakeSessionResolver{err: auth.ErrSessionExpired}, nil, true))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer expired-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "UNAUTHENTICATED" {
		t.Fatalf("error code = %q, want UNAUTHENTICATED", body.Error.Code)
	}
}

func TestSessionIdentityMiddlewareKeepsDevelopmentFallbackWhenMissingBearer(t *testing.T) {
	resolver := &fakeSessionResolver{err: auth.ErrSessionExpired}
	nextCalled := false
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		user := auth.UserOrDevelopment(r.Context())
		if user.ID != auth.DevelopmentUserID {
			t.Fatalf("user = %#v, want development fallback", user)
		}
		w.WriteHeader(http.StatusNoContent)
	}), withSessionIdentity(resolver, nil, false))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/private", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if !nextCalled {
		t.Fatal("next handler was not called")
	}
	if resolver.tokenHash != "" {
		t.Fatalf("resolver tokenHash = %q, want blank when bearer is missing", resolver.tokenHash)
	}
}

func TestSessionIdentityMiddlewareKeepsDevelopmentOwnerWhenBearerIsStale(t *testing.T) {
	resolver := &fakeSessionResolver{err: auth.ErrSessionExpired}
	developmentSession := auth.Session{
		ID:          auth.DevelopmentSessionID,
		UserID:      auth.DevelopmentUserID,
		DisplayName: auth.DevelopmentDisplayName,
		Role:        "user",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	handler := chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserOrDevelopment(r.Context())
		if user.ID != auth.DevelopmentUserID {
			t.Fatalf("user = %#v, want fixed development owner", user)
		}
		contextUser, ok := auth.UserFromContext(r.Context())
		if !ok || contextUser.ID != auth.DevelopmentUserID {
			t.Fatalf("context user = %#v, ok=%v; want explicit development owner", contextUser, ok)
		}
		session, ok := auth.SessionFromContext(r.Context())
		if !ok || session.ID != auth.DevelopmentSessionID || session.UserID != auth.DevelopmentUserID {
			t.Fatalf("development session = %#v, ok=%v", session, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}), withSessionIdentity(resolver, &developmentSession, false))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/knowledge/collections", nil)
	request.Header.Set("Authorization", "Bearer stale-browser-session")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNoContent, recorder.Body.String())
	}
	if resolver.tokenHash != "" {
		t.Fatalf("resolver tokenHash = %q, want unused in development mode", resolver.tokenHash)
	}
}

func TestSessionIdentityMiddlewareSkipsLoginRoute(t *testing.T) {
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "route-test"},
		WithSessionResolver(&fakeSessionResolver{err: auth.ErrSessionExpired}),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/auth/login",
		strings.NewReader(`{"email":"owner@example.test","password":"not-the-user-password"}`),
	)
	req.Header.Set("Authorization", "Bearer expired-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("error code = %q, want DATABASE_REQUIRED", body.Error.Code)
	}
}

func TestAuthRequiredModeRejectsMissingCredentialsAndKeepsPublicRoutes(t *testing.T) {
	handler := NewHandler(config.Config{
		Addr:    ":0",
		Version: "route-test",
		Auth:    config.AuthConfig{Mode: config.AuthModeRequired},
	})

	publicRoutes := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{method: http.MethodGet, path: "/health", want: http.StatusOK},
		{method: http.MethodGet, path: "/ready", want: http.StatusOK},
		{method: http.MethodGet, path: "/metrics", want: http.StatusOK},
		{method: http.MethodGet, path: "/v1/version", want: http.StatusOK},
		{method: http.MethodGet, path: "/v1/config", want: http.StatusOK},
		{method: http.MethodGet, path: "/v1/byok/public-key", want: http.StatusServiceUnavailable},
		{
			method: http.MethodPost,
			path:   "/v1/auth/login",
			body:   `{"email":"owner@example.test","password":"not-the-user-password"}`,
			want:   http.StatusServiceUnavailable,
		},
		{
			method: http.MethodPost,
			path:   "/v1/auth/invites/accept",
			body:   `{"token":"token","password":"invite-password-value"}`,
			want:   http.StatusServiceUnavailable,
		},
		{
			method: http.MethodPost,
			path:   "/v1/auth/recovery/request",
			body:   `{"email":"owner@example.test"}`,
			want:   http.StatusServiceUnavailable,
		},
		{
			method: http.MethodPost,
			path:   "/v1/auth/recovery/complete",
			body:   `{"token":"token","newPassword":"replacement-password-value"}`,
			want:   http.StatusServiceUnavailable,
		},
	}
	for _, route := range publicRoutes {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(
			route.method,
			route.path,
			strings.NewReader(route.body),
		)

		handler.ServeHTTP(rec, req)

		if rec.Code != route.want {
			t.Fatalf("%s %s status = %d, want %d; body=%s", route.method, route.path, rec.Code, route.want, rec.Body.String())
		}
	}

	protectedRoutes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/v1/me"},
		{method: http.MethodDelete, path: "/v1/me/sessions"},
		{method: http.MethodPost, path: "/v1/auth/logout"},
		{method: http.MethodPost, path: "/v1/providers/models"},
		{method: http.MethodGet, path: "/v1/admin/search/providers"},
		{method: http.MethodPost, path: "/v1/admin/search/providers/tavily/test"},
		{method: http.MethodGet, path: "/v1/admin/voice/providers"},
		{method: http.MethodPost, path: "/v1/admin/voice/providers/siliconflow/test"},
		{method: http.MethodGet, path: "/v1/admin/rag/providers"},
		{method: http.MethodPost, path: "/v1/admin/rag/providers/jina/configure"},
		{method: http.MethodGet, path: "/v1/admin/task-models"},
		{method: http.MethodPost, path: "/v1/search"},
		{method: http.MethodGet, path: "/v1/chat/conversations"},
		{method: http.MethodPost, path: "/v1/chat/generate"},
		{method: http.MethodGet, path: "/v1/files/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodGet, path: "/v1/import/browser/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodPost, path: "/v1/images/generations"},
		{method: http.MethodPost, path: "/v1/code/executions"},
		{method: http.MethodPost, path: "/v1/voice/synthesize"},
		{method: http.MethodPost, path: "/v1/jobs/job-1/cancel"},
		{method: http.MethodGet, path: "/v1/rag/provider-status"},
		{method: http.MethodGet, path: "/v1/teams"},
		{method: http.MethodGet, path: "/v1/teams/33333333-3333-4333-8333-333333333333/members"},
		{method: http.MethodGet, path: "/v1/knowledge/collections"},
		{method: http.MethodPost, path: "/v1/knowledge/collections"},
		{method: http.MethodGet, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodPatch, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodDelete, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodGet, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/documents"},
		{method: http.MethodPost, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/documents"},
		{method: http.MethodGet, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/processing-consents"},
		{method: http.MethodPut, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/processing-consents/mineru"},
		{method: http.MethodDelete, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/processing-consents/mineru"},
		{method: http.MethodGet, path: "/v1/me/knowledge/query-consents"},
		{method: http.MethodPut, path: "/v1/me/knowledge/query-consents/mineru"},
		{method: http.MethodDelete, path: "/v1/me/knowledge/query-consents/mineru"},
		{method: http.MethodGet, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodDelete, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodGet, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333/content"},
		{method: http.MethodPost, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333/versions"},
		{method: http.MethodPost, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333/reprocess"},
	}
	for _, route := range protectedRoutes {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(route.method, route.path, nil)

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401; body=%s", route.method, route.path, rec.Code, rec.Body.String())
		}
		var body ErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s %s response: %v", route.method, route.path, err)
		}
		if body.Error.Code != "UNAUTHENTICATED" {
			t.Fatalf("%s %s error code = %q, want UNAUTHENTICATED", route.method, route.path, body.Error.Code)
		}
	}
}

func TestAuthRequiredModeLetsInternalRAGSourceRouteUseItsOwnTokenGate(t *testing.T) {
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Auth:    config.AuthConfig{Mode: config.AuthModeRequired},
		},
		WithRAGSourceService(ragsource.NewService(
			nil,
			nil,
			ragsource.WithInternalToken("unit-test-rag-source-token"),
		)),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		ragsource.InternalSourceObjectPath,
		strings.NewReader(`{}`),
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 from ragsource token gate; body=%s", rec.Code, rec.Body.String())
	}
	var body ragsource.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode ragsource error response: %v", err)
	}
	if body.Error.Code != "RAG_SOURCE_OBJECT_UNAUTHORIZED" {
		t.Fatalf("error code = %q, want RAG_SOURCE_OBJECT_UNAUTHORIZED", body.Error.Code)
	}
}

func TestAuthRequiredModeLetsInternalProviderRoutesUseTheirOwnTokenGate(t *testing.T) {
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Auth:    config.AuthConfig{Mode: config.AuthModeRequired},
			RAG: config.RAGConfig{
				SourceGatewayToken: "unit-test-rag-provider-token",
			},
		},
	)
	for _, path := range []string{
		ragproviders.InternalMinerUAllocatePath,
		ragproviders.InternalMinerUPollPath,
		ragproviders.InternalSiliconFlowEmbeddingsPath,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")

		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401; body=%s", path, recorder.Code, recorder.Body.String())
		}
		var body ragproviders.ErrorResponse
		if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s response: %v", path, err)
		}
		if body.Error.Code != "RAG_PROVIDER_GATEWAY_UNAUTHORIZED" {
			t.Fatalf("%s error code = %q", path, body.Error.Code)
		}
	}
}

func TestNewHandlerRegistersRAGProviderStatusRoute(t *testing.T) {
	resolver := &fakeSessionResolver{session: auth.Session{
		ID:          "session-1",
		UserID:      "77777777-7777-4777-8777-777777777777",
		DisplayName: "RAG Admin",
		Role:        "owner",
		ExpiresAt:   time.Now().Add(time.Hour),
	}}
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
		},
		WithSessionResolver(resolver),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/rag/provider-status", nil)
	req.Header.Set("Authorization", "Bearer rag-provider-status-session")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if resolver.tokenHash != "" {
		t.Fatalf("development provider status unexpectedly resolved bearer hash = %q", resolver.tokenHash)
	}
}

func TestNewHandlerRegistersTeamRoutes(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	for _, path := range []string{
		"/v1/teams",
		"/v1/teams/33333333-3333-4333-8333-333333333333",
		"/v1/teams/33333333-3333-4333-8333-333333333333/members",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("GET %s was blocked by authentication in development mode; body=%s", path, rec.Body.String())
		}
	}
}

func TestNewHandlerRegistersKnowledgeCollectionRoutes(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/v1/knowledge/collections"},
		{method: http.MethodPost, path: "/v1/knowledge/collections"},
		{method: http.MethodGet, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodPatch, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodDelete, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodGet, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/documents"},
		{method: http.MethodPost, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/documents"},
		{method: http.MethodGet, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/processing-consents"},
		{method: http.MethodPut, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/processing-consents/mineru"},
		{method: http.MethodDelete, path: "/v1/knowledge/collections/33333333-3333-4333-8333-333333333333/processing-consents/mineru"},
		{method: http.MethodGet, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodDelete, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333"},
		{method: http.MethodGet, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333/content"},
		{method: http.MethodPost, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333/versions"},
		{method: http.MethodPost, path: "/v1/knowledge/documents/33333333-3333-4333-8333-333333333333/reprocess"},
		{method: http.MethodGet, path: "/v1/me/knowledge/query-consents"},
		{method: http.MethodPut, path: "/v1/me/knowledge/query-consents/mineru"},
		{method: http.MethodDelete, path: "/v1/me/knowledge/query-consents/mineru"},
	}
	for _, route := range routes {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(route.method, route.path, nil)
		handler.ServeHTTP(recorder, request)
		if recorder.Code == http.StatusUnauthorized {
			t.Fatalf("%s %s was blocked by authentication in development mode; body=%s", route.method, route.path, recorder.Body.String())
		}
	}
}

func TestDevelopmentModeTeamRoutesAlwaysUseFixedOwner(t *testing.T) {
	resolver := &fakeSessionResolver{session: auth.Session{
		ID:          "session-1",
		UserID:      "77777777-7777-4777-8777-777777777777",
		DisplayName: "Team User",
		Role:        "user",
		ExpiresAt:   time.Now().Add(time.Hour),
	}}
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "route-test"},
		WithSessionResolver(resolver),
	)

	withoutBearer := httptest.NewRecorder()
	handler.ServeHTTP(
		withoutBearer,
		httptest.NewRequest(http.MethodGet, "/v1/teams", nil),
	)
	if withoutBearer.Code == http.StatusUnauthorized {
		t.Fatalf("development Team route without bearer was rejected; body=%s", withoutBearer.Body.String())
	}

	withBearer := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/teams", nil)
	request.Header.Set("Authorization", "Bearer development-team-session")
	handler.ServeHTTP(withBearer, request)
	if withBearer.Code != http.StatusServiceUnavailable {
		t.Fatalf("development Team route with bearer status = %d; body=%s", withBearer.Code, withBearer.Body.String())
	}
	if resolver.tokenHash != "" {
		t.Fatalf("development Team route resolved bearer instead of using fixed owner: %q", resolver.tokenHash)
	}
}

func TestAuthRequiredModeFailsClosedWhenResolverIsMissing(t *testing.T) {
	handler := NewHandler(config.Config{
		Addr:    ":0",
		Version: "route-test",
		Auth:    config.AuthConfig{Mode: config.AuthModeRequired},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Header.Set("Authorization", "Bearer raw-session-token")

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("error code = %q, want DATABASE_REQUIRED", body.Error.Code)
	}
}

func TestNewHandlerRejectsNonGETWithJSONError(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/health", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "METHOD_NOT_ALLOWED")
	}
}

func TestNewHandlerReturnsJSONNotFound(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "NOT_FOUND")
	}
}

func TestNewHandlerRegistersChatRoutesWithDatabaseRequired(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/conversations", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/runs/33333333-3333-4333-8333-333333333333/cancel",
		nil,
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("cancel status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode cancel error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("cancel error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/generate",
		strings.NewReader(`{"prompt":"polish","modelRef":{"providerId":"mock","modelId":"model"}}`),
	)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("generate status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode generate error response: %v", err)
	}
	if body.Error.Code != "PROVIDER_REQUIRED" {
		t.Fatalf("generate error code = %q, want %q", body.Error.Code, "PROVIDER_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/tools/plan",
		strings.NewReader(`{}`),
	)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("tool plan status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode tool plan error response: %v", err)
	}
	if body.Error.Code != "PROVIDER_REQUIRED" {
		t.Fatalf("tool plan error code = %q, want %q", body.Error.Code, "PROVIDER_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/files/33333333-3333-4333-8333-333333333333",
		nil,
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("file status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode file error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("file error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodGet,
		"/v1/import/browser/33333333-3333-4333-8333-333333333333",
		nil,
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("import status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode import error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("import error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(
		http.MethodPost,
		"/v1/auth/login",
		strings.NewReader(`{"email":"owner@example.test","password":"not-the-user-password"}`),
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("auth login status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode auth login error response: %v", err)
	}
	if body.Error.Code != "DATABASE_REQUIRED" {
		t.Fatalf("auth login error code = %q, want %q", body.Error.Code, "DATABASE_REQUIRED")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/me", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestNewHandlerRegistersPluginRoutesWithFailClosedRegistryFallbacks(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/plugins", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("plugin list status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var listBody struct {
		Plugins     []any `json:"plugins"`
		Unavailable bool  `json:"unavailable"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode plugin list: %v", err)
	}
	if listBody.Unavailable || len(listBody.Plugins) == 0 {
		t.Fatalf("plugin list = %#v, unavailable=%v; want available built-ins", listBody.Plugins, listBody.Unavailable)
	}

	for _, tc := range []struct {
		path   string
		body   string
		status int
		code   string
	}{
		{path: "/v1/plugins/install", body: `{"customInput":"not-json"}`, status: http.StatusBadRequest, code: "PLUGIN_MANIFEST_INVALID"},
		{path: "/v1/plugins/execute", body: `{"pluginId":"missing","functionName":"lookup","args":{"secret":"sk_live_secret"}}`, status: http.StatusNotFound, code: "PLUGIN_NOT_REGISTERED"},
	} {
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s status = %d, want %d; body=%s", tc.path, rec.Code, tc.status, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "sk_live_secret") {
			t.Fatalf("%s response leaked request secret: %s", tc.path, rec.Body.String())
		}
		var body ErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode %s error response: %v", tc.path, err)
		}
		if body.Error.Code != tc.code {
			t.Fatalf("%s code = %q, want %q", tc.path, body.Error.Code, tc.code)
		}
	}
}

func TestNewHandlerRegistersJobCancelRouteAsFailClosedAdmission(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/cancel", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("job cancel status = %d, want %d; body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode job cancel error response: %v", err)
	}
	if body.Error.Code != "JOB_CANCELLATION_UNAVAILABLE" {
		t.Fatalf("job cancel code = %q, want JOB_CANCELLATION_UNAVAILABLE", body.Error.Code)
	}
}

func TestNewHandlerRegistersCodeJobRouteAsFailClosedAdmission(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/code/executions",
		strings.NewReader(`{"modelRef":{"providerId":"gemini","modelId":"gemini-code"},"code":"print('hi')"}`),
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("code job status = %d, want %d; body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode code job error response: %v", err)
	}
	if body.Error.Code != "CODE_EXECUTION_UNAVAILABLE" {
		t.Fatalf("code job code = %q, want CODE_EXECUTION_UNAVAILABLE", body.Error.Code)
	}
}

func TestNewHandlerRegistersImageJobRouteAsFailClosedAdmission(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/images/generations",
		strings.NewReader(`{"modelRef":{"providerId":"openai","modelId":"gpt-image-1"},"prompt":"paint"}`),
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("image job status = %d, want %d; body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode image job error response: %v", err)
	}
	if body.Error.Code != "IMAGE_JOBS_UNAVAILABLE" {
		t.Fatalf("image job code = %q, want IMAGE_JOBS_UNAVAILABLE", body.Error.Code)
	}
}

func TestNewHandlerRoutesConfiguredImageJobService(t *testing.T) {
	executor := &fakeHTTPImageExecutor{result: imagejobs.GenerateResult{
		Images: []imagejobs.GeneratedImageResult{{
			JobID:       "job-1",
			Filename:    "generated.png",
			ContentType: "image/png",
			Size:        5,
			Body:        strings.NewReader("image"),
		}},
		Message: "stored",
	}}
	store := &fakeHTTPArtifactStore{artifact: jobartifacts.Artifact{
		FileID:      "33333333-3333-4333-8333-333333333333",
		Purpose:     "image",
		ContentType: "image/png",
		Size:        5,
	}}
	var auditEvent jobaudit.Event
	service := imagejobs.NewService(
		imagejobs.WithExecutor(executor),
		imagejobs.WithArtifactStore(store),
		imagejobs.WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
			auditEvent = event
			return nil
		})),
	)
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "route-test"},
		WithImageJobService(service),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/images/generations",
		strings.NewReader(`{"modelRef":{"providerId":"openai","modelId":"gpt-image-2"},"prompt":"private prompt","count":1}`),
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("image job status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response imagejobs.GenerateResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if response.Message != "stored" ||
		len(response.Images) != 1 ||
		response.Images[0].FileID != "33333333-3333-4333-8333-333333333333" ||
		response.Images[0].Purpose != "image" ||
		response.Images[0].ContentType != "image/png" ||
		response.Images[0].Size != 5 {
		t.Fatalf("image response = %#v", response)
	}
	if !executor.called || executor.request.Prompt != "private prompt" || executor.request.ModelRef.ModelID != "gpt-image-2" {
		t.Fatalf("executor state = called:%v request:%#v", executor.called, executor.request)
	}
	if len(store.inputs) != 1 || store.inputs[0].Kind != jobartifacts.KindImage {
		t.Fatalf("artifact inputs = %#v", store.inputs)
	}
	if auditEvent.Kind != jobaudit.KindImageGenerate ||
		auditEvent.Status != jobaudit.StatusAdmitted ||
		auditEvent.ModelID != "gpt-image-2" {
		t.Fatalf("audit event = %#v", auditEvent)
	}
	if strings.Contains(rec.Body.String(), "private prompt") {
		t.Fatalf("image response leaked prompt: %s", rec.Body.String())
	}
}

func TestNewHandlerRegistersVoiceJobRoutesAsFailClosedAdmission(t *testing.T) {
	handler := NewHandler(config.Config{Addr: ":0", Version: "route-test"})

	for _, tc := range []struct {
		name string
		path string
		body string
	}{
		{name: "synthesize", path: "/v1/voice/synthesize", body: `{"text":"hello","provider":"default"}`},
		{name: "transcribe", path: "/v1/voice/transcribe", body: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			var req *http.Request
			if tc.path == "/v1/voice/transcribe" {
				body, contentType := multipartAudioBody(t, map[string]string{"provider": "default"})
				req = httptest.NewRequest(http.MethodPost, tc.path, body)
				req.Header.Set("Content-Type", contentType)
			} else {
				req = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			}

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("%s status = %d, want %d; body=%s", tc.path, rec.Code, http.StatusNotImplemented, rec.Body.String())
			}
			var body ErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode %s error response: %v", tc.path, err)
			}
			if body.Error.Code != "VOICE_JOBS_UNAVAILABLE" {
				t.Fatalf("%s code = %q, want VOICE_JOBS_UNAVAILABLE", tc.path, body.Error.Code)
			}
		})
	}
}

func TestChatImageGeneratorClassifiesProviderTimeout(t *testing.T) {
	service := imagejobs.NewService(
		imagejobs.WithExecutor(&fakeHTTPImageExecutor{err: context.DeadlineExceeded}),
		imagejobs.WithArtifactStore(&fakeHTTPArtifactStore{}),
		imagejobs.WithAuditRecorder(jobaudit.RecorderFunc(
			func(context.Context, jobaudit.Event) error { return nil },
		)),
	)

	_, err := (chatImageGenerator{service: service}).GenerateImage(
		context.Background(),
		chat.ImageGenerationRequest{
			ModelRef: chat.ModelRef{ProviderID: "openai", ModelID: "gpt-image-2"},
			Prompt:   "paint",
			Size:     "1024x1024",
		},
	)

	var imageErr *chat.ImageGenerationError
	if !errors.As(err, &imageErr) || imageErr.Code != chat.ImageProviderTimeoutCode {
		t.Fatalf("GenerateImage() error = %v", err)
	}
}

func TestChatImageGeneratorClassifiesProviderConnectionFailure(t *testing.T) {
	executor, err := imagejobs.NewOpenAICompatibleExecutor(
		imagejobs.OpenAICompatibleExecutorConfig{
			BaseURL: "https://provider.test/v1",
			APIKey:  "fixture-key",
			HTTPClient: &http.Client{Transport: httpRoundTripFunc(
				func(*http.Request) (*http.Response, error) {
					return nil, errors.New("fixture connection interrupted")
				},
			)},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	service := imagejobs.NewService(
		imagejobs.WithExecutor(executor),
		imagejobs.WithArtifactStore(&fakeHTTPArtifactStore{}),
		imagejobs.WithAuditRecorder(jobaudit.RecorderFunc(
			func(context.Context, jobaudit.Event) error { return nil },
		)),
	)

	_, err = (chatImageGenerator{service: service}).GenerateImage(
		context.Background(),
		chat.ImageGenerationRequest{
			ModelRef: chat.ModelRef{ProviderID: "openai", ModelID: "gpt-image-2"},
			Prompt:   "paint",
			Size:     "1024x1024",
		},
	)

	var imageErr *chat.ImageGenerationError
	if !errors.As(err, &imageErr) || imageErr.Code != chat.ImageProviderConnectionCode {
		t.Fatalf("GenerateImage() error = %v", err)
	}
}

func TestNewHandlerRoutesConfiguredVoiceJobService(t *testing.T) {
	executor := &fakeHTTPVoiceExecutor{synthesizeResult: voicejobs.SynthesizeResult{
		JobID:       "job-1",
		Filename:    "voice.mp3",
		ContentType: "audio/mpeg",
		Size:        5,
		Body:        strings.NewReader("audio"),
	}}
	store := &fakeHTTPArtifactStore{artifact: jobartifacts.Artifact{
		FileID:      "44444444-4444-4444-8444-444444444444",
		Purpose:     "audio",
		ContentType: "audio/mpeg",
		Size:        5,
	}}
	var auditEvent jobaudit.Event
	service := voicejobs.NewService(
		voicejobs.WithExecutor(executor),
		voicejobs.WithArtifactStore(store),
		voicejobs.WithAuditRecorder(jobaudit.RecorderFunc(func(_ context.Context, event jobaudit.Event) error {
			auditEvent = event
			return nil
		})),
	)
	handler := NewHandler(
		config.Config{Addr: ":0", Version: "route-test"},
		WithVoiceJobService(service),
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/voice/synthesize",
		strings.NewReader(`{"text":"private speech","provider":"default","modelId":"tts-1","jobId":"job-1"}`),
	)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("voice job status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response voicejobs.SynthesizeResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode voice response: %v", err)
	}
	if response.FileID != "44444444-4444-4444-8444-444444444444" ||
		response.Purpose != "audio" ||
		response.ContentType != "audio/mpeg" ||
		response.Size != 5 {
		t.Fatalf("voice response = %#v", response)
	}
	if !executor.synthesizeCalled ||
		executor.synthesizeRequest.Text != "private speech" ||
		executor.synthesizeRequest.ModelID != "tts-1" {
		t.Fatalf("executor state = called:%v request:%#v", executor.synthesizeCalled, executor.synthesizeRequest)
	}
	if len(store.inputs) != 1 || store.inputs[0].Kind != jobartifacts.KindAudio {
		t.Fatalf("artifact inputs = %#v", store.inputs)
	}
	if auditEvent.Kind != jobaudit.KindVoiceSynthesize ||
		auditEvent.Status != jobaudit.StatusAdmitted ||
		auditEvent.ModelID != "tts-1" {
		t.Fatalf("audit event = %#v", auditEvent)
	}
	if strings.Contains(rec.Body.String(), "private speech") {
		t.Fatalf("voice response leaked synthesis text: %s", rec.Body.String())
	}
}

func TestRateLimitMiddlewareLimitsNonExemptRoutes(t *testing.T) {
	store := newFakeRateLimitStore()
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Redis: config.RedisConfig{
				RateLimitEnabled:  true,
				RateLimitRequests: 2,
				RateLimitWindow:   time.Minute,
			},
		},
		WithRateLimitStore(store),
	)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		req.RemoteAddr = "203.0.113.10:4444"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("request %d status = %d, want 404; body=%s", i+1, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-RateLimit-Limit"); got != "2" {
			t.Fatalf("request %d X-RateLimit-Limit = %q, want 2", i+1, got)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.RemoteAddr = "203.0.113.10:4444"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header is blank")
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "2" {
		t.Fatalf("X-RateLimit-Limit = %q, want 2", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "0" {
		t.Fatalf("X-RateLimit-Remaining = %q, want 0", got)
	}
	if got := rec.Header().Get("X-RateLimit-Reset"); got == "" {
		t.Fatal("X-RateLimit-Reset header is blank")
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode rate limit response: %v", err)
	}
	if body.Error.Code != "RATE_LIMITED" {
		t.Fatalf("error code = %q, want RATE_LIMITED", body.Error.Code)
	}
}

func TestRateLimitMiddlewareLimitsJobControlRoutes(t *testing.T) {
	store := newFakeRateLimitStore()
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Redis: config.RedisConfig{
				RateLimitEnabled:  true,
				RateLimitRequests: 1,
				RateLimitWindow:   time.Minute,
			},
		},
		WithRateLimitStore(store),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/cancel", nil)
	req.RemoteAddr = "203.0.113.20:4444"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("first job cancel status = %d, want 501; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "1" {
		t.Fatalf("first X-RateLimit-Limit = %q, want 1", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/cancel", nil)
	req.RemoteAddr = "203.0.113.20:4444"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second job cancel status = %d, want 429; body=%s", rec.Code, rec.Body.String())
	}
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode job rate limit response: %v", err)
	}
	if body.Error.Code != "RATE_LIMITED" {
		t.Fatalf("job rate limit code = %q, want RATE_LIMITED", body.Error.Code)
	}
}

func TestRateLimitMiddlewareExemptsHealthReadyMetricsAndVersionRoutes(t *testing.T) {
	store := newFakeRateLimitStore()
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Redis: config.RedisConfig{
				RateLimitEnabled:  true,
				RateLimitRequests: 1,
				RateLimitWindow:   time.Minute,
			},
		},
		WithRateLimitStore(store),
	)

	tests := []struct {
		path string
		code int
	}{
		{path: "/health", code: http.StatusOK},
		{path: "/ready", code: http.StatusOK},
		{path: "/metrics", code: http.StatusOK},
		{path: "/v1/version", code: http.StatusOK},
	}

	for _, tt := range tests {
		for i := 0; i < 3; i++ {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.RemoteAddr = "203.0.113.10:4444"
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.code {
				t.Fatalf("%s request %d status = %d, want %d", tt.path, i+1, rec.Code, tt.code)
			}
		}
	}
	if store.calls != 0 {
		t.Fatalf("rate limit store calls = %d, want 0 for exempt routes", store.calls)
	}
}

func TestRateLimitMiddlewareFailsOpenOnStoreError(t *testing.T) {
	store := newFakeRateLimitStore()
	store.err = errors.New("redis down")
	handler := NewHandler(
		config.Config{
			Addr:    ":0",
			Version: "route-test",
			Redis: config.RedisConfig{
				RateLimitEnabled:  true,
				RateLimitRequests: 1,
				RateLimitWindow:   time.Minute,
			},
		},
		WithRateLimitStore(store),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.RemoteAddr = "203.0.113.10:4444"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 fail-open; body=%s", rec.Code, rec.Body.String())
	}
}

type fakeRateLimitStore struct {
	calls  int
	counts map[string]int
	err    error
}

type fakeHTTPImageExecutor struct {
	called  bool
	request imagejobs.GenerateRequest
	result  imagejobs.GenerateResult
	err     error
}

type httpRoundTripFunc func(*http.Request) (*http.Response, error)

func (f httpRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func (e *fakeHTTPImageExecutor) Generate(
	_ context.Context,
	request imagejobs.GenerateRequest,
) (imagejobs.GenerateResult, error) {
	e.called = true
	e.request = request
	if e.err != nil {
		return imagejobs.GenerateResult{}, e.err
	}
	return e.result, nil
}

type fakeHTTPVoiceExecutor struct {
	transcribeCalled   bool
	synthesizeCalled   bool
	transcribeRequest  voicejobs.TranscribeRequest
	synthesizeRequest  voicejobs.SynthesizeRequest
	transcribeResponse voicejobs.TranscribeResponse
	synthesizeResult   voicejobs.SynthesizeResult
	err                error
}

func (e *fakeHTTPVoiceExecutor) Transcribe(
	_ context.Context,
	request voicejobs.TranscribeRequest,
) (voicejobs.TranscribeResponse, error) {
	e.transcribeCalled = true
	e.transcribeRequest = request
	if e.err != nil {
		return voicejobs.TranscribeResponse{}, e.err
	}
	return e.transcribeResponse, nil
}

func (e *fakeHTTPVoiceExecutor) Synthesize(
	_ context.Context,
	request voicejobs.SynthesizeRequest,
) (voicejobs.SynthesizeResult, error) {
	e.synthesizeCalled = true
	e.synthesizeRequest = request
	if e.err != nil {
		return voicejobs.SynthesizeResult{}, e.err
	}
	return e.synthesizeResult, nil
}

type fakeHTTPArtifactStore struct {
	inputs   []jobartifacts.StoreInput
	artifact jobartifacts.Artifact
	err      error
}

func (s *fakeHTTPArtifactStore) Store(
	_ context.Context,
	input jobartifacts.StoreInput,
) (jobartifacts.Artifact, error) {
	s.inputs = append(s.inputs, input)
	if s.err != nil {
		return jobartifacts.Artifact{}, s.err
	}
	return s.artifact, nil
}

func newFakeRateLimitStore() *fakeRateLimitStore {
	return &fakeRateLimitStore{counts: map[string]int{}}
}

func decodeJSONLogLines(t *testing.T, payload []byte) []map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(payload), []byte("\n"))
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode log line %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func (s *fakeRateLimitStore) Allow(
	_ context.Context,
	key string,
	limit int,
	window time.Duration,
	now time.Time,
) (ratelimit.Result, error) {
	s.calls++
	if s.err != nil {
		return ratelimit.Result{}, s.err
	}
	s.counts[key]++
	remaining := limit - s.counts[key]
	if remaining < 0 {
		remaining = 0
	}
	return ratelimit.Result{
		Allowed:    s.counts[key] <= limit,
		Limit:      limit,
		Remaining:  remaining,
		RetryAfter: window,
		ResetAt:    now.Add(window),
	}, nil
}

func multipartAudioBody(t *testing.T, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "audio.webm")
	if err != nil {
		t.Fatalf("create audio part: %v", err)
	}
	if _, err := part.Write([]byte("audio-bytes")); err != nil {
		t.Fatalf("write audio: %v", err)
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return &body, writer.FormDataContentType()
}

type fakeSessionResolver struct {
	session   auth.Session
	err       error
	tokenHash string
}

func (r *fakeSessionResolver) ResolveByTokenHash(_ context.Context, tokenHash string) (auth.Session, error) {
	r.tokenHash = tokenHash
	if r.err != nil {
		return auth.Session{}, r.err
	}
	return r.session, nil
}
