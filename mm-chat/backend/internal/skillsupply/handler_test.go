package skillsupply

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
)

func TestHandlerZIPReviewInstallAndLibraryLifecycle(t *testing.T) {
	repository := newMemoryRepository()
	objects := newMemoryObjectStore()
	service := NewService(WithRepository(repository), WithObjectStore(objects), WithAdministratorUserID(testSkillAdmin))
	service.newID = sequenceIDs("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	handler := NewHandler(service)
	archive := mustTestArchive(t, []packageFile{{path: "SKILL.md", data: []byte(validSkillMarkdown("handler-skill"))}}, 0, time.Time{})

	unauthorized := skillRequest(handler, http.MethodPost, "/v1/skills/candidates/zip",
		bytesReader(archive), "application/zip", testSkillUser)
	assertSkillHTTP(t, unauthorized, http.StatusForbidden, "SKILL_ADMIN_REQUIRED")

	created := skillRequest(handler, http.MethodPost, "/v1/skills/candidates/zip",
		bytesReader(archive), "application/zip", testSkillAdmin)
	assertSkillHTTP(t, created, http.StatusCreated, `"status":"validated"`)
	candidate := repository.candidates["aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"]

	unknown := skillRequest(handler, http.MethodPost, "/v1/skills/candidates/"+candidate.ID+"/review",
		strings.NewReader(`{"status":"admitted","expectedRevision":1,"packageFingerprint":"`+
			candidate.Package.PackageFingerprint+`","reason":"reviewed","grantTools":true}`),
		"application/json", testSkillAdmin)
	assertSkillHTTP(t, unknown, http.StatusBadRequest, "INVALID_JSON")

	review := skillRequest(handler, http.MethodPost, "/v1/skills/candidates/"+candidate.ID+"/review",
		strings.NewReader(`{"status":"admitted","expectedRevision":1,"packageFingerprint":"`+
			candidate.Package.PackageFingerprint+`","reason":"reviewed"}`),
		"application/json", testSkillAdmin)
	assertSkillHTTP(t, review, http.StatusOK, `"status":"admitted"`)
	changed := skillRequest(handler, http.MethodPost, "/v1/skills/store/items/"+candidate.ID+"/install",
		strings.NewReader(`{"packageFingerprint":"sha256:`+strings.Repeat("f", 64)+`"}`),
		"application/json", testSkillUser)
	assertSkillHTTP(t, changed, http.StatusConflict, "SKILL_PACKAGE_CHANGED")

	install := skillRequest(handler, http.MethodPost, "/v1/skills/store/items/"+candidate.ID+"/install",
		strings.NewReader(`{"packageFingerprint":"`+candidate.Package.PackageFingerprint+`"}`),
		"application/json", testSkillUser)
	assertSkillHTTP(t, install, http.StatusCreated, `"allowedTools":["Read","Search"]`)
	var installationID string
	for id := range repository.installations {
		installationID = id
	}
	library := skillRequest(handler, http.MethodGet, "/v1/skills/library", nil, "", testSkillUser)
	assertSkillHTTP(t, library, http.StatusOK, installationID)

	stale := skillRequest(handler, http.MethodDelete, "/v1/skills/library/"+installationID+"?revision=2",
		nil, "", testSkillUser)
	assertSkillHTTP(t, stale, http.StatusConflict, "SKILL_REVISION_CONFLICT")
	removed := skillRequest(handler, http.MethodDelete, "/v1/skills/library/"+installationID+"?revision=1",
		nil, "", testSkillUser)
	if removed.Code != http.StatusNoContent {
		t.Fatalf("uninstall status=%d body=%s", removed.Code, removed.Body.String())
	}
	if removed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", removed.Header().Get("Cache-Control"))
	}
}

func TestHandlerRejectsWrongMethodsMediaAndQueries(t *testing.T) {
	handler := NewHandler(NewService(WithRepository(newMemoryRepository()),
		WithObjectStore(newMemoryObjectStore()), WithAdministratorUserID(testSkillAdmin)))
	tests := []struct {
		method, path, body, media, user string
		status                          int
		needle                          string
	}{
		{http.MethodGet, "/v1/skills/candidates/official", "", "", testSkillAdmin, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.MethodPost, "/v1/skills/candidates/zip", "not zip", "application/json", testSkillAdmin, http.StatusUnsupportedMediaType, "INVALID_SKILL_ARCHIVE"},
		{http.MethodGet, "/v1/skills/store?page=0&pageSize=20", "", "", testSkillUser, http.StatusBadRequest, "INVALID_SKILL_STORE_QUERY"},
		{http.MethodGet, "/v1/skills/store?page=1&pageSize=10", "", "", testSkillUser, http.StatusBadRequest, "INVALID_SKILL_STORE_QUERY"},
		{http.MethodDelete, "/v1/skills/library/not-a-uuid?revision=1", "", "", testSkillUser, http.StatusBadRequest, "INVALID_SKILL_UNINSTALL"},
	}
	for _, test := range tests {
		recorder := skillRequest(handler, test.method, test.path, strings.NewReader(test.body), test.media, test.user)
		assertSkillHTTP(t, recorder, test.status, test.needle)
	}
}

func skillRequest(
	handler http.Handler,
	method, path string,
	body io.Reader,
	mediaType, userID string,
) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, body)
	if mediaType != "" {
		request.Header.Set("Content-Type", mediaType)
	}
	request = request.WithContext(auth.WithUser(context.Background(), auth.User{ID: userID}))
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertSkillHTTP(t *testing.T, recorder *httptest.ResponseRecorder, status int, needle string) {
	t.Helper()
	if recorder.Code != status || !strings.Contains(recorder.Body.String(), needle) {
		t.Fatalf("status=%d body=%s, want %d/%q", recorder.Code, recorder.Body.String(), status, needle)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control = %q", recorder.Header().Get("Cache-Control"))
	}
}

func bytesReader(data []byte) *bytes.Reader { return bytes.NewReader(data) }
