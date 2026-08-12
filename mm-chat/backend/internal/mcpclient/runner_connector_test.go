package mcpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunnerConnectorRoutesPrivateServerToApprovedArtifactID(t *testing.T) {
	t.Parallel()
	const artifactID = "marketplace-upstash-context7-2.2.0"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
		_, _ = writer.Write([]byte(`{"tools":[]}`))
	}))
	defer server.Close()

	connector, err := NewRunnerConnector(server.URL, strings.Repeat("t", 32))
	if err != nil {
		t.Fatal(err)
	}
	session, err := connector.Connect(context.Background(), Server{
		Ref: ServerRef{Source: SourcePrivate, ID: "private-id"}, Transport: TransportStdio,
		Metadata: map[string]any{"runnerArtifactId": artifactID},
	}, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	tools, err := session.ListTools(context.Background())
	if err != nil || len(tools) != 0 {
		t.Fatalf("ListTools() tools=%#v error=%v", tools, err)
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
