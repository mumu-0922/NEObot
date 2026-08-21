package hostworkspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
)

func workspaceRequest(method, path, body string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return request
}

func TestHandlerImportsAndListsLegacyWorkspace(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(repo, nil)
	service.now = func() time.Time { return time.Date(2026, 8, 21, 8, 0, 0, 0, time.UTC) }
	handler := NewHandler(service)

	input := `{"name":"Neo Chat","systemPrompt":"safe","files":[],"color":"blue","enableSearch":true}`
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, workspaceRequest(http.MethodPut, workspacePathPrefix+testWorkspaceID, input))
	if recorder.Code != http.StatusOK {
		t.Fatalf("import status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var imported map[string]workspaceDTO
	if err := json.Unmarshal(recorder.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	if imported["workspace"].BindingStatus != "unbound" || imported["workspace"].ID != testWorkspaceID {
		t.Fatalf("unexpected import: %+v", imported)
	}

	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, workspaceRequest(http.MethodGet, workspacePath, ""))
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(testWorkspaceID)) {
		t.Fatalf("list status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerRejectsMalformedWorkspaceJSON(t *testing.T) {
	handler := NewHandler(NewService(newFakeRepository(), nil))
	for _, body := range []string{
		`{"name":"test","files":[],"unknown":true}`,
		`{"name":"test","name":"duplicate","files":[]}`,
		`{"name":"test","files":[]} {}`,
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, workspaceRequest(http.MethodPut, workspacePathPrefix+testWorkspaceID, body))
		if recorder.Code != http.StatusBadRequest || strings.Contains(recorder.Body.String(), body) {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestHandlerBindFailsClosedWhenRunnerUnavailable(t *testing.T) {
	repo := newFakeRepository()
	repo.items[testWorkspaceID] = Workspace{ID: testWorkspaceID, Revision: 1, Settings: validSettings()}
	handler := NewHandler(NewService(repo, nil))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, workspaceRequest(
		http.MethodPost, workspacePathPrefix+testWorkspaceID+"/bind",
		`{"expectedRevision":1,"path":"/home/private/project"}`,
	))
	if recorder.Code != http.StatusServiceUnavailable ||
		strings.Contains(recorder.Body.String(), "/home/private/project") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerSanitizesHostProtocolFailure(t *testing.T) {
	repo := newFakeRepository()
	repo.items[testWorkspaceID] = Workspace{ID: testWorkspaceID, Revision: 1, Settings: validSettings()}
	handler := NewHandler(NewService(repo, fakeResolver{
		runnerID: "wsl-test-runner",
		err:      agenthost.ErrHostProtocol,
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, workspaceRequest(
		http.MethodPost, workspacePathPrefix+testWorkspaceID+"/bind",
		`{"expectedRevision":1,"path":"/home/private/project"}`,
	))
	if recorder.Code != http.StatusBadGateway ||
		!strings.Contains(recorder.Body.String(), "HOST_WORKSPACE_PROTOCOL_INVALID") ||
		strings.Contains(recorder.Body.String(), "/home/private/project") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerConversationBindingRoute(t *testing.T) {
	repo := newFakeRepository()
	handler := NewHandler(NewService(repo, nil))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, workspaceRequest(
		http.MethodPut,
		workspacePathPrefix+testWorkspaceID+"/conversations/"+testConversationID,
		"",
	))
	if recorder.Code != http.StatusNoContent || repo.setCalls != 1 {
		t.Fatalf("status = %d, setCalls = %d", recorder.Code, repo.setCalls)
	}
}
