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
	handler := NewHandler(NewService(repo, WithCursorCodec(codec), WithIDGenerator(func() (string, error) {
		return repo.createResult.ID, nil
	})))

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
	} {
		status, body := serviceErrorFor(test.err)
		if status != test.want || body.Code != test.code {
			t.Fatalf("serviceErrorFor(%v) = %d/%s", test.err, status, body.Code)
		}
	}
}
