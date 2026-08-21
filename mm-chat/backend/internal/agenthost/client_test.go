package agenthost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func startUnixHTTPServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	path := filepath.Join(secureSocketDirectory(t), "agent-host.sock")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("ListenUnix() error = %v", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	done := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(done)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		_ = listener.Close()
		<-done
	})
	return path
}

func newTestClient(t *testing.T, socketPath, expectedRunnerID string) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{
		SocketPath:       socketPath,
		Token:            testToken,
		ExpectedRunnerID: expectedRunnerID,
		Timeout:          2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestClientRunnerID(t *testing.T) {
	client := &Client{expectedRunnerID: "wsl-test-runner"}
	if got := client.RunnerID(); got != "wsl-test-runner" {
		t.Fatalf("RunnerID() = %q", got)
	}
	var nilClient *Client
	if got := nilClient.RunnerID(); got != "" {
		t.Fatalf("nil RunnerID() = %q", got)
	}
}

func TestClientServerRoundTripOverUnixSocket(t *testing.T) {
	project := t.TempDir()
	resolver := newTestResolver(t)
	handler := newTestHandler(t, resolver)
	client := newTestClient(t, startUnixHTTPServer(t, handler), "wsl-test-runner")
	capabilities, err := client.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if capabilities.RunnerID != "wsl-test-runner" || !capabilities.Features.WorkspaceResolve ||
		!capabilities.Features.DirectoryBrowse || capabilities.Features.Execution {
		t.Fatalf("unexpected capabilities: %+v", capabilities)
	}
	workspace, err := client.ResolveWorkspace(context.Background(), project)
	if err != nil {
		t.Fatalf("ResolveWorkspace() error = %v", err)
	}
	if workspace.CanonicalPath != project || workspace.PathKind != "wsl" {
		t.Fatalf("unexpected workspace: %+v", workspace)
	}
	listing, err := client.BrowseDirectories(context.Background(), project)
	if err != nil {
		t.Fatalf("BrowseDirectories() error = %v", err)
	}
	if listing.Path != project || listing.Entries == nil {
		t.Fatalf("unexpected directory listing: %+v", listing)
	}
}

func TestClientRejectsRunnerIdentityMismatch(t *testing.T) {
	handler := newTestHandler(t, stubWorkspaceResolver{})
	client := newTestClient(t, startUnixHTTPServer(t, handler), "wsl-other-runner")
	if _, err := client.Capabilities(context.Background()); !errors.Is(err, ErrHostProtocol) {
		t.Fatalf("error = %v, want %v", err, ErrHostProtocol)
	}
}

func TestClientRejectsMalformedOrOversizeResponses(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
	}{
		{"wrong content type", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte(`{}`))
		})},
		{"unknown field", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"protocolVersion":1,"runnerId":"wsl-test-runner","unknown":true}`))
		})},
		{"duplicate field", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"protocolVersion":1,"protocolVersion":1,"runnerId":"wsl-test-runner"}`))
		})},
		{"trailing json", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{} {}`))
		})},
		{"oversize", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(strings.Repeat("x", int(maxControlResponseBytes)+1)))
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, startUnixHTTPServer(t, test.handler), "wsl-test-runner")
			if _, err := client.Capabilities(context.Background()); !errors.Is(err, ErrHostProtocol) {
				t.Fatalf("error = %v, want %v", err, ErrHostProtocol)
			}
		})
	}
}

func TestClientRejectsInvalidCapabilities(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(Capabilities{
			ProtocolVersion: ProtocolVersion,
			RunnerID:        "wsl-test-runner",
			Version:         "test",
			Platform:        "linux-wsl\nprivate",
			Architecture:    "amd64",
			Features: HostFeatures{
				PermissionModes: []PermissionMode{"not-enforced"},
			},
			Limits: HostLimits{
				MaxRequestBytes:  maxControlRequestBytes,
				MaxResponseBytes: maxControlResponseBytes,
				MaxPathBytes:     maxWorkspacePathBytes,
			},
		})
	})
	client := newTestClient(t, startUnixHTTPServer(t, handler), "wsl-test-runner")
	if _, err := client.Capabilities(context.Background()); !errors.Is(err, ErrHostProtocol) {
		t.Fatalf("error = %v, want %v", err, ErrHostProtocol)
	}
}

func TestClientRejectsInvalidWorkspaceFingerprint(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(WorkspaceResolveResponse{
			ProtocolVersion: ProtocolVersion,
			RunnerID:        "wsl-test-runner",
			Workspace: WorkspaceDescriptor{
				CanonicalPath:        "/tmp/project",
				DisplayPath:          "/tmp/project",
				PathKind:             "wsl",
				DirectoryFingerprint: "sha256:" + strings.Repeat("z", 64),
			},
		})
	})
	client := newTestClient(t, startUnixHTTPServer(t, handler), "wsl-test-runner")
	if _, err := client.ResolveWorkspace(context.Background(), "/tmp/project"); !errors.Is(err, ErrHostProtocol) {
		t.Fatalf("error = %v, want %v", err, ErrHostProtocol)
	}
}

func TestClientReturnsSanitizedRemoteError(t *testing.T) {
	secretPath := "/home/private/customer-project"
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(ErrorResponse{Error: ErrorBody{
			Code:    "WORKSPACE_PATH_UNAVAILABLE",
			Message: "probe failed for " + secretPath,
		}})
	})
	client := newTestClient(t, startUnixHTTPServer(t, handler), "wsl-test-runner")
	_, err := client.ResolveWorkspace(context.Background(), secretPath)
	var remoteError RemoteError
	if !errors.As(err, &remoteError) || remoteError.Code != "WORKSPACE_PATH_UNAVAILABLE" {
		t.Fatalf("error = %#v", err)
	}
	if strings.Contains(err.Error(), secretPath) || strings.Contains(remoteError.Message, secretPath) {
		t.Fatal("client error disclosed Host path")
	}
}

func TestClientRejectsUnknownRemoteErrorCode(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"code":"FUTURE_OR_HOSTILE_CODE","message":"failure"}}`))
	})
	client := newTestClient(t, startUnixHTTPServer(t, handler), "wsl-test-runner")
	if _, err := client.ResolveWorkspace(context.Background(), "/tmp/project"); !errors.Is(err, ErrHostProtocol) {
		t.Fatalf("error = %v, want %v", err, ErrHostProtocol)
	}
}

func TestNewClientRejectsUnsafeConfiguration(t *testing.T) {
	for _, config := range []ClientConfig{
		{SocketPath: "relative.sock", Token: testToken, ExpectedRunnerID: "wsl-test-runner"},
		{SocketPath: "/tmp/agent-host.sock\nprivate", Token: testToken, ExpectedRunnerID: "wsl-test-runner"},
		{SocketPath: "/tmp/agent-host.sock", Token: "short", ExpectedRunnerID: "wsl-test-runner"},
		{SocketPath: "/tmp/agent-host.sock", Token: testToken, ExpectedRunnerID: "INVALID"},
	} {
		if _, err := NewClient(config); err == nil {
			t.Fatalf("NewClient(%+v) succeeded", config)
		}
	}
}
