package plugins

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerListsUnavailableRegistry(t *testing.T) {
	handler := NewHandler()
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

func TestHandlerFailsClosedForInstallAndExecute(t *testing.T) {
	handler := NewHandler()
	cases := []struct {
		name string
		path string
		code string
	}{
		{name: "install", path: "/v1/plugins/install", code: "PLUGIN_INSTALL_UNAVAILABLE"},
		{name: "execute", path: "/v1/plugins/execute", code: "PLUGIN_EXECUTION_UNAVAILABLE"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(
				http.MethodPost,
				tc.path,
				strings.NewReader(`{"authConfig":{"value":"sk_live_secret"},"args":{"token":"private"}}`),
			)

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "sk_live_secret") || strings.Contains(rec.Body.String(), "private") {
				t.Fatalf("response leaked request secret: %s", rec.Body.String())
			}
			var body ErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body.Error.Code != tc.code {
				t.Fatalf("code = %q, want %q", body.Error.Code, tc.code)
			}
		})
	}
}

func TestHandlerRejectsUnsupportedMethods(t *testing.T) {
	handler := NewHandler()
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
