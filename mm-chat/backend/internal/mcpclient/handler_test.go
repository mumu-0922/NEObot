package mcpclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"neo-chat/mm-chat/backend/internal/auth"
)

func TestViewServerEncodesEmptyToolsAsArray(t *testing.T) {
	encoded, err := json.Marshal(serverResponse{Server: viewServer(Server{
		Ref:       ServerRef{Source: SourcePrivate, ID: "server-id"},
		Name:      "Draft",
		Transport: TransportStreamableHTTP,
		AuthType:  AuthNone,
		Status:    ServerStatusDraft,
	})})
	if err != nil {
		t.Fatalf("marshal draft server response: %v", err)
	}
	if string(encoded) == "" || !jsonContainsEmptyToolsArray(encoded) {
		t.Fatalf("draft server response = %s, want tools array", encoded)
	}
}

func TestViewServerHidesPrivateRunnerEndpoint(t *testing.T) {
	encoded, err := json.Marshal(serverResponse{Server: viewServer(Server{
		Ref: ServerRef{Source: SourcePrivate, ID: "server-id"}, Name: "Context7",
		Icon:      "https://github.com/upstash.png",
		Transport: TransportStdio, EndpointURL: "runner://context7-artifact",
		AuthType: AuthNone, Status: ServerStatusReady,
		Metadata: map[string]any{"runnerArtifactId": "context7-artifact"},
	})})
	if err != nil {
		t.Fatalf("marshal Runner server response: %v", err)
	}
	if bytes.Contains(encoded, []byte("runner://")) || bytes.Contains(encoded, []byte("runnerArtifactId")) {
		t.Fatalf("Runner internals leaked in response: %s", encoded)
	}
	if !bytes.Contains(encoded, []byte(`"icon":"https://github.com/upstash.png"`)) {
		t.Fatalf("normalized icon missing from response: %s", encoded)
	}
}

func TestViewServerOmitsUnsafeIcon(t *testing.T) {
	encoded, err := json.Marshal(serverResponse{Server: viewServer(Server{
		Ref:       ServerRef{Source: SourcePrivate, ID: "server-id"},
		Name:      "Unsafe icon",
		Icon:      "http://127.0.0.1/icon.png",
		Transport: TransportStreamableHTTP,
		AuthType:  AuthNone,
		Status:    ServerStatusReady,
	})})
	if err != nil {
		t.Fatalf("marshal server response: %v", err)
	}
	if bytes.Contains(encoded, []byte(`"icon"`)) {
		t.Fatalf("unsafe icon leaked in response: %s", encoded)
	}
}

func TestSelectionViewsEncodeEmptyCollectionsAsArrays(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{
			name: "conversation without servers",
			value: viewConversationSelection(Selection{
				ConversationID: "conversation-id",
				Mode:           SelectionModeCustom,
			}),
		},
		{
			name: "workspace without servers",
			value: viewWorkspaceSelection(WorkspaceSelection{
				WorkspaceID: "workspace-id",
			}),
		},
		{
			name: "server without disabled tools",
			value: viewConversationSelection(Selection{
				ConversationID: "conversation-id",
				Mode:           SelectionModeCustom,
				Servers: []SelectionServer{{
					Ref: ServerRef{Source: SourcePrivate, ID: "server-id"},
				}},
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("marshal selection view: %v", err)
			}
			if !jsonSelectionCollectionsAreArrays(encoded) {
				t.Fatalf("selection response = %s, want array collections", encoded)
			}
		})
	}
}

func TestWriteMCPServiceErrorMapsDuplicateServerToConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeMCPServiceError(recorder, ErrServerConflict)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != "MCP_CONFLICT" {
		t.Fatalf("error code = %q, want MCP_CONFLICT", body.Error.Code)
	}
}

func TestWriteMCPServiceErrorMapsAdministratorBoundary(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeMCPServiceError(recorder, ErrAdministratorRequired)
	if recorder.Code != http.StatusForbidden || !bytes.Contains(recorder.Body.Bytes(), []byte("MCP_ADMIN_REQUIRED")) {
		t.Fatalf("administrator error status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerRejectsNonAdministratorServerCreation(t *testing.T) {
	administratorID := "00000000-0000-0000-0000-000000000001"
	ordinaryID := "00000000-0000-0000-0000-000000000009"
	config := testMCPConfig()
	config.AdministratorUserID = administratorID
	service, err := NewService(config, newFakeRepository(), nil, nil, nil, Catalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(service)
	request := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", bytes.NewBufferString(
		`{"name":"blocked","endpointUrl":"https://mcp.example.com/mcp","authType":"none"}`,
	))
	request = request.WithContext(auth.WithUser(request.Context(), auth.User{ID: ordinaryID}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !bytes.Contains(recorder.Body.Bytes(), []byte("MCP_ADMIN_REQUIRED")) {
		t.Fatalf("ordinary create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestMCPServerEndpointConflictMatchesOnlyOwningConstraint(t *testing.T) {
	if !isMCPServerEndpointConflict(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "idx_mcp_servers_user_endpoint_active",
	}) {
		t.Fatal("owning endpoint constraint was not recognized")
	}
	if isMCPServerEndpointConflict(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "another_constraint",
	}) {
		t.Fatal("unrelated unique constraint was recognized")
	}
}

func jsonContainsEmptyToolsArray(encoded []byte) bool {
	var body struct {
		Server struct {
			Tools []Tool `json:"tools"`
		} `json:"server"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		return false
	}
	return body.Server.Tools != nil && len(body.Server.Tools) == 0
}

func jsonSelectionCollectionsAreArrays(encoded []byte) bool {
	var body struct {
		Servers []struct {
			DisabledTools []string `json:"disabledTools"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil || body.Servers == nil {
		return false
	}
	for _, server := range body.Servers {
		if server.DisabledTools == nil {
			return false
		}
	}
	return true
}
