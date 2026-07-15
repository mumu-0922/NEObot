package plugins

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

func TestHandlerListsUnavailableRegistry(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/plugins", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response ListResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Unavailable {
		t.Fatal("Unavailable = false, want true")
	}
	if len(response.Plugins) != 0 {
		t.Fatalf("plugins = %#v, want empty", response.Plugins)
	}
}

func TestHandlerFailsClosedForInstallAndIdOnlyExecute(t *testing.T) {
	handler := NewHandler(nil)
	cases := []struct {
		name   string
		path   string
		body   string
		status int
		code   string
	}{
		{name: "install", path: "/v1/plugins/install", body: `{"authConfig":{"value":"sk_live_secret"},"args":{"token":"private"}}`, status: http.StatusNotImplemented, code: "PLUGIN_INSTALL_UNAVAILABLE"},
		{name: "execute id-only", path: "/v1/plugins/execute", body: `{"pluginId":"weather","functionName":"lookup","authConfig":{"value":"sk_live_secret"},"args":{"token":"private"}}`, status: http.StatusNotImplemented, code: "PLUGIN_REGISTRY_REQUIRED"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.status, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "sk_live_secret") || strings.Contains(rec.Body.String(), "private") {
				t.Fatalf("response leaked request secret: %s", rec.Body.String())
			}
			assertErrorCode(t, rec, tc.code)
		})
	}
}

func TestHandlerExecutesPluginGETWithBoundedSandbox(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/lookup/42" {
			t.Fatalf("path = %q, want /lookup/42", r.URL.Path)
		}
		if r.URL.Query().Get("city") != "Shanghai" {
			t.Fatalf("city query = %q", r.URL.Query().Get("city"))
		}
		return jsonResponse(http.StatusOK, `{"ok":true,"city":"Shanghai"}`), nil
	})}

	handler := NewHandler(NewService(config.Config{}, WithAllowPrivateNetwork(true), WithHTTPClient(client)))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/execute", bytes.NewReader(mustJSON(t, executePayload("https://plugins.example"))))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response ExecuteResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result, ok := response.Result.(map[string]any)
	if !ok || result["ok"] != true || result["city"] != "Shanghai" {
		t.Fatalf("result = %#v", response.Result)
	}
}

func TestHandlerBlocksPrivatePluginURLByDefault(t *testing.T) {
	handler := NewHandler(NewService(config.Config{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/execute", bytes.NewReader(mustJSON(t, executePayload("http://127.0.0.1:1"))))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	assertErrorCode(t, rec, "PLUGIN_URL_BLOCKED")
}

func TestHandlerBlocksPrivatePluginRedirectByDefault(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "127.0.0.1" {
			t.Fatalf("followed private redirect to %s", r.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusFound,
			Header: http.Header{
				"Location": []string{"http://127.0.0.1/private"},
			},
			Body:    io.NopCloser(strings.NewReader("")),
			Request: r,
		}, nil
	})}
	handler := NewHandler(NewService(config.Config{}, WithHTTPClient(client)))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/execute", bytes.NewReader(mustJSON(t, executePayload("https://93.184.216.34"))))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	assertErrorCode(t, rec, "PLUGIN_URL_BLOCKED")
}

func TestHandlerDecryptsPluginAuthSecret(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	const kid = "kid-test"
	const pluginID = "auth-plugin"
	envelope := encryptSecretEnvelope(t, key, kid, "plugin:"+pluginID+":auth", "secret-token")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("X-API-Key"); got != "secret-token" {
			t.Fatalf("X-API-Key = %q, want secret-token", got)
		}
		return jsonResponse(http.StatusOK, `{"authorized":true}`), nil
	})}

	handler := NewHandler(NewService(config.Config{
		BYOK: config.BYOKConfig{PrivateKeyPEM: privateKeyPEM(key), KeyID: kid},
	}, WithAllowPrivateNetwork(true), WithHTTPClient(client)))
	payload := executePayload("https://plugins.example")
	payload.Plugin.ID = pluginID
	payload.Plugin.Auth = &PluginAuth{Type: "apiKey", Name: "X-API-Key"}
	payload.AuthConfig = &PluginAuthConfig{Type: "apiKey", Key: "X-API-Key", AddTo: "header", ValueSecret: &envelope}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/execute", bytes.NewReader(mustJSON(t, payload)))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandlerDecryptsPluginAuthSecretInQuery(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	const kid = "kid-test"
	const pluginID = "query-auth-plugin"
	envelope := encryptSecretEnvelope(t, key, kid, "plugin:"+pluginID+":auth", "secret-token")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.Query().Get("api_key"); got != "secret-token" {
			t.Fatalf("api_key query = %q, want secret-token", got)
		}
		if got := r.Header.Get("api_key"); got != "" {
			t.Fatalf("api_key header = %q, want empty", got)
		}
		return jsonResponse(http.StatusOK, `{"authorized":true}`), nil
	})}

	handler := NewHandler(NewService(config.Config{
		BYOK: config.BYOKConfig{PrivateKeyPEM: privateKeyPEM(key), KeyID: kid},
	}, WithAllowPrivateNetwork(true), WithHTTPClient(client)))
	payload := executePayload("https://plugins.example")
	payload.Plugin.ID = pluginID
	payload.Plugin.Auth = &PluginAuth{Type: "apiKey", Name: "api_key", In: "query"}
	payload.AuthConfig = &PluginAuthConfig{Type: "apiKey", Key: "api_key", AddTo: "query", ValueSecret: &envelope}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/execute", bytes.NewReader(mustJSON(t, payload)))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestHandlerRejectsOversizedPluginResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"tooLarge":"`+strings.Repeat("x", 32)+`"}`), nil
	})}
	handler := NewHandler(NewService(
		config.Config{},
		WithAllowPrivateNetwork(true),
		WithHTTPClient(client),
		WithMaxResponseBytes(16),
	))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/execute", bytes.NewReader(mustJSON(t, executePayload("https://plugins.example"))))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	assertErrorCode(t, rec, "PLUGIN_RESPONSE_TOO_LARGE")
}

func TestHandlerRejectsPlaintextPluginAuth(t *testing.T) {
	handler := NewHandler(NewService(config.Config{}, WithAllowPrivateNetwork(true)))
	payload := executePayload("https://example.com")
	payload.Plugin.Auth = &PluginAuth{Type: "apiKey", Name: "X-API-Key"}
	payload.AuthConfig = &PluginAuthConfig{Type: "apiKey", Value: "sk_live_secret"}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/plugins/execute", bytes.NewReader(mustJSON(t, payload)))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk_live_secret") {
		t.Fatalf("response leaked plaintext secret: %s", rec.Body.String())
	}
	assertErrorCode(t, rec, "PLAINTEXT_PLUGIN_AUTH_REJECTED")
}

func TestHandlerRejectsUnsupportedMethods(t *testing.T) {
	handler := NewHandler(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v1/plugins", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if rec.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", rec.Header().Get("Allow"), http.MethodGet)
	}
}

func executePayload(baseURL string) ExecuteRequest {
	fn := PluginFunction{
		Name:        "lookup",
		Description: "Lookup",
		Path:        "/lookup/{id}",
		Method:      http.MethodGet,
		Parameters:  map[string]any{"type": "object"},
	}
	return ExecuteRequest{
		Plugin: &Plugin{
			ID:          "test-plugin",
			Title:       "Test Plugin",
			Description: "Test plugin",
			LogoURL:     "",
			ManifestURL: "https://plugins.test/test.json",
			BaseURL:     baseURL,
			Functions:   []PluginFunction{fn},
			Auth:        &PluginAuth{Type: "none"},
		},
		FunctionDef: &fn,
		Args: map[string]any{
			"id":   "42",
			"city": "Shanghai",
		},
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return data
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var body ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != want {
		t.Fatalf("code = %q, want %q", body.Error.Code, want)
	}
}

func privateKeyPEM(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func encryptSecretEnvelope(t *testing.T, key *rsa.PrivateKey, kid string, context string, secret string) runtimeconfig.EncryptedSecretEnvelope {
	t.Helper()
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatalf("generate aes key: %v", err)
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		t.Fatalf("new aes cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new gcm: %v", err)
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("generate iv: %v", err)
	}
	ciphertext := gcm.Seal(nil, iv, []byte(secret), []byte(context))
	wrappedKey, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &key.PublicKey, aesKey, nil)
	if err != nil {
		t.Fatalf("wrap aes key: %v", err)
	}
	return runtimeconfig.EncryptedSecretEnvelope{
		V:          1,
		KID:        kid,
		Alg:        "RSA-OAEP-256+A256GCM",
		IV:         base64.RawURLEncoding.EncodeToString(iv),
		WrappedKey: base64.RawURLEncoding.EncodeToString(wrappedKey),
		Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		Context:    context,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
