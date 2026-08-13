package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestHandlerListsNormalizedAgentsFromRegistry(t *testing.T) {
	var requestedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"agents":[
				{"identifier":"agent-1","meta":{"title":" Agent One ","description":"Useful","tags":["tools","tools"],"category":"Productivity"},"author":" Team "},
				{"identifier":"bad/agent","meta":{"title":"Bad"}}
			]
		}`))
	}))
	defer upstream.Close()

	handler := NewHandler(NewService(WithRegistryBaseURL(upstream.URL)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/agents?locale=zh-CN", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if requestedPath != "/index.zh-CN.json" {
		t.Fatalf("upstream path = %q, want /index.zh-CN.json", requestedPath)
	}
	var response ListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Unavailable || len(response.Agents) != 1 {
		t.Fatalf("response = %#v", response)
	}
	agent := response.Agents[0]
	if agent.Identifier != "agent-1" || agent.Meta.Title != "Agent One" || agent.Meta.Tags[0] != "tools" || agent.Author != "Team" {
		t.Fatalf("agent = %#v", agent)
	}
}

func TestHandlerAgentListDegradesWhenRegistryUnavailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	defer upstream.Close()

	handler := NewHandler(NewService(WithRegistryBaseURL(upstream.URL)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/agents?locale=en", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	var response ListResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Unavailable || len(response.Agents) != 0 {
		t.Fatalf("response = %#v", response)
	}
}

func TestHandlerGetsLocalizedAgentDetail(t *testing.T) {
	var requestedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"identifier":"different",
			"meta":{"title":" Detail ","description":"Full"},
			"config":{"systemRole":" System prompt "}
		}`))
	}))
	defer upstream.Close()

	handler := NewHandler(NewService(WithRegistryBaseURL(upstream.URL)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/agents/agent-1?locale=ja-JP", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	if requestedPath != "/agent-1.ja-JP.json" {
		t.Fatalf("upstream path = %q, want /agent-1.ja-JP.json", requestedPath)
	}
	var agent Agent
	if err := json.NewDecoder(recorder.Body).Decode(&agent); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if agent.Identifier != "agent-1" || agent.Meta.Title != "Detail" || agent.Meta.SystemRole != "System prompt" || agent.Config.SystemRole != "System prompt" {
		t.Fatalf("agent = %#v", agent)
	}
}

func TestHandlerRejectsInvalidAgentIdentifiers(t *testing.T) {
	handler := NewHandler(NewService(WithHTTPClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("upstream should not be called for invalid identifiers")
		return nil, nil
	}))))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/agents/bad$agent", nil)

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "INVALID_AGENT_IDENTIFIER") {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestAssistantHandlerCopyUsesSourceRevisionCAS(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(WithRepository(repo))
	snapshot, err := normalizeCustomSnapshot(CreateCustomInput{Title: "Writer", SystemPrompt: "Write carefully."})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SourceIdentifier = "writer"
	snapshot.ContentFingerprint = fingerprintSnapshot(snapshot)
	entry := repo.create(SourceLobeHub, snapshot)
	handler := NewHandler(service)

	stale := httptest.NewRecorder()
	handler.ServeHTTP(stale, httptest.NewRequest(http.MethodPost,
		"/v1/assistants/library/"+entry.ID+"/copy",
		strings.NewReader(`{"expectedRevision":2}`)))
	if stale.Code != http.StatusConflict || !strings.Contains(stale.Body.String(), "ASSISTANT_REVISION_CONFLICT") {
		t.Fatalf("stale copy status=%d body=%s", stale.Code, stale.Body.String())
	}
	if len(repo.library) != 1 {
		t.Fatal("stale copy created a library entry")
	}

	current := httptest.NewRecorder()
	handler.ServeHTTP(current, httptest.NewRequest(http.MethodPost,
		"/v1/assistants/library/"+entry.ID+"/copy",
		strings.NewReader(`{"expectedRevision":1}`)))
	if current.Code != http.StatusCreated || len(repo.library) != 2 {
		t.Fatalf("copy status=%d body=%s library=%#v", current.Code, current.Body.String(), repo.library)
	}
}

func TestAssistantHandlerRejectsUnknownJSONFields(t *testing.T) {
	handler := NewHandler(NewService(WithRepository(newFakeRepository())))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/assistants/library",
		strings.NewReader(`{"title":"Writer","systemPrompt":"Write.","enableTools":true}`))

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "INVALID_JSON") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAssistantHandlerReviewRequiresConfiguredAdministrator(t *testing.T) {
	handler := NewHandler(NewService(WithRepository(newFakeRepository()), WithAdministratorUserID(adminUserID)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/v1/assistants/market/items/writer/review?locale=en",
		strings.NewReader(`{"status":"admitted","fingerprint":"`+strings.Repeat("a", 64)+`"}`))
	request = request.WithContext(auth.WithUser(context.Background(), auth.User{ID: testUserID}))

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ASSISTANT_ADMIN_REQUIRED") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAssistantHandlerUsesActionSpecificStatusAndMethods(t *testing.T) {
	repo := newFakeRepository()
	service := NewService(WithRepository(repo))
	snapshot, _ := normalizeCustomSnapshot(CreateCustomInput{Title: "Writer", SystemPrompt: "Write."})
	snapshot.SourceIdentifier = "writer"
	snapshot.ContentFingerprint = fingerprintSnapshot(snapshot)
	repo.admissions["writer"] = Admission{Snapshot: snapshot, Status: AdmissionAdmitted}
	entry := repo.create(SourceLobeHub, snapshot)
	handler := NewHandler(service)

	updated := httptest.NewRecorder()
	handler.ServeHTTP(updated, httptest.NewRequest(http.MethodPost,
		"/v1/assistants/library/"+entry.ID+"/update", strings.NewReader(`{"expectedRevision":1}`)))
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}

	wrongMethod := httptest.NewRecorder()
	handler.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodGet,
		"/v1/assistants/library/"+entry.ID+"/copy", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("method status=%d allow=%q", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return fn(request)
}
