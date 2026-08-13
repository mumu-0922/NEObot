package mcprunner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"

	"neo-chat/mm-chat/backend/internal/mcpclient"
)

const helperEnvironment = "MM_CHAT_MCP_RUNNER_TEST_HELPER"
const helperDescendantPIDFile = "MM_CHAT_MCP_RUNNER_DESCENDANT_PID_FILE"
const playwrightBrowsersPath = "/ms-playwright"

func TestMain(m *testing.M) {
	if os.Getenv(helperEnvironment) == "1" {
		runHelperServer()
		return
	}
	os.Exit(m.Run())
}

func runHelperServer() {
	server := protocol.NewServer(&protocol.Implementation{Name: "runner-test", Version: "1"}, nil)
	server.AddTool(&protocol.Tool{
		Name: "echo",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"value": map[string]any{"type": "string"}},
		},
	}, func(_ context.Context, request *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
		var arguments map[string]any
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		value, _ := arguments["value"].(string)
		return &protocol.CallToolResult{Content: []protocol.Content{&protocol.TextContent{Text: value}}}, nil
	})
	server.AddTool(&protocol.Tool{Name: "crash", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
			os.Exit(23)
			return nil, nil
		})
	server.AddTool(&protocol.Tool{Name: "hang", InputSchema: map[string]any{"type": "object"}},
		func(ctx context.Context, _ *protocol.CallToolRequest) (*protocol.CallToolResult, error) {
			child := exec.Command("sleep", "300")
			if err := child.Start(); err != nil {
				return nil, err
			}
			if path := os.Getenv(helperDescendantPIDFile); path != "" {
				if err := os.WriteFile(path, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
					return nil, err
				}
			}
			<-ctx.Done()
			return nil, ctx.Err()
		})
	if err := server.Run(context.Background(), &protocol.StdioTransport{}); err != nil {
		log.Print(err)
	}
}

func TestManagerStartsApprovedServerOnDemandAndReapsIt(t *testing.T) {
	server := helperManifestServer(t, "one")
	manager, err := NewManager(Config{
		MaxProcesses: 1, WorkRoot: t.TempDir(), ReapInterval: 10 * time.Millisecond,
	}, []mcpclient.Server{server})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if len(manager.sessions) != 0 {
		t.Fatal("server process started eagerly")
	}
	tools, err := manager.ListTools(context.Background(), "one")
	if err != nil || len(tools) != 3 || tools[0].Name != "crash" || tools[1].Name != "echo" || tools[2].Name != "hang" {
		t.Fatalf("tools=%#v err=%v", tools, err)
	}
	result, err := manager.CallTool(context.Background(), "one", "echo", map[string]any{"value": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Content[0].(*protocol.TextContent).Text; got != "hello" {
		t.Fatalf("result=%q", got)
	}
	if len(manager.sessions) != 1 {
		t.Fatalf("sessions=%d", len(manager.sessions))
	}
	if got := environmentValue(manager.sessions["one"].command.Env, "PLAYWRIGHT_BROWSERS_PATH"); got != playwrightBrowsersPath {
		t.Fatalf("PLAYWRIGHT_BROWSERS_PATH=%q, want %q", got, playwrightBrowsersPath)
	}
	manager.reap(time.Now().Add(time.Minute))
	if len(manager.sessions) != 0 {
		t.Fatalf("idle session not reaped: %d", len(manager.sessions))
	}
}

func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func TestManagerRestartsPrivateInstanceWhenSecretEnvironmentChanges(t *testing.T) {
	server := helperManifestServer(t, "secret-instance")
	server.Command.UserSecretEnv = []string{"TEST_API_KEY"}
	manager, err := NewManager(Config{MaxProcesses: 1, WorkRoot: t.TempDir()}, []mcpclient.Server{server})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	options := instanceOptions{InstanceID: "00000000-0000-0000-0000-000000000001", Environment: map[string]string{"TEST_API_KEY": "first"}}
	if _, err := manager.ListTools(context.Background(), server.Ref.ID, options); err != nil {
		t.Fatal(err)
	}
	first := manager.sessions[options.InstanceID]
	options.Environment["TEST_API_KEY"] = "second"
	if _, err := manager.ListTools(context.Background(), server.Ref.ID, options); err != nil {
		t.Fatal(err)
	}
	second := manager.sessions[options.InstanceID]
	if first == second || first.environmentFingerprint == second.environmentFingerprint {
		t.Fatal("secret change reused old Runner process")
	}
}

func TestManagerRecoversAfterApprovedServerCrash(t *testing.T) {
	manager, err := NewManager(Config{MaxProcesses: 1, WorkRoot: t.TempDir()}, []mcpclient.Server{helperManifestServer(t, "crashable")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	if _, err := manager.CallTool(context.Background(), "crashable", "crash", map[string]any{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("crash error = %v, want ErrUnavailable", err)
	}
	if len(manager.sessions) != 0 {
		t.Fatalf("crashed session retained: %d", len(manager.sessions))
	}
	result, err := manager.CallTool(context.Background(), "crashable", "echo", map[string]any{"value": "recovered"})
	if err != nil {
		t.Fatalf("recovery call: %v", err)
	}
	if got := result.Content[0].(*protocol.TextContent).Text; got != "recovered" {
		t.Fatalf("recovery result = %q", got)
	}
}

func TestManagerCancellationKillsServerProcessGroupDescendants(t *testing.T) {
	pidFile := t.TempDir() + "/descendant.pid"
	server := helperManifestServer(t, "cancelable")
	server.Command.Env[helperDescendantPIDFile] = pidFile
	manager, err := NewManager(Config{MaxProcesses: 1, WorkRoot: t.TempDir()}, []mcpclient.Server{server})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := manager.CallTool(ctx, "cancelable", "hang", map[string]any{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel error = %v, want deadline exceeded", err)
	}
	if len(manager.sessions) != 0 {
		t.Fatalf("canceled session retained: %d", len(manager.sessions))
	}
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("descendant pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		t.Fatalf("descendant pid value: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant process %d survived process-group cancellation: %v", pid, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRunnerConnectorCompletesStdioProtocolThroughPrivateAPI(t *testing.T) {
	server := helperManifestServer(t, "connector")
	manager, err := NewManager(Config{MaxProcesses: 1, WorkRoot: t.TempDir()}, []mcpclient.Server{server})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	token := "runner-connector-token-with-thirty-two-bytes"
	handler, err := NewHandler(manager, token)
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	connector, err := mcpclient.NewRunnerConnector(httpServer.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.Connect(context.Background(), server, "")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := session.ListTools(context.Background())
	if err != nil || len(tools) != 3 {
		t.Fatalf("connector tools = %#v, err=%v", tools, err)
	}
	result, err := session.CallTool(context.Background(), "echo", map[string]any{"value": "through-runner"})
	if err != nil || len(result.Content) != 1 || result.Content[0].Text != "through-runner" {
		t.Fatalf("connector result = %#v, err=%v", result, err)
	}
}

func TestHandlerRequiresIndependentBearerAndRejectsUnknownFields(t *testing.T) {
	manager, err := NewManager(Config{MaxProcesses: 1, WorkRoot: t.TempDir()}, []mcpclient.Server{helperManifestServer(t, "one")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	token := "runner-token-with-at-least-thirty-two-bytes"
	handler, err := NewHandler(manager, token)
	if err != nil {
		t.Fatal(err)
	}

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/internal/v1/tools/list", bytes.NewBufferString(`{"serverId":"one"}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}

	invalid := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/tools/list", bytes.NewBufferString(`{"serverId":"one","command":"/bin/sh"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(invalid, request)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status=%d body=%s", invalid.Code, invalid.Body.String())
	}

	valid := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/internal/v1/tools/list", bytes.NewBufferString(`{"serverId":"one"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(valid, request)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid status=%d body=%s", valid.Code, valid.Body.String())
	}
}

func TestDynamicServerRejectsNonRegistryPackageSpecs(t *testing.T) {
	for _, packageSpec := range []string{
		"https://example.com/server.tgz@1.0.0", "git+https://example.com/repo@1.0.0",
		"file:../server@1.0.0", "package", "package@latest",
	} {
		if _, err := dynamicServer("npm-artifact", mcpclient.DynamicRunnerArtifact{
			ID: "npm-artifact", PackageSpec: packageSpec, IdleSeconds: 60, LifetimeSeconds: 120,
		}); !errors.Is(err, ErrServerNotApproved) {
			t.Fatalf("dynamicServer(%q) error=%v", packageSpec, err)
		}
	}
	server, err := dynamicServer("npm-artifact", mcpclient.DynamicRunnerArtifact{
		ID: "npm-artifact", PackageSpec: "@scope/package@1.2.3", IdleSeconds: 60, LifetimeSeconds: 120,
	})
	if err != nil || server.Command == nil || server.Command.Argv[0] != "/usr/local/bin/npx" {
		t.Fatalf("valid dynamic server=%#v error=%v", server, err)
	}
}

func helperManifestServer(t *testing.T, id string) mcpclient.Server {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return mcpclient.Server{
		Ref:       mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: id},
		Transport: mcpclient.TransportStdio,
		Command: &mcpclient.Command{
			Argv: []string{executable}, Env: map[string]string{helperEnvironment: "1"},
			IdleTimeout: 20 * time.Millisecond, MaxLifetime: time.Minute,
		},
	}
}
