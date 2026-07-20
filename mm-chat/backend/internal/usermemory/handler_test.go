package usermemory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
