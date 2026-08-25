package resourceorchestrator

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/skillsupply"
)

func TestHandlerInstallsExactSkillThroughUnifiedContract(t *testing.T) {
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	handler := NewHandler(NewService(fakeSkills{store: skillsupply.StoreResult{
		Items: []skillsupply.Candidate{{
			ID: "candidate-id", Status: skillsupply.StatusAdmitted,
			Package: skillsupply.PackageVersion{
				Name: "office-xlsx", Version: "1.0.0", PackageFingerprint: fingerprint,
			},
		}},
	}}, fakeMCP{}))
	request := httptest.NewRequest(http.MethodPost, ResourcesPath+"/install", strings.NewReader(`{
		"kind":"skill","id":"candidate-id","version":"1.0.0",
		"exactRevision":"`+fingerprint+`","conversationId":"conversation-id"
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"name":"office-xlsx"`) ||
		!strings.Contains(response.Body.String(), `"refreshRequired":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerRejectsStaleAndUnknownInstallInput(t *testing.T) {
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	handler := NewHandler(NewService(fakeSkills{store: skillsupply.StoreResult{
		Items: []skillsupply.Candidate{{
			ID: "candidate-id", Status: skillsupply.StatusAdmitted,
			Package: skillsupply.PackageVersion{
				Name: "office-xlsx", Version: "1.0.0", PackageFingerprint: fingerprint,
			},
		}},
	}}, fakeMCP{}))

	for _, fixture := range []struct {
		name string
		body string
		want int
	}{
		{
			name: "stale version",
			body: `{"kind":"skill","id":"candidate-id","version":"2.0.0",` +
				`"exactRevision":"` + fingerprint + `","conversationId":"conversation-id"}`,
			want: http.StatusConflict,
		},
		{
			name: "unknown field",
			body: `{"kind":"skill","id":"candidate-id","version":"1.0.0",` +
				`"exactRevision":"` + fingerprint + `","conversationId":"conversation-id","secret":"leak"}`,
			want: http.StatusBadRequest,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, ResourcesPath+"/install", strings.NewReader(fixture.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != fixture.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestHandlerRoutesLifecycleMutationThroughUnifiedContract(t *testing.T) {
	handler := NewHandler(NewService(fakeSkills{library: []skillsupply.Installation{{
		ID: "installation-id", Name: "office-xlsx", Revision: 4,
	}}}, fakeMCP{}))
	request := httptest.NewRequest(http.MethodPost, ResourcesPath+"/mutate", strings.NewReader(`{
		"kind":"skill","action":"remove","id":"installation-id",
		"expectedRevision":4,"conversationId":"conversation-id"
	}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"status":"removed"`) ||
		!strings.Contains(response.Body.String(), `"refreshRequired":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerRejectsForbiddenAndSecretBearingLifecycleMutation(t *testing.T) {
	handler := NewHandler(NewService(fakeSkills{}, fakeMCP{}))

	for _, fixture := range []struct {
		name string
		body string
		want int
	}{
		{
			name: "cross owner skill id",
			body: `{"kind":"skill","action":"remove","id":"installation-id",` +
				`"expectedRevision":4,"conversationId":"conversation-id"}`,
			want: http.StatusForbidden,
		},
		{
			name: "secret field",
			body: `{"kind":"mcp","action":"enable","id":"catalog:deepwiki",` +
				`"expectedRevision":1,"conversationId":"conversation-id","secret":"leak"}`,
			want: http.StatusBadRequest,
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, ResourcesPath+"/mutate", strings.NewReader(fixture.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != fixture.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
