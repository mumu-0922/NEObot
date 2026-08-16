package mcpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRunnerConnectorRoutesPrivateServerToApprovedArtifactID(t *testing.T) {
	t.Parallel()
	const artifactID = "marketplace-upstash-context7-2.2.0"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/internal/v1/tools/list" ||
			request.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 32) {
			t.Fatalf("request path/auth = %q/%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body struct {
			ServerID string `json:"serverId"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil || body.ServerID != artifactID {
			t.Fatalf("request body = %#v error=%v", body, err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tools":[
		  {"name":"query-docs","inputSchema":{"type":"object"}},
		  {"name":"run-code-unsafe","inputSchema":{"type":"object"}}
		]}`))
	}))
	defer server.Close()

	connector, err := NewRunnerConnector(server.URL, strings.Repeat("t", 32))
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.Connect(context.Background(), Server{
		Ref: ServerRef{Source: SourcePrivate, ID: "private-id"}, Transport: TransportStdio,
		Metadata: map[string]any{
			"runnerArtifactId":   artifactID,
			manifestAllowedTools: []string{"query-docs"},
		},
	}, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	tools, err := session.ListTools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "query-docs" {
		t.Fatalf("ListTools() tools=%#v error=%v", tools, err)
	}
	if _, err := session.CallTool(
		context.Background(), "run-code-unsafe", map[string]any{"code": "process.exit()"},
	); err != ErrToolNotFound {
		t.Fatalf("unsafe private CallTool error = %v, want ErrToolNotFound", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("unsafe private CallTool reached Runner; requests=%d", requests.Load())
	}
}

func TestRunnerConnectorRejectsPrivateServerWithoutArtifactID(t *testing.T) {
	t.Parallel()
	connector, err := NewRunnerConnector("http://runner:8090", strings.Repeat("t", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connector.Connect(context.Background(), Server{
		Ref: ServerRef{Source: SourcePrivate, ID: "private-id"}, Transport: TransportStdio,
	}, ""); err != ErrServerUnavailable {
		t.Fatalf("Connect() error = %v, want ErrServerUnavailable", err)
	}
}

func TestRunnerConnectorUsesRunInstanceAndFiltersManifestTools(t *testing.T) {
	t.Parallel()
	const instanceID = "mcp-run-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		var body struct {
			ServerID   string `json:"serverId"`
			InstanceID string `json:"instanceId"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil ||
			body.ServerID != "browser" || body.InstanceID != instanceID {
			t.Fatalf("Runner route = %#v error=%v", body, err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"tools":[
		  {"name":"browser_snapshot","description":"safe","inputSchema":{"type":"object"}},
		  {"name":"browser_run_code_unsafe","description":"unsafe","inputSchema":{"type":"object"}}
		]}`))
	}))
	defer server.Close()

	connector, err := NewRunnerConnector(server.URL, strings.Repeat("t", 32))
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.Connect(context.Background(), Server{
		Ref:       ServerRef{Source: SourceManifest, ID: "browser"},
		Transport: TransportStdio, Command: &Command{Argv: []string{"/opt/browser"}},
		Metadata: map[string]any{
			runnerInstanceID:     instanceID,
			manifestAllowedTools: []string{"browser_snapshot"},
			"toolPolicy":         map[string]string{"browser_snapshot": ClassificationRead},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := session.ListTools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "browser_snapshot" ||
		tools[0].Classification != ClassificationRead {
		t.Fatalf("filtered Runner Tools = %#v error=%v", tools, err)
	}
	if _, err := session.CallTool(
		context.Background(), "browser_run_code_unsafe", map[string]any{"code": "process.exit()"},
	); err != ErrToolNotFound {
		t.Fatalf("unsafe CallTool error = %v, want ErrToolNotFound", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("unsafe CallTool reached Runner; requests=%d", requests.Load())
	}
}
