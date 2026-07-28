package usermemory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerMemoryCRUDAndSettings(t *testing.T) {
	repo := &fakeRepository{}
	handler := NewHandler(NewService(repo))

	settingsResponse := serveMemoryRequest(
		t,
		handler,
		http.MethodGet,
		memorySettingsPath,
		"",
	)
	if settingsResponse.Code != http.StatusOK ||
		!strings.Contains(settingsResponse.Body.String(), `"searchEnabled":true`) ||
		!strings.Contains(settingsResponse.Body.String(), `"enabled":false`) {
		t.Fatalf("default settings response = %d %s", settingsResponse.Code, settingsResponse.Body.String())
	}

	settingsResponse = serveMemoryRequest(
		t,
		handler,
		http.MethodPatch,
		memorySettingsPath,
		`{"enabled":true,"autoRecordEnabled":true}`,
	)
	if settingsResponse.Code != http.StatusOK ||
		!strings.Contains(settingsResponse.Body.String(), `"autoRecordEnabled":true`) {
		t.Fatalf("updated settings response = %d %s", settingsResponse.Code, settingsResponse.Body.String())
	}

	createdResponse := serveMemoryRequest(
		t,
		handler,
		http.MethodPost,
		memoriesPath,
		`{"type":"preference","content":"Keep answers concise","importance":4,"tags":["Style"]}`,
	)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create response = %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created MemoryDTO
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Source != "manual" || created.Content != "Keep answers concise" {
		t.Fatalf("created memory = %#v", created)
	}

	updatedResponse := serveMemoryRequest(
		t,
		handler,
		http.MethodPatch,
		memoriesPath+"/"+created.ID,
		`{"type":"preference","content":"Keep every answer concise","importance":5,"tags":["style"]}`,
	)
	if updatedResponse.Code != http.StatusOK ||
		!strings.Contains(updatedResponse.Body.String(), "Keep every answer concise") {
		t.Fatalf("update response = %d %s", updatedResponse.Code, updatedResponse.Body.String())
	}

	listResponse := serveMemoryRequest(t, handler, http.MethodGet, memoriesPath, "")
	if listResponse.Code != http.StatusOK ||
		!strings.Contains(listResponse.Body.String(), created.ID) {
		t.Fatalf("list response = %d %s", listResponse.Code, listResponse.Body.String())
	}

	deletedResponse := serveMemoryRequest(
		t,
		handler,
		http.MethodDelete,
		memoriesPath+"/"+created.ID,
		"",
	)
	if deletedResponse.Code != http.StatusNoContent {
		t.Fatalf("delete response = %d %s", deletedResponse.Code, deletedResponse.Body.String())
	}
	listResponse = serveMemoryRequest(t, handler, http.MethodGet, memoriesPath, "")
	if strings.Contains(listResponse.Body.String(), created.ID) {
		t.Fatalf("deleted memory remained visible: %s", listResponse.Body.String())
	}
}

func TestHandlerRejectsUnknownFieldsAndInvalidIDs(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{}))
	response := serveMemoryRequest(
		t,
		handler,
		http.MethodPost,
		memoriesPath,
		`{"type":"fact","content":"value","secret":"no"}`,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field status = %d body=%s", response.Code, response.Body.String())
	}
	response = serveMemoryRequest(
		t,
		handler,
		http.MethodDelete,
		memoriesPath+"/not-a-uuid",
		"",
	)
	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "INVALID_MEMORY_ID") {
		t.Fatalf("invalid id response = %d %s", response.Code, response.Body.String())
	}
}

func TestHandlerMemoryActivityPollingUsageAndUndo(t *testing.T) {
	now := time.Now().UTC()
	repo := &handlerActionRepository{
		fakeRepository: &fakeRepository{},
		activities: []MemoryActivity{{
			ID:                 "41000000-0000-4000-8000-000000000001",
			AssistantMessageID: "31000000-0000-4000-8000-000000000001",
			Ordinal:            1,
			SubjectType:        "memory",
			SubjectID:          "51000000-0000-4000-8000-000000000001",
			Action:             "created", Status: "completed", ReasonCode: "DIRECT_CREATED",
			UndoKind: "created", UndoStatus: "available",
			MemoryType: "preference", MemoryContent: "Keep replies concise",
			CreatedAt: now, UpdatedAt: now,
		}},
		usages: []MessageMemoryUsage{{
			AssistantMessageID: "31000000-0000-4000-8000-000000000001",
			Ordinal:            1,
			MemoryID:           "51000000-0000-4000-8000-000000000001",
			MemoryRevision:     1,
			ScopeType:          "global",
			MemoryDeleted:      true,
			CreatedAt:          now,
		}},
	}
	handler := NewHandler(NewService(repo))

	response := serveMemoryRequest(
		t, handler, http.MethodGet, memoryActivitiesPath+"?limit=1", "",
	)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"action":"created"`) ||
		!strings.Contains(response.Body.String(), `"nextCursor":"41000000-0000-4000-8000-000000000001"`) {
		t.Fatalf("activities response = %d %s", response.Code, response.Body.String())
	}
	response = serveMemoryRequest(
		t, handler, http.MethodGet,
		memoryUsagesPath+"?assistantMessageId=31000000-0000-4000-8000-000000000001", "",
	)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"memoryDeleted":true`) ||
		strings.Contains(response.Body.String(), "Keep replies concise") {
		t.Fatalf("usages response = %d %s", response.Code, response.Body.String())
	}
	response = serveMemoryRequest(
		t, handler, http.MethodPost,
		memoryActivitiesPath+"/41000000-0000-4000-8000-000000000001/undo",
		`{"expectedRevision":1}`,
	)
	if response.Code != http.StatusOK || repo.undo.ActivityID == "" ||
		repo.undo.ExpectedRevision != 1 {
		t.Fatalf("undo response = %d %s input=%#v", response.Code, response.Body.String(), repo.undo)
	}
}

type handlerActionRepository struct {
	*fakeRepository
	activities []MemoryActivity
	usages     []MessageMemoryUsage
	undo       UndoActivityInput
}

func (r *handlerActionRepository) HydrateDirectAction(
	context.Context, DirectActionHydrationInput,
) (DirectActionContext, error) {
	return DirectActionContext{}, nil
}

func (r *handlerActionRepository) ApplyDirectAction(
	context.Context, DirectActionApplyInput,
) (DirectActionResult, error) {
	return DirectActionResult{}, nil
}

func (r *handlerActionRepository) ListActivities(
	context.Context, string, int,
) ([]MemoryActivity, error) {
	return append([]MemoryActivity(nil), r.activities...), nil
}

func (r *handlerActionRepository) ListMessageUsages(
	context.Context, string,
) ([]MessageMemoryUsage, error) {
	return append([]MessageMemoryUsage(nil), r.usages...), nil
}

func (r *handlerActionRepository) UndoActivity(
	_ context.Context,
	input UndoActivityInput,
) (UndoActivityResult, error) {
	r.undo = input
	return UndoActivityResult{
		Status: "undone", ResultCode: "UNDO_APPLIED",
		MemoryID: "51000000-0000-4000-8000-000000000001", MemoryRevision: 2,
	}, nil
}

func serveMemoryRequest(
	t *testing.T,
	handler http.Handler,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	handler.ServeHTTP(response, request)
	return response
}
