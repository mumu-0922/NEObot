package agentcontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentcron"
	"neo-chat/mm-chat/backend/internal/agentlearning"
	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	centerPath         = "/v1/agent-center"
	maxRequestJSONSize = int64(256 << 10)
	contentTypeJSON    = "application/json; charset=utf-8"
)

type Handler struct{ service *Service }

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService()
	}
	return &Handler{service: service}
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	path := strings.TrimPrefix(request.URL.Path, centerPath)
	user := auth.UserOrDevelopment(request.Context())
	switch {
	case path == "/status":
		handler.handleStatus(writer, request, user.ID)
	case path == "/runs":
		handler.handleRuns(writer, request, user.ID)
	case strings.HasPrefix(path, "/runs/"):
		handler.handleRun(writer, request, user.ID, strings.TrimPrefix(path, "/runs/"))
	case path == "/schedules":
		handler.handleSchedules(writer, request, user.ID)
	case strings.HasPrefix(path, "/schedules/"):
		handler.handleSchedule(writer, request, user.ID, strings.TrimPrefix(path, "/schedules/"))
	case path == "/learning/drafts":
		handler.handleDrafts(writer, request, user.ID)
	case strings.HasPrefix(path, "/learning/drafts/"):
		handler.handleDraft(writer, request, user.ID, strings.TrimPrefix(path, "/learning/drafts/"))
	case path == "/shadow/opt-in":
		handler.handleShadowOptIn(writer, request, user.ID)
	case path == "/admin/shadow-policy":
		handler.handleShadowPolicy(writer, request, user.ID)
	default:
		writeError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (handler *Handler) handleStatus(writer http.ResponseWriter, request *http.Request, userID string) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	status, err := handler.service.Status(request.Context(), userID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, status)
}

func (handler *Handler) handleRuns(writer http.ResponseWriter, request *http.Request, userID string) {
	switch request.Method {
	case http.MethodGet:
		limit, ok := queryLimit(request, 50)
		if !ok {
			writeError(writer, http.StatusBadRequest, "INVALID_AGENT_QUERY", "Agent query is invalid")
			return
		}
		runs, err := handler.service.ListRuns(request.Context(), userID, limit)
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"runs": runs})
	case http.MethodPost:
		if err := handler.service.EnqueueRootRun(request.Context(), userID); err != nil {
			writeServiceError(writer, err)
			return
		}
	default:
		methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

func (handler *Handler) handleRun(
	writer http.ResponseWriter,
	request *http.Request,
	userID, suffix string,
) {
	parts := strings.Split(suffix, "/")
	if len(parts) == 1 && parts[0] != "" {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer, http.MethodGet)
			return
		}
		detail, err := handler.service.GetRun(request.Context(), userID, parts[0])
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, detail)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" {
		handler.handleRunCancellation(writer, request, userID, parts[0])
		return
	}
	if len(parts) == 4 && parts[1] == "artifacts" && parts[3] == "content" {
		handler.handleArtifactContent(writer, request, userID, parts[0], parts[2])
		return
	}
	if len(parts) == 4 && parts[1] == "approvals" {
		handler.handleApproval(writer, request, userID, parts[0], parts[2], parts[3])
		return
	}
	writeError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
}

func (handler *Handler) handleArtifactContent(
	writer http.ResponseWriter,
	request *http.Request,
	userID, runID, artifactID string,
) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	artifact, reader, err := handler.service.GetArtifactContent(
		request.Context(), userID, runID, artifactID,
	)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	defer reader.Close()
	writer.Header().Set("Content-Type", artifact.MediaType)
	writer.Header().Set("Content-Length", fmt.Sprintf("%d", artifact.Size))
	writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": artifact.Name,
	}))
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.Copy(writer, io.LimitReader(reader, artifact.Size))
}

func (handler *Handler) handleRunCancellation(
	writer http.ResponseWriter,
	request *http.Request,
	userID, runID string,
) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var body struct {
		CancellationID      string `json:"cancellationId"`
		ExpectedState       string `json:"expectedState"`
		SnapshotFingerprint string `json:"snapshotFingerprint"`
		Mode                string `json:"mode"`
		ReasonCode          string `json:"reasonCode"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	result, err := handler.service.CancelRun(request.Context(), userID, CancelRunInput{
		RunID: runID, CancellationID: body.CancellationID,
		ExpectedState: body.ExpectedState, SnapshotFingerprint: body.SnapshotFingerprint,
		Mode: body.Mode, ReasonCode: body.ReasonCode,
	})
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"cancellation": result})
}

func (handler *Handler) handleApproval(
	writer http.ResponseWriter,
	request *http.Request,
	userID, runID, intentID, action string,
) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	detail, err := handler.service.GetRun(request.Context(), userID, runID)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	found := false
	for _, approval := range detail.Approvals {
		if approval.IntentID == intentID {
			found = true
			break
		}
	}
	if !found {
		writeServiceError(writer, ErrNotFound)
		return
	}
	switch action {
	case "decision":
		var body struct {
			ApprovalID        string `json:"approvalId"`
			IntentFingerprint string `json:"intentFingerprint"`
			Decision          string `json:"decision"`
			ReasonCode        string `json:"reasonCode"`
			ExpectedRevision  int64  `json:"expectedRevision"`
		}
		if !decodeJSON(writer, request, &body) {
			return
		}
		result, err := handler.service.DecideApproval(request.Context(), userID, agentbroker.ApprovalInput{
			ApprovalID: body.ApprovalID, IntentID: intentID,
			IntentFingerprint: body.IntentFingerprint, Decision: body.Decision,
			ReasonCode: body.ReasonCode, ExpectedRevision: body.ExpectedRevision,
		})
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"approval": result})
	case "cancel":
		var body struct {
			CancellationID    string `json:"cancellationId"`
			IntentFingerprint string `json:"intentFingerprint"`
			ReasonCode        string `json:"reasonCode"`
		}
		if !decodeJSON(writer, request, &body) {
			return
		}
		result, err := handler.service.CancelApproval(request.Context(), userID, agentbroker.CancelInput{
			CancellationID: body.CancellationID, IntentID: intentID,
			IntentFingerprint: body.IntentFingerprint, ReasonCode: body.ReasonCode,
		})
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"approval": result})
	default:
		writeError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (handler *Handler) handleSchedules(writer http.ResponseWriter, request *http.Request, userID string) {
	switch request.Method {
	case http.MethodGet:
		limit, ok := queryLimit(request, 50)
		if !ok {
			writeError(writer, http.StatusBadRequest, "INVALID_AGENT_QUERY", "Agent query is invalid")
			return
		}
		items, err := handler.service.ListSchedules(request.Context(), userID, limit)
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"schedules": items})
	case http.MethodPost:
		var body struct {
			TemplateID       string                 `json:"templateId"`
			ExpectedRevision int64                  `json:"expectedRevision"`
			Spec             agentcron.TemplateSpec `json:"spec"`
			ReasonCode       string                 `json:"reasonCode"`
			ActivateAt       *time.Time             `json:"activateAt"`
		}
		if !decodeJSON(writer, request, &body) {
			return
		}
		input := agentcron.CreateInput{
			TemplateID: body.TemplateID, ExpectedRevision: body.ExpectedRevision,
			Spec: body.Spec, Approval: agentcron.Approval{ReasonCode: body.ReasonCode},
		}
		if body.ActivateAt != nil {
			input.ActivateAt = *body.ActivateAt
		}
		item, created, err := handler.service.CreateSchedule(request.Context(), userID, input)
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, map[string]any{"schedule": item, "created": created})
	default:
		methodNotAllowed(writer, http.MethodGet+", "+http.MethodPost)
	}
}

func (handler *Handler) handleSchedule(
	writer http.ResponseWriter,
	request *http.Request,
	userID, suffix string,
) {
	parts := strings.Split(suffix, "/")
	if len(parts) == 1 && parts[0] != "" {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer, http.MethodGet)
			return
		}
		item, err := handler.service.GetSchedule(request.Context(), userID, parts[0])
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"schedule": item})
		return
	}
	if len(parts) == 2 && parts[1] == "lifecycle" {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer, http.MethodPost)
			return
		}
		var body struct {
			ExpectedRevision int64  `json:"expectedRevision"`
			State            string `json:"state"`
			ReasonCode       string `json:"reasonCode"`
		}
		if !decodeJSON(writer, request, &body) {
			return
		}
		item, err := handler.service.ChangeScheduleLifecycle(request.Context(), userID,
			parts[0], body.State, body.ExpectedRevision, body.ReasonCode)
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"schedule": item})
		return
	}
	writeError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
}

func (handler *Handler) handleDrafts(writer http.ResponseWriter, request *http.Request, userID string) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	limit, ok := queryLimit(request, 50)
	if !ok {
		writeError(writer, http.StatusBadRequest, "INVALID_AGENT_QUERY", "Agent query is invalid")
		return
	}
	items, err := handler.service.ListDrafts(request.Context(), userID, limit)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"drafts": items})
}

func (handler *Handler) handleDraft(
	writer http.ResponseWriter,
	request *http.Request,
	userID, suffix string,
) {
	parts := strings.Split(suffix, "/")
	if len(parts) == 1 && parts[0] != "" {
		if request.Method != http.MethodGet {
			methodNotAllowed(writer, http.MethodGet)
			return
		}
		item, err := handler.service.GetDraft(request.Context(), userID, parts[0])
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"draft": item})
		return
	}
	if len(parts) != 2 {
		writeError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	switch parts[1] {
	case "diff":
		if request.Method != http.MethodGet {
			methodNotAllowed(writer, http.MethodGet)
			return
		}
		diff, err := handler.service.GetDraftDiff(request.Context(), userID, parts[0])
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"files": diff})
	case "review":
		if request.Method != http.MethodPost {
			methodNotAllowed(writer, http.MethodPost)
			return
		}
		var body struct {
			Decision                   string `json:"decision"`
			ExpectedRevision           int64  `json:"expectedRevision"`
			DraftFingerprint           string `json:"draftFingerprint"`
			ProposedPackageFingerprint string `json:"proposedPackageFingerprint"`
			ReasonCode                 string `json:"reasonCode"`
		}
		if !decodeJSON(writer, request, &body) {
			return
		}
		result, err := handler.service.ReviewDraft(request.Context(), userID, body.Decision,
			agentlearning.ReviewInput{
				DraftID: parts[0], ExpectedRevision: body.ExpectedRevision,
				DraftFingerprint:           body.DraftFingerprint,
				ProposedPackageFingerprint: body.ProposedPackageFingerprint,
				ReasonCode:                 body.ReasonCode,
			})
		if err != nil {
			writeServiceError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, result)
	default:
		writeError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (handler *Handler) handleShadowOptIn(writer http.ResponseWriter, request *http.Request, userID string) {
	if request.Method != http.MethodPut {
		methodNotAllowed(writer, http.MethodPut)
		return
	}
	var body struct {
		ExpectedGeneration int64  `json:"expectedGeneration"`
		PolicyRevision     int64  `json:"policyRevision"`
		OptedIn            bool   `json:"optedIn"`
		ReasonCode         string `json:"reasonCode"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	result, err := handler.service.SetShadowOptIn(request.Context(), userID, SetShadowOptInInput{
		ExpectedGeneration: body.ExpectedGeneration, PolicyRevision: body.PolicyRevision,
		OptedIn: body.OptedIn, ReasonCode: body.ReasonCode,
	})
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"shadow": result})
}

func (handler *Handler) handleShadowPolicy(writer http.ResponseWriter, request *http.Request, userID string) {
	if request.Method != http.MethodPut {
		methodNotAllowed(writer, http.MethodPut)
		return
	}
	var body struct {
		ExpectedRevision         int64      `json:"expectedRevision"`
		Enabled                  bool       `json:"enabled"`
		Mode                     string     `json:"mode"`
		AdmissionID              string     `json:"admissionId"`
		PackageFingerprint       string     `json:"packageFingerprint"`
		RuntimeBundleFingerprint string     `json:"runtimeBundleFingerprint"`
		CohortBasisPoints        int        `json:"cohortBasisPoints"`
		MaxObservations          int        `json:"maxObservations"`
		MaxErrors                int        `json:"maxErrors"`
		StartsAt                 *time.Time `json:"startsAt"`
		ExpiresAt                *time.Time `json:"expiresAt"`
		ReasonCode               string     `json:"reasonCode"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	result, err := handler.service.UpdateShadowPolicy(request.Context(), userID, UpdateShadowPolicyInput{
		ExpectedRevision: body.ExpectedRevision, Enabled: body.Enabled, Mode: body.Mode,
		AdmissionID: body.AdmissionID, PackageFingerprint: body.PackageFingerprint,
		RuntimeBundleFingerprint: body.RuntimeBundleFingerprint,
		CohortBasisPoints:        body.CohortBasisPoints, MaxObservations: body.MaxObservations,
		MaxErrors: body.MaxErrors, StartsAt: body.StartsAt, ExpiresAt: body.ExpiresAt,
		ReasonCode: body.ReasonCode,
	})
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"policy": result})
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	data, err := io.ReadAll(io.LimitReader(request.Body, maxRequestJSONSize+1))
	if err != nil || strictjson.Decode(data, int(maxRequestJSONSize), destination) != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_JSON", "request body is invalid")
		return false
	}
	return true
}

func queryLimit(request *http.Request, fallback int) (int, bool) {
	value := strings.TrimSpace(request.URL.Query().Get("limit"))
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil && parsed >= 1 && parsed <= 100
}

func writeServiceError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, agentbroker.ErrInvalidInput),
		errors.Is(err, agentcron.ErrInvalidInput), errors.Is(err, agentlearning.ErrInvalidInput):
		writeError(writer, http.StatusBadRequest, "INVALID_AGENT_REQUEST", "Agent request is invalid")
	case errors.Is(err, ErrAdministratorNeeded), errors.Is(err, agentlearning.ErrAdministratorNeeded):
		writeError(writer, http.StatusForbidden, "AGENT_ADMIN_REQUIRED", "Agent administrator access is required")
	case errors.Is(err, ErrNotFound), errors.Is(err, agentbroker.ErrNotFound),
		errors.Is(err, agentcron.ErrNotFound), errors.Is(err, agentlearning.ErrNotFound):
		writeError(writer, http.StatusNotFound, "AGENT_NOT_FOUND", "Agent record was not found")
	case errors.Is(err, ErrIsolationUnavailable), errors.Is(err, agentlearning.ErrLearningDisabled):
		writeError(writer, http.StatusConflict, RuntimeHeldReason, "Agent Runtime is held until exact-host isolation passes")
	case errorIsConflict(err), errors.Is(err, ErrGenerationStale),
		errors.Is(err, ErrFingerprintDrift), errors.Is(err, agentbroker.ErrReplayDetected):
		writeError(writer, http.StatusConflict, "AGENT_STALE_CONFLICT", "Agent state changed; reload before writing")
	case errors.Is(err, ErrBudgetExceeded), errors.Is(err, agentbroker.ErrBudgetExhausted):
		writeError(writer, http.StatusConflict, "AGENT_BUDGET_EXCEEDED", "Agent budget is exhausted")
	case errors.Is(err, ErrRunCancelBlocked), errors.Is(err, agentbroker.ErrInvalidTransition),
		errors.Is(err, agentbroker.ErrApprovalDenied), errors.Is(err, agentbroker.ErrIntentExpired):
		writeError(writer, http.StatusConflict, "AGENT_TRANSITION_DENIED", "Agent transition is not currently allowed")
	case errors.Is(err, agentcron.ErrOwnerRevoked), errors.Is(err, agentcron.ErrSkillRevoked),
		errors.Is(err, agentcron.ErrGrantRevoked), errors.Is(err, agentcron.ErrApprovalRevoked),
		errors.Is(err, agentcron.ErrExpired), errors.Is(err, agentcron.ErrKillSwitchActive),
		errors.Is(err, ErrKillSwitchActive), errors.Is(err, agentlearning.ErrKillSwitchActive),
		errors.Is(err, agentlearning.ErrPromotionDenied),
		errors.Is(err, agentlearning.ErrChecksIncomplete):
		writeError(writer, http.StatusConflict, "AGENT_AUTHORITY_DENIED", "Agent authority is no longer current")
	default:
		writeError(writer, http.StatusServiceUnavailable, "AGENT_CONTROL_UNAVAILABLE", "Agent control plane is unavailable")
	}
}

func methodNotAllowed(writer http.ResponseWriter, allow string) {
	writer.Header().Set("Allow", allow)
	writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", contentTypeJSON)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
