package knowledge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/teams"
)

func TestHandlerCollectionCRUDAndStrictPayloads(t *testing.T) {
	codec, _ := teams.NewCursorCodec(teams.CursorKeyring{
		ActiveKeyID: "test", Keys: map[string][]byte{"test": []byte("01234567890123456789012345678901")},
	})
	repo := &fakeRepository{createResult: testCollection("22222222-2222-4222-8222-222222222222")}
	repo.documentResult = Document{ID: "33333333-3333-4333-8333-333333333333", CollectionID: repo.createResult.ID,
		Status: "processing", CreatedAt: repo.createResult.CreatedAt, UpdatedAt: repo.createResult.UpdatedAt}
	repo.documentPage = DocumentPageResult{Items: []Document{repo.documentResult}}
	repo.contentResult = DocumentContentMetadata{DocumentID: repo.documentResult.ID,
		ObjectKey: "private/never-expose", FileName: "source.html", MIMEType: "text/html", ByteSize: 6}
	store := &fakeObjectStore{body: []byte("source")}
	handler := NewHandler(NewService(repo, WithCursorCodec(codec), WithIDGenerator(func() (string, error) {
		return repo.createResult.ID, nil
	}), WithObjectStore(store)))

	perform := func(method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request = request.WithContext(auth.WithUser(request.Context(), auth.User{ID: testActorID}))
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	created := perform(http.MethodPost, collectionsPath, `{"name":"Research","scope":"personal","idempotencyKey":"create-1"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d; body=%s", created.Code, created.Body.String())
	}
	var dto collectionDTO
	if err := json.NewDecoder(created.Body).Decode(&dto); err != nil || dto.Permissions.Manage != true {
		t.Fatalf("create DTO = %#v, err=%v", dto, err)
	}

	for _, body := range []string{
		`{"name":"A","scope":"personal","idempotencyKey":"1","ownerUserId":"` + testActorID + `"}`,
		`{"name":"A","scope":"personal","idempotencyKey":"1","bogus":true}`,
		`[]`,
	} {
		recorder := perform(http.MethodPost, collectionsPath, body)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("strict body %s status = %d", body, recorder.Code)
		}
	}

	forbidden := perform(http.MethodGet, collectionsPath+"?aclRevision=1", "")
	if forbidden.Code != http.StatusBadRequest || !strings.Contains(forbidden.Body.String(), ErrorCodeForbiddenIdentityField) {
		t.Fatalf("forbidden query response = %d %s", forbidden.Code, forbidden.Body.String())
	}
	deleteBody := perform(
		http.MethodDelete,
		collectionsPath+"/22222222-2222-4222-8222-222222222222",
		`{"ownerUserId":"`+testActorID+`"}`,
	)
	if deleteBody.Code != http.StatusBadRequest || !strings.Contains(deleteBody.Body.String(), ErrorCodeForbiddenIdentityField) {
		t.Fatalf("delete body response = %d %s", deleteBody.Code, deleteBody.Body.String())
	}
	document := perform(http.MethodPost, collectionsPath+"/"+repo.createResult.ID+"/documents",
		`{"fileId":"44444444-4444-4444-8444-444444444444","idempotencyKey":"doc-1"}`)
	if document.Code != http.StatusCreated || !strings.Contains(document.Body.String(), repo.documentResult.ID) {
		t.Fatalf("document response = %d %s", document.Code, document.Body.String())
	}
	listed := perform(http.MethodGet, collectionsPath+"/"+repo.createResult.ID+"/documents?limit=1", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), repo.documentResult.ID) {
		t.Fatalf("document list response = %d %s", listed.Code, listed.Body.String())
	}
	gotDocument := perform(http.MethodGet, documentsPathBase+repo.documentResult.ID, "")
	if gotDocument.Code != http.StatusOK || !strings.Contains(gotDocument.Body.String(), repo.documentResult.ID) {
		t.Fatalf("document get response = %d %s", gotDocument.Code, gotDocument.Body.String())
	}
	content := perform(http.MethodGet, documentsPathBase+repo.documentResult.ID+"/content", "")
	if content.Code != http.StatusOK || content.Body.String() != "source" {
		t.Fatalf("document content response = %d %q", content.Code, content.Body.String())
	}
	if content.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.HasPrefix(content.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("unsafe content headers = %#v", content.Header())
	}
	if strings.Contains(content.Header().Get("Content-Disposition"), "private") ||
		strings.Contains(content.Body.String(), repo.contentResult.ObjectKey) {
		t.Fatal("private object key leaked from content endpoint")
	}
	replacement := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/versions",
		`{"fileId":"44444444-4444-4444-8444-444444444444","idempotencyKey":"replace-1"}`)
	if replacement.Code != http.StatusCreated || !strings.Contains(replacement.Body.String(), repo.documentResult.ID) {
		t.Fatalf("replacement response = %d %s", replacement.Code, replacement.Body.String())
	}
	for _, body := range []string{
		`{"fileId":"44444444-4444-4444-8444-444444444444","idempotencyKey":"replace-2","unknown":true}`,
		`[]`, `{`,
	} {
		invalid := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/versions", body)
		if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), ErrorCodeInvalidDocumentPayload) {
			t.Fatalf("invalid replacement body %q = %d %s", body, invalid.Code, invalid.Body.String())
		}
	}
	forbiddenReplacement := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/versions",
		`{"fileId":"44444444-4444-4444-8444-444444444444","idempotencyKey":"replace-2","ownerUserId":"`+testActorID+`"}`)
	if forbiddenReplacement.Code != http.StatusBadRequest ||
		!strings.Contains(forbiddenReplacement.Body.String(), ErrorCodeForbiddenIdentityField) {
		t.Fatalf("forbidden replacement = %d %s", forbiddenReplacement.Code, forbiddenReplacement.Body.String())
	}
	queryReplacement := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/versions?aclRevision=1",
		`{"fileId":"44444444-4444-4444-8444-444444444444","idempotencyKey":"replace-2"}`)
	if queryReplacement.Code != http.StatusBadRequest {
		t.Fatalf("replacement query = %d %s", queryReplacement.Code, queryReplacement.Body.String())
	}
	wrongMethod := perform(http.MethodGet, documentsPathBase+repo.documentResult.ID+"/versions", "")
	if wrongMethod.Code != http.StatusMethodNotAllowed || wrongMethod.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("replacement method = %d allow=%q", wrongMethod.Code, wrongMethod.Header().Get("Allow"))
	}
	reprocess := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/reprocess",
		`{"idempotencyKey":"reprocess-1"}`)
	if reprocess.Code != http.StatusCreated || !strings.Contains(reprocess.Body.String(), repo.documentResult.ID) {
		t.Fatalf("reprocess response = %d %s", reprocess.Code, reprocess.Body.String())
	}
	invalidReprocess := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/reprocess",
		`{"idempotencyKey":"reprocess-2","fileId":"44444444-4444-4444-8444-444444444444"}`)
	if invalidReprocess.Code != http.StatusBadRequest ||
		!strings.Contains(invalidReprocess.Body.String(), ErrorCodeInvalidDocumentPayload) {
		t.Fatalf("invalid reprocess = %d %s", invalidReprocess.Code, invalidReprocess.Body.String())
	}
	emptyReprocess := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/reprocess",
		`{"idempotencyKey":""}`)
	if emptyReprocess.Code != http.StatusBadRequest ||
		!strings.Contains(emptyReprocess.Body.String(), ErrorCodeInvalidDocumentPayload) {
		t.Fatalf("empty reprocess = %d %s", emptyReprocess.Code, emptyReprocess.Body.String())
	}
	forbiddenReprocess := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/reprocess",
		`{"idempotencyKey":"reprocess-2","ownerUserId":"`+testActorID+`"}`)
	if forbiddenReprocess.Code != http.StatusBadRequest ||
		!strings.Contains(forbiddenReprocess.Body.String(), ErrorCodeForbiddenIdentityField) {
		t.Fatalf("forbidden reprocess = %d %s", forbiddenReprocess.Code, forbiddenReprocess.Body.String())
	}
	queryReprocess := perform(http.MethodPost, documentsPathBase+repo.documentResult.ID+"/reprocess?cursor=x",
		`{"idempotencyKey":"reprocess-2"}`)
	if queryReprocess.Code != http.StatusBadRequest ||
		!strings.Contains(queryReprocess.Body.String(), ErrorCodeInvalidDocumentPayload) {
		t.Fatalf("reprocess query = %d %s", queryReprocess.Code, queryReprocess.Body.String())
	}
	wrongReprocessMethod := perform(http.MethodGet, documentsPathBase+repo.documentResult.ID+"/reprocess", "")
	if wrongReprocessMethod.Code != http.StatusMethodNotAllowed ||
		wrongReprocessMethod.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("reprocess method = %d allow=%q", wrongReprocessMethod.Code,
			wrongReprocessMethod.Header().Get("Allow"))
	}
}

func TestHandlerMapsDisclosureErrors(t *testing.T) {
	for _, test := range []struct {
		err  error
		want int
		code string
	}{
		{ErrCollectionNotFound, http.StatusNotFound, "COLLECTION_NOT_FOUND"},
		{ErrTeamAdminRequired, http.StatusForbidden, "TEAM_ADMIN_REQUIRED"},
		{ErrIdempotencyConflict, http.StatusConflict, "IDEMPOTENCY_CONFLICT"},
		{ErrDocumentNotFound, http.StatusNotFound, "DOCUMENT_NOT_FOUND"},
		{ErrDocumentProcessing, http.StatusConflict, "DOCUMENT_PROCESSING"},
		{ErrKnowledgeProcessorUnavailable, http.StatusServiceUnavailable, "KNOWLEDGE_PROCESSOR_UNAVAILABLE"},
	} {
		status, body := serviceErrorFor(test.err)
		if status != test.want || body.Code != test.code {
			t.Fatalf("serviceErrorFor(%v) = %d/%s", test.err, status, body.Code)
		}
	}
}
