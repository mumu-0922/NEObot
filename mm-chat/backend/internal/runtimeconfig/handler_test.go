package runtimeconfig

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
)

func TestHandlerRoutesRuntimeConfig(t *testing.T) {
	handler := NewHandler(NewService(config.Config{}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/config", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var response PublicConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ModelProvider.Available || len(response.ModelProvider.Models) != 0 {
		t.Fatalf("model provider = %#v", response.ModelProvider)
	}
	for _, retiredField := range []string{`"rag"`, `"documentParseJobStore"`} {
		if strings.Contains(rec.Body.String(), retiredField) {
			t.Fatalf("retired field %s remains in public config: %s", retiredField, rec.Body.String())
		}
	}
}

func TestHandlerRoutesAdminTaskModelSettings(t *testing.T) {
	providerRepo := &fakeProviderConfigRepository{
		ok: true,
		stored: StoredProviderConfig{
			UserID: authDevelopmentUserID(), ProviderID: "CUSTOM", Label: "Custom",
			Config: StoredProviderConfigPayload{
				Kind: providerConfigKindModel, Models: []string{"gpt-task"}, Enabled: true,
			},
		},
	}
	handler := NewHandler(NewService(
		config.Config{},
		WithProviderConfigRepository(providerRepo),
		WithTaskModelSettingsRepository(&fakeTaskModelSettingsRepository{}),
	))

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(
		http.MethodGet, "/v1/admin/task-models", nil,
	))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"configured":false`) {
		t.Fatalf("get status = %d, body=%s", get.Code, get.Body.String())
	}
	if get.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("task model Cache-Control = %q", get.Header().Get("Cache-Control"))
	}

	patch := httptest.NewRecorder()
	handler.ServeHTTP(patch, httptest.NewRequest(
		http.MethodPatch,
		"/v1/admin/task-models",
		strings.NewReader(`{"titleGeneration":"CUSTOM:gpt-task"}`),
	))
	if patch.Code != http.StatusOK ||
		!strings.Contains(patch.Body.String(), `"titleGeneration":"CUSTOM:gpt-task"`) ||
		!strings.Contains(patch.Body.String(), `"configured":true`) {
		t.Fatalf("patch status = %d, body=%s", patch.Code, patch.Body.String())
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(
		http.MethodPatch,
		"/v1/admin/task-models",
		strings.NewReader(`{"memory":"CUSTOM:missing"}`),
	))
	if invalid.Code != http.StatusConflict ||
		!strings.Contains(invalid.Body.String(), "TASK_MODEL_UNAVAILABLE") {
		t.Fatalf("invalid status = %d, body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestHandlerProviderModelsRequiresServerAuthority(t *testing.T) {
	handler := NewHandler(NewService(config.Config{}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost,
		"/v1/providers/models",
		strings.NewReader(`{"provider":{"source":"server-default"}}`),
	))

	if rec.Code != http.StatusServiceUnavailable ||
		!strings.Contains(rec.Body.String(), "DATABASE_REQUIRED") {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
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

func TestHandlerRedactsProviderVaultErrors(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{err: ErrProviderSecretVaultUnavailable, code: "PROVIDER_SECRET_VAULT_UNAVAILABLE"},
		{err: ErrProviderSecretInvalid, code: "PROVIDER_SECRET_UNAVAILABLE"},
	} {
		rec := httptest.NewRecorder()
		writeServiceError(rec, test.err)
		if rec.Code != http.StatusServiceUnavailable ||
			!strings.Contains(rec.Body.String(), test.code) {
			t.Fatalf("error %v status = %d, body=%s", test.err, rec.Code, rec.Body.String())
		}
	}
}

func TestHandlerRoutesAdminProviderCollection(t *testing.T) {
	repo := &fakeProviderConfigRepository{}
	handler := NewHandler(NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
	))

	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(
		http.MethodPut,
		"/v1/admin/providers/CUSTOM",
		strings.NewReader(`{"name":"Custom","type":"OpenAI Compatible","baseUrl":"https://custom.example/v1","models":["gpt-custom"],"enabled":true}`),
	))
	if put.Code != http.StatusOK || !strings.Contains(put.Body.String(), `"source":"server-stored"`) {
		t.Fatalf("put status = %d, body=%s", put.Code, put.Body.String())
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/v1/admin/providers", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", list.Code, list.Body.String())
	}
	var response AdminProviderConfigsResponse
	if err := json.Unmarshal(list.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Providers) != 2 || response.Providers[1].ID != "CUSTOM" {
		t.Fatalf("providers = %#v", response.Providers)
	}

	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/v1/admin/providers/CUSTOM", nil))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestHandlerRoutesProviderConnectionTestAndActivation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-live"}]}`))
	}))
	defer upstream.Close()

	vault := testProviderSecretVault(t, "handler-v1", 22)
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: testStoredVaultProvider(
			t,
			vault,
			"CUSTOM",
			ProviderTypeOpenAICompatible,
			upstream.URL,
			"handler-key",
		),
	}
	handler := NewHandler(NewService(
		config.Config{Provider: config.ProviderConfig{Timeout: time.Second}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
	))

	tested := httptest.NewRecorder()
	handler.ServeHTTP(tested, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/providers/CUSTOM/test",
		nil,
	))
	if tested.Code != http.StatusOK ||
		!strings.Contains(tested.Body.String(), `"connectionTestValid":true`) ||
		!strings.Contains(tested.Body.String(), `"models":["gpt-live"]`) {
		t.Fatalf("test status = %d, body=%s", tested.Code, tested.Body.String())
	}

	activated := httptest.NewRecorder()
	handler.ServeHTTP(activated, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/providers/CUSTOM/activate",
		nil,
	))
	if activated.Code != http.StatusOK ||
		!strings.Contains(activated.Body.String(), `"enabled":true`) {
		t.Fatalf("activate status = %d, body=%s", activated.Code, activated.Body.String())
	}

	wrongMethod := httptest.NewRecorder()
	handler.ServeHTTP(wrongMethod, httptest.NewRequest(
		http.MethodGet,
		"/v1/admin/providers/CUSTOM/test",
		nil,
	))
	if wrongMethod.Code != http.StatusMethodNotAllowed ||
		wrongMethod.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("wrong method status = %d", wrongMethod.Code)
	}
}

func TestHandlerRoutesModelBuiltInSearchAttestation(t *testing.T) {
	vault := testProviderSecretVault(t, "handler-built-in-v1", 24)
	stored := testStoredVaultProvider(
		t,
		vault,
		"CUSTOM",
		ProviderTypeOpenAICompatible,
		"https://custom.example/v1",
		"handler-built-in-key",
	)
	stored.Config.Models = []string{"gpt-search"}
	stored.Config.ModelBuiltInSearchProtocol = ModelBuiltInSearchProtocolOpenAIResponses
	stored.Config.ModelBuiltInSearchModel = "gpt-search"
	attestStoredProvider(&stored, true)
	repo := &fakeProviderConfigRepository{ok: true, stored: stored}
	var tested ModelBuiltInSearchTestInput
	handler := NewHandler(NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(vault),
		WithModelBuiltInSearchTester(modelBuiltInSearchTesterFunc(func(
			_ context.Context,
			input ModelBuiltInSearchTestInput,
		) (int, error) {
			tested = input
			return 1, nil
		})),
	))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/providers/CUSTOM/built-in-search-test",
		strings.NewReader(`{"protocol":"openai_responses","model":"gpt-search"}`),
	))
	if recorder.Code != http.StatusOK ||
		!strings.Contains(recorder.Body.String(), `"sourceCount":1`) ||
		!strings.Contains(recorder.Body.String(), `"connectionTestValid":true`) ||
		tested.ProviderID != "CUSTOM" || tested.Model != "gpt-search" ||
		tested.APIKey != "handler-built-in-key" {
		t.Fatalf(
			"built-in test status/body/input = %d / %s / %#v",
			recorder.Code,
			recorder.Body.String(),
			tested,
		)
	}
}

func TestHandlerRoutesAdminSearchProviderLifecycle(t *testing.T) {
	repo := &fakeProviderConfigRepository{}
	handler := NewHandler(NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
	))

	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(
		http.MethodPut,
		"/v1/admin/search/providers/tavily",
		strings.NewReader(`{"name":"Tavily","baseUrl":"","enabled":false}`),
	))
	if put.Code != http.StatusOK ||
		!strings.Contains(put.Body.String(), `"provider":"tavily"`) ||
		!strings.Contains(put.Body.String(), `"hasApiKey":false`) {
		t.Fatalf("put status = %d, body=%s", put.Code, put.Body.String())
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(
		http.MethodGet,
		"/v1/admin/search/providers",
		nil,
	))
	if list.Code != http.StatusOK ||
		!strings.Contains(list.Body.String(), `"providers":[`) {
		t.Fatalf("list status = %d, body=%s", list.Code, list.Body.String())
	}

	tested := httptest.NewRecorder()
	handler.ServeHTTP(tested, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/search/providers/tavily/test",
		nil,
	))
	if tested.Code != http.StatusBadRequest ||
		!strings.Contains(tested.Body.String(), "SEARCH_PROVIDER_SECRET_REQUIRED") {
		t.Fatalf("test status = %d, body=%s", tested.Code, tested.Body.String())
	}

	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, httptest.NewRequest(
		http.MethodDelete,
		"/v1/admin/search/providers/tavily",
		nil,
	))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestHandlerRoutesEncryptedAdminVoiceProviderLifecycle(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	const fixtureCredential = "voice-handler-fixture"
	repo := &fakeProviderConfigRepository{}
	service := NewService(
		config.Config{BYOK: config.BYOKConfig{PrivateKeyPEM: pemValue}},
		WithProviderConfigRepository(repo),
		WithProviderSecretVault(testProviderSecretVault(t, "voice-handler-v1", 53)),
		WithVoiceProviderHTTPClient(&http.Client{Transport: voiceRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
			if request.Header.Get("Authorization") != "Bearer "+fixtureCredential {
				t.Fatalf("unexpected Voice authorization header")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
				Body:       io.NopCloser(strings.NewReader("ID3-handler-audio")),
			}, nil
		})}),
	)
	handler := NewHandler(service)
	requestBody, err := json.Marshal(UpdateAdminVoiceProviderConfigRequest{
		APIKeySecret: encryptedSecretEnvelope(
			t,
			privateKey,
			fixtureCredential,
			voiceProviderIngressContext(voiceProviderSiliconFlow),
		),
	})
	if err != nil {
		t.Fatal(err)
	}

	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(
		http.MethodPut,
		"/v1/admin/voice/providers/siliconflow",
		strings.NewReader(string(requestBody)),
	))
	if put.Code != http.StatusOK ||
		!strings.Contains(put.Body.String(), `"provider":"siliconflow"`) ||
		!strings.Contains(put.Body.String(), `"hasApiKey":true`) ||
		strings.Contains(put.Body.String(), fixtureCredential) ||
		storedSecretAlgorithm(repo.stored.EncryptedSecretRef) == byokAlgorithm {
		t.Fatalf("put status/body = %d / %s", put.Code, put.Body.String())
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(
		http.MethodGet,
		"/v1/admin/voice/providers",
		nil,
	))
	if list.Code != http.StatusOK || list.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(list.Body.String(), `"providers":[`) {
		t.Fatalf("list status/body = %d / %s", list.Code, list.Body.String())
	}

	tested := httptest.NewRecorder()
	handler.ServeHTTP(tested, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/voice/providers/siliconflow/test",
		nil,
	))
	if tested.Code != http.StatusOK ||
		!strings.Contains(tested.Body.String(), `"connectionTestValid":true`) ||
		!strings.Contains(tested.Body.String(), `"contentType":"audio/mpeg"`) {
		t.Fatalf("test status/body = %d / %s", tested.Code, tested.Body.String())
	}

	activated := httptest.NewRecorder()
	handler.ServeHTTP(activated, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/voice/providers/siliconflow/activate",
		nil,
	))
	if activated.Code != http.StatusOK ||
		!strings.Contains(activated.Body.String(), `"enabled":true`) {
		t.Fatalf("activate status/body = %d / %s", activated.Code, activated.Body.String())
	}

	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/voice/providers/siliconflow/clone",
		nil,
	))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown action status = %d, body=%s", unknown.Code, unknown.Body.String())
	}

	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, httptest.NewRequest(
		http.MethodDelete,
		"/v1/admin/voice/providers/siliconflow",
		nil,
	))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestHandlerMapsVoiceProviderAdminErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{ErrVoiceProviderConfigUnsupported, http.StatusBadRequest, "VOICE_PROVIDER_CONFIG_UNSUPPORTED"},
		{ErrVoiceProviderNotFound, http.StatusNotFound, "VOICE_PROVIDER_NOT_FOUND"},
		{ErrVoiceProviderSecretRequired, http.StatusBadRequest, "VOICE_PROVIDER_SECRET_REQUIRED"},
		{ErrVoiceProviderConnectionFailed, http.StatusBadGateway, "VOICE_PROVIDER_CONNECTION_TEST_FAILED"},
		{ErrVoiceProviderConfigChanged, http.StatusConflict, "VOICE_PROVIDER_CONFIG_CHANGED"},
	} {
		recorder := httptest.NewRecorder()
		writeServiceError(recorder, test.err)
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) {
			t.Fatalf("error %v status = %d, body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestHandlerMapsSearchProviderAdminErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{ErrSearchProviderConfigUnsupported, http.StatusBadRequest, "SEARCH_PROVIDER_CONFIG_UNSUPPORTED"},
		{ErrSearchProviderNotFound, http.StatusNotFound, "SEARCH_PROVIDER_NOT_FOUND"},
		{ErrSearchProviderSecretRequired, http.StatusBadRequest, "SEARCH_PROVIDER_SECRET_REQUIRED"},
		{ErrSearchProviderConnectionFailed, http.StatusBadGateway, "SEARCH_PROVIDER_CONNECTION_TEST_FAILED"},
		{ErrSearchProviderConfigChanged, http.StatusConflict, "SEARCH_PROVIDER_CONFIG_CHANGED"},
	} {
		recorder := httptest.NewRecorder()
		writeServiceError(recorder, test.err)
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) {
			t.Fatalf("error %v status = %d, body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}

func TestHandlerRoutesAdminRAGProviderLifecycle(t *testing.T) {
	repo := &fakeProviderConfigRepository{
		ok: true,
		stored: StoredProviderConfig{
			ID:         "rag-provider-config-1",
			UserID:     authDevelopmentUserID(),
			ProviderID: ragProviderRecordID(RAGProviderSiliconFlow),
			Label:      "SiliconFlow",
			Config: StoredProviderConfigPayload{
				Kind: providerConfigKindRAG, RAGProvider: string(RAGProviderSiliconFlow),
			},
		},
	}
	handler := NewHandler(NewService(
		config.Config{},
		WithProviderConfigRepository(repo),
	))

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(
		http.MethodGet,
		"/v1/admin/rag/providers",
		nil,
	))
	if list.Code != http.StatusOK ||
		list.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(list.Body.String(), `"providers":[`) {
		t.Fatalf("list status = %d, body=%s", list.Code, list.Body.String())
	}

	configure := httptest.NewRecorder()
	handler.ServeHTTP(configure, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/rag/providers/siliconflow/configure",
		strings.NewReader(`{}`),
	))
	if configure.Code != http.StatusBadRequest ||
		!strings.Contains(configure.Body.String(), "RAG_PROVIDER_SECRET_REQUIRED") {
		t.Fatalf("configure status = %d, body=%s", configure.Code, configure.Body.String())
	}

	invalidConfigure := httptest.NewRecorder()
	handler.ServeHTTP(invalidConfigure, httptest.NewRequest(
		http.MethodPost,
		"/v1/admin/rag/providers/siliconflow/configure",
		strings.NewReader(`{"apiKeySecret":{},"unexpected":true}`),
	))
	if invalidConfigure.Code != http.StatusBadRequest ||
		!strings.Contains(invalidConfigure.Body.String(), "INVALID_REQUEST") {
		t.Fatalf(
			"invalid configure status = %d, body=%s",
			invalidConfigure.Code,
			invalidConfigure.Body.String(),
		)
	}

	for _, path := range []string{
		"/v1/admin/rag/providers/jina/test",
		"/v1/admin/rag/providers/jina/activate",
	} {
		retired := httptest.NewRecorder()
		handler.ServeHTTP(retired, httptest.NewRequest(http.MethodPost, path, nil))
		if retired.Code != http.StatusNotFound {
			t.Fatalf("retired route %s status = %d, body=%s", path, retired.Code, retired.Body.String())
		}
	}

	put := httptest.NewRecorder()
	handler.ServeHTTP(put, httptest.NewRequest(
		http.MethodPut,
		"/v1/admin/rag/providers/siliconflow",
		strings.NewReader(`{"name":"SiliconFlow"}`),
	))
	if put.Code != http.StatusMethodNotAllowed || put.Header().Get("Allow") != http.MethodDelete {
		t.Fatalf("retired put status = %d, allow=%q, body=%s", put.Code, put.Header().Get("Allow"), put.Body.String())
	}

	deleted := httptest.NewRecorder()
	handler.ServeHTTP(deleted, httptest.NewRequest(
		http.MethodDelete,
		"/v1/admin/rag/providers/siliconflow",
		nil,
	))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s", deleted.Code, deleted.Body.String())
	}

	for _, request := range []*http.Request{
		httptest.NewRequest(
			http.MethodPost,
			"/v1/admin/rag/providers/jina/configure",
			strings.NewReader(`{"apiKeySecret":{"ciphertext":"retired"}}`),
		),
		httptest.NewRequest(
			http.MethodDelete,
			"/v1/admin/rag/providers/jina",
			nil,
		),
	} {
		retired := httptest.NewRecorder()
		handler.ServeHTTP(retired, request)
		if retired.Code != http.StatusBadRequest ||
			!strings.Contains(retired.Body.String(), "RAG_PROVIDER_CONFIG_UNSUPPORTED") {
			t.Fatalf("retired Jina route status=%d body=%s", retired.Code, retired.Body.String())
		}
	}
}

func TestHandlerMapsRAGProviderAdminErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{ErrRAGProviderConfigUnsupported, http.StatusBadRequest, "RAG_PROVIDER_CONFIG_UNSUPPORTED"},
		{ErrRAGProviderNotFound, http.StatusNotFound, "RAG_PROVIDER_NOT_FOUND"},
		{ErrRAGProviderSecretRequired, http.StatusBadRequest, "RAG_PROVIDER_SECRET_REQUIRED"},
		{ErrRAGProviderSecretVaultUnavailable, http.StatusServiceUnavailable, "RAG_PROVIDER_SECRET_VAULT_UNAVAILABLE"},
		{ErrRAGProviderSecretInvalid, http.StatusServiceUnavailable, "RAG_PROVIDER_SECRET_UNAVAILABLE"},
		{ErrRAGProviderConnectionFailed, http.StatusBadGateway, "RAG_PROVIDER_CONNECTION_TEST_FAILED"},
		{ErrRAGProviderConfigChanged, http.StatusConflict, "RAG_PROVIDER_CONFIG_CHANGED"},
	} {
		recorder := httptest.NewRecorder()
		writeServiceError(recorder, test.err)
		if recorder.Code != test.status || !strings.Contains(recorder.Body.String(), test.code) {
			t.Fatalf("error %v status = %d, body=%s", test.err, recorder.Code, recorder.Body.String())
		}
	}
}
