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

func TestHandlerCatalogDetailInstallAndLibraryLifecycle(t *testing.T) {
	commit := strings.Repeat("e", 40)
	archive := mustRawTestArchive(t, []testZipEntry{{
		name: "skills-" + commit + "/skills/.curated/demo-skill/SKILL.md",
		body: "---\nname: demo-skill\ndescription: Official-style instruction-only fixture.\n---\n\n# Demo\n",
	}})
	validated, err := ValidateArchive(ArchiveSource{
		Type: SourceGit, Ref: "fixture", ExpectedName: "demo-skill",
		StripPrefix: "skills-" + commit + "/skills/.curated/demo-skill/",
		Data:        archive,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := sourceRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.String() {
		case "https://api.github.com/repos/openai/skills/contents/skills/.curated?ref=main":
			return sourceResponse(request, "application/json",
				`[{"name":"demo-skill","path":"skills/.curated/demo-skill","type":"dir"}]`), nil
		case "https://api.github.com/repos/openai/skills/commits/main":
			return sourceResponse(request, "application/json", `{"sha":"`+commit+`"}`), nil
		case "https://codeload.github.com/openai/skills/zip/" + commit:
			return &http.Response{StatusCode: http.StatusOK,
				Body: io.NopCloser(bytes.NewReader(archive)), Request: request}, nil
		default:
			t.Fatalf("unexpected request %s", request.URL)
			return nil, nil
		}
	})
	repository := newMemoryRepository()
	service := NewService(
		WithRepository(repository), WithObjectStore(newMemoryObjectStore()),
		WithGitHubClient(client),
	)
	service.newID = sequenceIDs("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	handler := NewHandler(service)

	list := skillRequest(handler, http.MethodGet, "/v1/skills/catalog", nil, "", testSkillUser)
	assertSkillHTTP(t, list, http.StatusOK, `"catalogSource":"openai/skills curated"`)
	detail := skillRequest(handler, http.MethodGet, "/v1/skills/catalog/items/demo-skill", nil, "", testSkillUser)
	assertSkillHTTP(t, detail, http.StatusOK, `"resolvedCommit":"`+commit+`"`)
	assertSkillHTTP(t, detail, http.StatusOK, `"allowedTools":[]`)

	drift := skillRequest(handler, http.MethodPost, "/v1/skills/catalog/items/demo-skill/install",
		strings.NewReader(`{"resolvedCommit":"`+commit+`","packageFingerprint":"sha256:`+
			strings.Repeat("f", 64)+`"}`), "application/json", testSkillUser)
	assertSkillHTTP(t, drift, http.StatusConflict, "SKILL_PACKAGE_CHANGED")
	install := skillRequest(handler, http.MethodPost, "/v1/skills/catalog/items/demo-skill/install",
		strings.NewReader(`{"resolvedCommit":"`+commit+`","packageFingerprint":"`+
			validated.Package.PackageFingerprint+`"}`), "application/json", testSkillUser)
	assertSkillHTTP(t, install, http.StatusCreated, `"name":"demo-skill"`)
	assertSkillHTTP(t, install, http.StatusCreated, `"allowedTools":[]`)
	library := skillRequest(handler, http.MethodGet, "/v1/skills/library", nil, "", testSkillUser)
	assertSkillHTTP(t, library, http.StatusOK, `"name":"demo-skill"`)
	assertSkillHTTP(t, library, http.StatusOK, `"allowedTools":[]`)
	store := skillRequest(handler, http.MethodGet, "/v1/skills/store?page=1&pageSize=20", nil, "", testSkillUser)
	assertSkillHTTP(t, store, http.StatusOK, `"totalCount":0`)
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
