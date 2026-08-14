package agentcontrol

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/agentcron"
	"neo-chat/mm-chat/backend/internal/auth"
)

func requestWithUser(method, target, body, userID string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request.WithContext(auth.WithUser(request.Context(), auth.User{ID: userID}))
}

func TestHandlerStatusAndHeldEnqueue(t *testing.T) {
	repository := &fakeRepository{shadow: ShadowSnapshot{HeldReasonCode: "SHADOW_DISABLED"}}
	handler := NewHandler(NewService(WithRepository(repository), WithAdministratorUserID(testAdminID)))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodGet, centerPath+"/status", "", testAdminID))
	if response.Code != http.StatusOK {
		t.Fatalf("status code = %d, body = %s", response.Code, response.Body.String())
	}
	var status CenterStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.IsAdministrator || status.Runtime.ReasonCode != RuntimeHeldReason {
		t.Fatalf("status = %#v", status)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPost, centerPath+"/runs", `{}`, testAdminID))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), RuntimeHeldReason) {
		t.Fatalf("enqueue code = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHandlerRejectsCrossUserRunAndNonAdminLearning(t *testing.T) {
	repository := &fakeRepository{detailByUser: map[string]RunDetail{
		testUserID + "/" + testRunID: {Run: RunSummary{ID: testRunID}},
	}}
	handler := NewHandler(NewService(WithRepository(repository), WithAdministratorUserID(testAdminID)))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodGet, centerPath+"/runs/"+testRunID, "", testAdminID))
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-user code = %d, body = %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodGet, centerPath+"/learning/drafts", "", testUserID))
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "AGENT_ADMIN_REQUIRED") {
		t.Fatalf("learning code = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHandlerRunCancellationRejectsUnknownFieldsAndBindsUser(t *testing.T) {
	repository := &fakeRepository{}
	handler := NewHandler(NewService(WithRepository(repository)))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPost, centerPath+"/runs/"+testRunID+"/cancel",
		`{"expectedState":"queued","snapshotFingerprint":"`+testFP+`","mode":"cancel","reasonCode":"USER_REQUESTED","extra":true}`,
		testUserID))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field code = %d, body = %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPost, centerPath+"/runs/"+testRunID+"/cancel",
		`{"expectedState":"queued","snapshotFingerprint":"`+testFP+`","mode":"cancel","reasonCode":"USER_REQUESTED"}`,
		testUserID))
	if response.Code != http.StatusOK || repository.cancellation.UserID != testUserID {
		t.Fatalf("cancel code = %d, body = %s, capture = %#v", response.Code, response.Body.String(), repository.cancellation)
	}
}

func TestHandlerMethodAndUnknownRouteAreBounded(t *testing.T) {
	handler := NewHandler(NewService(WithRepository(&fakeRepository{})))
	for _, testCase := range []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodDelete, centerPath + "/status", http.StatusMethodNotAllowed},
		{http.MethodGet, centerPath + "/unknown", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, requestWithUser(testCase.method, testCase.path, "", testUserID))
		if response.Code != testCase.want || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s %s = %d, headers=%v", testCase.method, testCase.path, response.Code, response.Header())
		}
	}
}

func TestHandlerDownloadsOwnedArtifactWithoutLeakingObjectKey(t *testing.T) {
	repository := &fakeRepository{artifact: ArtifactSource{
		Artifact: Artifact{
			ID: "artifact_1234567890abcdef", AttemptID: "attempt_1234567890abcdef",
			Generation: 1, Name: "report.txt", MediaType: "text/plain", Size: 6,
			Fingerprint: testFP,
		},
		ObjectKey: "agent-artifacts/private/object-key",
	}}
	handler := NewHandler(NewService(
		WithRepository(repository),
		WithArtifactStore(fakeArtifactStore{payload: []byte("report")}),
	))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodGet,
		centerPath+"/runs/"+testRunID+"/artifacts/"+repository.artifact.ID+"/content", "", testUserID))
	if response.Code != http.StatusOK || response.Body.String() != "report" ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "report.txt") ||
		strings.Contains(response.Body.String(), repository.artifact.ObjectKey) {
		t.Fatalf("artifact response code=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String())
	}
}

func TestWriteServiceErrorSanitizesKillSwitchAuthority(t *testing.T) {
	response := httptest.NewRecorder()
	writeServiceError(response, ErrKillSwitchActive)
	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), "AGENT_AUTHORITY_DENIED") ||
		strings.Contains(response.Body.String(), "KILL_SWITCH_ACTIVE") {
		t.Fatalf("Kill Switch response code=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerBindsApprovalScheduleDraftAndShadowMutations(t *testing.T) {
	intentID := "intent_1234567890abcdef"
	draftID := "draft_1234567890abcdef"
	cronID := "cron_1234567890abcdef"
	repository := &fakeRepository{
		detailByUser: map[string]RunDetail{
			testUserID + "/" + testRunID: {
				Run:       RunSummary{ID: testRunID},
				Approvals: []Approval{{IntentID: intentID}},
			},
		},
		draft: DraftSummary{ID: draftID, Revision: 5},
		shadow: ShadowSnapshot{
			Policy: ShadowPolicy{Revision: 1},
			OptIn:  ShadowOptIn{Generation: 0},
		},
	}
	broker := &fakeBrokerService{}
	cron := &fakeCronService{template: agentcron.Template{
		ID: cronID, UserID: testUserID, CurrentRevision: 1,
		State: agentcron.TemplateActive,
	}}
	learning := &fakeLearningService{}
	handler := NewHandler(NewService(
		WithRepository(repository), WithBroker(broker), WithCron(cron),
		WithLearning(learning), WithAdministratorUserID(testAdminID),
	))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPost,
		centerPath+"/runs/"+testRunID+"/approvals/"+intentID+"/decision",
		`{"intentFingerprint":"`+testFP+`","decision":"approved","reasonCode":"USER_APPROVED","expectedRevision":2}`,
		testUserID))
	if response.Code != http.StatusOK || broker.decision.UserID != testUserID ||
		broker.decision.IntentID != intentID || broker.decision.ExpectedRevision != 2 {
		t.Fatalf("approval code=%d body=%s captured=%#v", response.Code, response.Body.String(), broker.decision)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPost,
		centerPath+"/runs/"+testRunID+"/approvals/"+intentID+"/cancel",
		`{"intentFingerprint":"`+testFP+`","reasonCode":"USER_CANCELED"}`,
		testUserID))
	if response.Code != http.StatusOK || broker.cancel.UserID != testUserID ||
		broker.cancel.IntentID != intentID || broker.cancel.CancellationID == "" {
		t.Fatalf("approval cancel code=%d body=%s captured=%#v", response.Code, response.Body.String(), broker.cancel)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPost, centerPath+"/schedules",
		`{"templateId":"`+cronID+`","expectedRevision":0,"spec":{},"reasonCode":"USER_CREATED"}`,
		testUserID))
	if response.Code != http.StatusCreated || cron.create.Spec.Owner.UserID != testUserID ||
		cron.create.Approval.ActorID != testUserID {
		t.Fatalf("schedule create code=%d body=%s captured=%#v", response.Code, response.Body.String(), cron.create)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPost,
		centerPath+"/schedules/"+cronID+"/lifecycle",
		`{"expectedRevision":1,"state":"paused","reasonCode":"USER_PAUSED"}`,
		testUserID))
	if response.Code != http.StatusOK || cron.lifecycle.UserID != testUserID ||
		cron.lifecycle.ExpectedRevision != 1 || cron.lifecycle.To != agentcron.TemplatePaused {
		t.Fatalf("schedule lifecycle code=%d body=%s captured=%#v", response.Code, response.Body.String(), cron.lifecycle)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPost,
		centerPath+"/learning/drafts/"+draftID+"/review",
		`{"decision":"reject","expectedRevision":4,"draftFingerprint":"`+testFP+`","proposedPackageFingerprint":"`+testRuntime+`","reasonCode":"ADMIN_REJECTED"}`,
		testAdminID))
	if response.Code != http.StatusOK || learning.administratorID != testAdminID ||
		learning.review.DraftID != draftID || learning.review.ExpectedRevision != 4 {
		t.Fatalf("Draft review code=%d body=%s admin=%q captured=%#v", response.Code, response.Body.String(), learning.administratorID, learning.review)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPut,
		centerPath+"/admin/shadow-policy",
		`{"expectedRevision":0,"enabled":false,"mode":"synthetic","cohortBasisPoints":0,"maxObservations":1,"maxErrors":0,"reasonCode":"ADMIN_DISABLED"}`,
		testAdminID))
	if response.Code != http.StatusOK || repository.policy.AdministratorID != testAdminID ||
		repository.policy.ExpectedRevision != 0 {
		t.Fatalf("Shadow policy code=%d body=%s captured=%#v", response.Code, response.Body.String(), repository.policy)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithUser(http.MethodPut,
		centerPath+"/shadow/opt-in",
		`{"expectedGeneration":0,"policyRevision":1,"optedIn":true,"reasonCode":"USER_OPT_IN"}`,
		testUserID))
	if response.Code != http.StatusOK || repository.optIn.UserID != testUserID ||
		repository.optIn.PolicyRevision != 1 || !repository.optIn.OptedIn {
		t.Fatalf("Shadow opt-in code=%d body=%s captured=%#v", response.Code, response.Body.String(), repository.optIn)
	}
}
