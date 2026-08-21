package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "0123456789abcdef0123456789abcdef"

type stubWorkspaceResolver struct {
	workspace WorkspaceDescriptor
	err       error
	interop   bool
}

func (resolver stubWorkspaceResolver) ResolveWorkspace(
	context.Context,
	string,
) (WorkspaceDescriptor, error) {
	return resolver.workspace, resolver.err
}

func (resolver stubWorkspaceResolver) WindowsPathInterop() bool {
	return resolver.interop
}

func newTestHandler(t *testing.T, resolver WorkspaceResolver) *Handler {
	t.Helper()
	handler, err := NewHandler(HandlerConfig{
		RunnerID: "wsl-test-runner",
		Version:  "test",
		Token:    testToken,
		Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	return handler
}

func authorizedRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func decodeErrorResponse(t *testing.T, recorder *httptest.ResponseRecorder) ErrorResponse {
	t.Helper()
	var response ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return response
}

func TestHandlerRequiresExactBearerTokenWithoutDisclosure(t *testing.T) {
	handler := newTestHandler(t, nil)
	for _, authorization := range []string{
		"",
		"Bearer wrong-token-that-is-long-enough-0000",
		"Basic " + testToken,
		"Bearer " + testToken + " extra",
	} {
		t.Run(authorization, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, CapabilitiesPath, nil)
			request.Header.Set("Authorization", authorization)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
			if strings.Contains(recorder.Body.String(), testToken) ||
				strings.Contains(recorder.Body.String(), "wrong-token") {
				t.Fatal("authorization response disclosed token material")
			}
		})
	}
}

func TestHandlerCapabilitiesAdvertiseFoundationOnly(t *testing.T) {
	handler := newTestHandler(t, stubWorkspaceResolver{interop: true})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authorizedRequest(http.MethodGet, CapabilitiesPath, ""))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var capabilities Capabilities
	if err := json.Unmarshal(recorder.Body.Bytes(), &capabilities); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if capabilities.ProtocolVersion != ProtocolVersion || capabilities.RunnerID != "wsl-test-runner" {
		t.Fatalf("unexpected identity: %+v", capabilities)
	}
	if !capabilities.Features.WorkspaceResolve || !capabilities.Features.WindowsPathInterop {
		t.Fatalf("workspace capabilities missing: %+v", capabilities.Features)
	}
	if capabilities.Features.Execution || capabilities.Features.DirectoryBrowse ||
		capabilities.Features.NativeDirectoryPicker || len(capabilities.Features.PermissionModes) != 0 {
		t.Fatalf("unimplemented capabilities advertised: %+v", capabilities.Features)
	}
}

func TestHandlerRejectsInvalidRouting(t *testing.T) {
	handler := newTestHandler(t, stubWorkspaceResolver{})
	tests := []struct {
		name       string
		request    *http.Request
		wantStatus int
		wantCode   string
	}{
		{
			"wrong method", authorizedRequest(http.MethodPost, CapabilitiesPath, `{}`),
			http.StatusMethodNotAllowed, "AGENT_HOST_METHOD_NOT_ALLOWED",
		},
		{
			"query", authorizedRequest(http.MethodGet, CapabilitiesPath+"?secret=/home/private", ""),
			http.StatusBadRequest, "AGENT_HOST_REQUEST_INVALID",
		},
		{
			"unknown path", authorizedRequest(http.MethodGet, "/internal/v1/private", ""),
			http.StatusNotFound, "AGENT_HOST_ROUTE_NOT_FOUND",
		},
		{
			"wrong protocol", authorizedRequest(
				http.MethodPost, WorkspaceResolvePath,
				`{"protocolVersion":2,"path":"/tmp"}`,
			),
			http.StatusConflict, "AGENT_HOST_PROTOCOL_UNSUPPORTED",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, test.request)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if response := decodeErrorResponse(t, recorder); response.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", response.Error.Code, test.wantCode)
			}
			if strings.Contains(recorder.Body.String(), "/home/private") {
				t.Fatal("routing error disclosed request path")
			}
		})
	}
}

func TestHandlerRejectsMalformedJSON(t *testing.T) {
	handler := newTestHandler(t, stubWorkspaceResolver{})
	largePath := strings.Repeat("a", int(maxControlRequestBytes))
	tests := []struct {
		name        string
		body        string
		contentType string
	}{
		{"unknown field", `{"protocolVersion":1,"path":"/tmp","extra":true}`, "application/json"},
		{"duplicate field", `{"protocolVersion":1,"protocolVersion":1,"path":"/tmp"}`, "application/json"},
		{"trailing json", `{"protocolVersion":1,"path":"/tmp"}{}`, "application/json"},
		{"oversize", `{"protocolVersion":1,"path":"` + largePath + `"}`, "application/json"},
		{"wrong content type", `{"protocolVersion":1,"path":"/tmp"}`, "text/plain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := authorizedRequest(http.MethodPost, WorkspaceResolvePath, test.body)
			request.Header.Set("Content-Type", test.contentType)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if response := decodeErrorResponse(t, recorder); response.Error.Code != "AGENT_HOST_REQUEST_INVALID" {
				t.Fatalf("unexpected response: %+v", response)
			}
		})
	}
}

func TestHandlerMapsWorkspaceErrorsWithoutHostPath(t *testing.T) {
	secretPath := "/home/private/customer-project"
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid", ErrWorkspacePathInvalid, http.StatusBadRequest, "WORKSPACE_PATH_INVALID"},
		{"unavailable", ErrWorkspacePathUnavailable, http.StatusNotFound, "WORKSPACE_PATH_UNAVAILABLE"},
		{"interop", ErrWindowsInteropUnavailable, http.StatusServiceUnavailable, "WINDOWS_PATH_INTEROP_UNAVAILABLE"},
		{"wrapped internal", errors.New("probe failed: " + secretPath), http.StatusBadRequest, "WORKSPACE_PATH_INVALID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newTestHandler(t, stubWorkspaceResolver{err: test.err})
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, authorizedRequest(
				http.MethodPost,
				WorkspaceResolvePath,
				`{"protocolVersion":1,"path":"`+secretPath+`"}`,
			))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if response := decodeErrorResponse(t, recorder); response.Error.Code != test.wantCode {
				t.Fatalf("code = %q, want %q", response.Error.Code, test.wantCode)
			}
			if strings.Contains(recorder.Body.String(), secretPath) {
				t.Fatal("workspace error disclosed Host path")
			}
		})
	}
}
