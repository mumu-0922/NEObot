package hostworkspace

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/agenthost"
)

type interactiveResolver struct{ fakeResolver }

func (resolver interactiveResolver) Capabilities(context.Context) (agenthost.Capabilities, error) {
	return agenthost.Capabilities{
		ProtocolVersion: agenthost.ProtocolVersion,
		RunnerID:        resolver.runnerID,
		Platform:        "linux-wsl",
		Architecture:    "amd64",
		Features: agenthost.HostFeatures{
			WorkspaceResolve: true, DirectoryBrowse: true, NativeDirectoryPicker: true,
			WindowsPathInterop: true, PermissionModes: []agenthost.PermissionMode{},
		},
	}, nil
}

func (resolver interactiveResolver) BrowseDirectories(context.Context, string) (agenthost.DirectoryBrowseResponse, error) {
	return agenthost.DirectoryBrowseResponse{
		Path: "/home/user", DisplayPath: "/home/user", PathKind: "wsl",
		Entries: []agenthost.DirectoryEntry{{
			Name: "project", Path: "/home/user/project",
			DisplayPath: "/home/user/project", PathKind: "wsl",
		}},
	}, nil
}

func (resolver interactiveResolver) PickNativeDirectory(context.Context) (agenthost.NativeDirectoryPickResponse, error) {
	return agenthost.NativeDirectoryPickResponse{
		Workspace: &agenthost.WorkspaceDescriptor{
			CanonicalPath: "/mnt/d/project", DisplayPath: `D:\project`, PathKind: "windows-mounted",
		},
	}, nil
}

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
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, workspaceRequest(
		http.MethodDelete,
		workspacePathPrefix+testWorkspaceID+"/conversations/"+testConversationID,
		"",
	))
	if recorder.Code != http.StatusNoContent || repo.setCalls != 2 {
		t.Fatalf("clear status = %d, setCalls = %d", recorder.Code, repo.setCalls)
	}
}

func TestHandlerProjectsHostStatusBrowseAndNativePicker(t *testing.T) {
	handler := NewHandler(NewService(newFakeRepository(), interactiveResolver{
		fakeResolver: fakeResolver{runnerID: "wsl-test-runner"},
	}))

	statusRecorder := httptest.NewRecorder()
	handler.ServeHTTP(statusRecorder, workspaceRequest(http.MethodGet, workspacePathPrefix+"host-status", ""))
	if statusRecorder.Code != http.StatusOK ||
		!strings.Contains(statusRecorder.Body.String(), `"status":"ready"`) ||
		!strings.Contains(statusRecorder.Body.String(), `"directoryBrowse":true`) {
		t.Fatalf("status = %d, body = %s", statusRecorder.Code, statusRecorder.Body.String())
	}

	browseRecorder := httptest.NewRecorder()
	handler.ServeHTTP(browseRecorder, workspaceRequest(
		http.MethodPost, workspacePathPrefix+"directories/browse", `{"path":"/home/user"}`,
	))
	if browseRecorder.Code != http.StatusOK ||
		!strings.Contains(browseRecorder.Body.String(), `"name":"project"`) {
		t.Fatalf("browse = %d, body = %s", browseRecorder.Code, browseRecorder.Body.String())
	}

	pickerRecorder := httptest.NewRecorder()
	handler.ServeHTTP(pickerRecorder, workspaceRequest(
		http.MethodPost, workspacePathPrefix+"directories/pick-native", `{}`,
	))
	if pickerRecorder.Code != http.StatusOK ||
		!strings.Contains(pickerRecorder.Body.String(), `"displayPath":"D:\\project"`) {
		t.Fatalf("picker = %d, body = %s", pickerRecorder.Code, pickerRecorder.Body.String())
	}
}

func TestHandlerServesWorkspaceFileContentWithVersionHeaders(t *testing.T) {
	repo := newFakeRepository()
	repo.items[testWorkspaceID] = Workspace{
		ID: testWorkspaceID, Revision: 2, Settings: validSettings(),
		RunnerID: "wsl-test-runner", CanonicalPath: "/home/user/project",
		DirectoryFingerprint: "sha256:" + strings.Repeat("a", 64),
	}
	version := "sha256:" + strings.Repeat("b", 64)
	resolver := &workspaceFileResolver{
		fakeResolver: fakeResolver{runnerID: "wsl-test-runner"},
		snapshot:     agentWorkspaceArtifact("reports/result.xlsx", []byte("xlsx"), version),
	}
	handler := NewHandler(NewService(repo, resolver))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, workspaceRequest(
		http.MethodGet,
		workspacePathPrefix+testWorkspaceID+"/files/content?path=reports%2Fresult.xlsx&download=true",
		"",
	))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "xlsx" ||
		recorder.Header().Get("ETag") != `"`+strings.Repeat("b", 64)+`"` ||
		!strings.HasPrefix(recorder.Header().Get("Content-Disposition"), "attachment;") ||
		recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status=%d headers=%#v body=%q", recorder.Code, recorder.Header(), recorder.Body.String())
	}
}

func TestHandlerRejectsInvalidWorkspaceFileQueryBeforeHost(t *testing.T) {
	resolver := &workspaceFileResolver{fakeResolver: fakeResolver{runnerID: "wsl-test-runner"}}
	handler := NewHandler(NewService(newFakeRepository(), resolver))
	for _, requestPath := range []string{
		workspacePathPrefix + testWorkspaceID + "/files/content?path=a.txt&extra=1",
		workspacePathPrefix + testWorkspaceID + "/files/preview?path=a.txt&download=true",
		workspacePathPrefix + testWorkspaceID + "/files/content?path=a.txt&path=b.txt",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, workspaceRequest(http.MethodGet, requestPath, ""))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("path=%q status=%d body=%s", requestPath, recorder.Code, recorder.Body.String())
		}
	}
	if len(resolver.requests) != 0 {
		t.Fatalf("invalid query reached Host: %#v", resolver.requests)
	}
}
