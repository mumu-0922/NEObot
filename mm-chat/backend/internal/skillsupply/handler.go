package skillsupply

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	skillsPath              = "/v1/skills"
	candidatesPath          = skillsPath + "/candidates"
	candidatesPathBase      = candidatesPath + "/"
	storePath               = skillsPath + "/store"
	storeItemPathBase       = storePath + "/items/"
	libraryPath             = skillsPath + "/library"
	libraryPathBase         = libraryPath + "/"
	conversationPathBase    = skillsPath + "/conversations/"
	maxSkillRequestJSONSize = int64(1 << 20)
	contentTypeJSON         = "application/json; charset=utf-8"
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
	switch {
	case strings.HasPrefix(request.URL.Path, conversationPathBase):
		handler.handleConversationSelection(writer, request, strings.TrimPrefix(request.URL.Path, conversationPathBase))
	case strings.HasPrefix(request.URL.Path, candidatesPathBase):
		handler.handleCandidate(writer, request, strings.TrimPrefix(request.URL.Path, candidatesPathBase))
	case request.URL.Path == storePath:
		handler.handleStore(writer, request)
	case strings.HasPrefix(request.URL.Path, storeItemPathBase):
		handler.handleStoreItem(writer, request, strings.TrimPrefix(request.URL.Path, storeItemPathBase))
	case request.URL.Path == libraryPath:
		handler.handleLibrary(writer, request)
	case strings.HasPrefix(request.URL.Path, libraryPathBase):
		handler.handleLibraryItem(writer, request, strings.TrimPrefix(request.URL.Path, libraryPathBase))
	default:
		writeSkillError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (handler *Handler) handleConversationSelection(
	writer http.ResponseWriter,
	request *http.Request,
	suffix string,
) {
	parts := strings.Split(suffix, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "selection" {
		writeSkillError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	switch request.Method {
	case http.MethodGet:
		selection, err := handler.service.GetConversationSelection(
			request.Context(), user.ID, parts[0],
		)
		if err != nil {
			writeSkillServiceError(writer, err)
			return
		}
		writeSkillJSON(writer, http.StatusOK, map[string]any{"selection": selection})
	case http.MethodPut:
		var input struct {
			Revision        int64    `json:"revision"`
			InstallationIDs []string `json:"installationIds"`
		}
		if !decodeSkillJSON(writer, request, &input) {
			return
		}
		selection, err := handler.service.ReplaceConversationSelection(
			request.Context(), user.ID, parts[0], input.Revision, input.InstallationIDs,
		)
		if err != nil {
			writeSkillServiceError(writer, err)
			return
		}
		writeSkillJSON(writer, http.StatusOK, map[string]any{"selection": selection})
	default:
		methodNotAllowed(writer, "GET, PUT")
	}
}

func (handler *Handler) handleCandidate(writer http.ResponseWriter, request *http.Request, suffix string) {
	parts := strings.Split(suffix, "/")
	user := auth.UserOrDevelopment(request.Context())
	if len(parts) == 1 {
		switch parts[0] {
		case SourceOfficial:
			handler.handleOfficialCandidate(writer, request, user.ID)
		case SourceLobeHub:
			handler.handleLobeHubCandidate(writer, request, user.ID)
		case SourceGit:
			handler.handleGitCandidate(writer, request, user.ID)
		case SourceZIP:
			handler.handleZIPCandidate(writer, request, user.ID)
		default:
			if request.Method != http.MethodGet {
				methodNotAllowed(writer, http.MethodGet)
				return
			}
			candidate, err := handler.service.GetCandidate(request.Context(), user.ID, parts[0])
			if err != nil {
				writeSkillServiceError(writer, err)
				return
			}
			writeSkillJSON(writer, http.StatusOK, map[string]any{"candidate": candidate})
		}
		return
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "review" {
		if request.Method != http.MethodPost {
			methodNotAllowed(writer, http.MethodPost)
			return
		}
		var input ReviewInput
		if !decodeSkillJSON(writer, request, &input) {
			return
		}
		candidate, err := handler.service.ReviewCandidate(request.Context(), user.ID, parts[0], input)
		if err != nil {
			writeSkillServiceError(writer, err)
			return
		}
		writeSkillJSON(writer, http.StatusOK, map[string]any{"candidate": candidate})
		return
	}
	writeSkillError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
}

func (handler *Handler) handleOfficialCandidate(writer http.ResponseWriter, request *http.Request, userID string) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var input struct {
		Identifier string `json:"identifier"`
		Version    string `json:"version"`
	}
	if !decodeSkillJSON(writer, request, &input) {
		return
	}
	candidate, err := handler.service.IngestOfficial(request.Context(), userID, input.Identifier, input.Version)
	writeCandidateResult(writer, candidate, err)
}

func (handler *Handler) handleLobeHubCandidate(writer http.ResponseWriter, request *http.Request, userID string) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var input struct {
		Identifier string `json:"identifier"`
		Version    string `json:"version"`
	}
	if !decodeSkillJSON(writer, request, &input) {
		return
	}
	candidate, err := handler.service.IngestLobeHub(request.Context(), userID, input.Identifier, input.Version)
	writeCandidateResult(writer, candidate, err)
}

func (handler *Handler) handleGitCandidate(writer http.ResponseWriter, request *http.Request, userID string) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var input struct {
		RepositoryURL string `json:"repositoryUrl"`
		Commit        string `json:"commit"`
		Subdirectory  string `json:"subdirectory"`
	}
	if !decodeSkillJSON(writer, request, &input) {
		return
	}
	candidate, err := handler.service.IngestGit(request.Context(), userID,
		input.RepositoryURL, input.Commit, input.Subdirectory)
	writeCandidateResult(writer, candidate, err)
}

func (handler *Handler) handleZIPCandidate(writer http.ResponseWriter, request *http.Request, userID string) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	if mediaType := strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0]); mediaType != "application/zip" {
		writeSkillError(writer, http.StatusUnsupportedMediaType, "INVALID_SKILL_ARCHIVE", "skill archive must be application/zip")
		return
	}
	data, err := io.ReadAll(io.LimitReader(request.Body, MaxSourceArchiveBytes+1))
	if err != nil || len(data) == 0 || int64(len(data)) > MaxSourceArchiveBytes {
		writeSkillError(writer, http.StatusBadRequest, "INVALID_SKILL_ARCHIVE", "skill archive is invalid")
		return
	}
	candidate, err := handler.service.IngestZIP(request.Context(), userID, data)
	writeCandidateResult(writer, candidate, err)
}

func (handler *Handler) handleStore(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	page, pageOK := skillQueryInt(request, "page", 1)
	pageSize, pageSizeOK := skillQueryInt(request, "pageSize", 20)
	if !pageOK || !pageSizeOK {
		writeSkillError(writer, http.StatusBadRequest, "INVALID_SKILL_STORE_QUERY", "skill Store query is invalid")
		return
	}
	result, err := handler.service.ListStore(request.Context(), page, pageSize)
	if err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writeSkillJSON(writer, http.StatusOK, result)
}

func (handler *Handler) handleStoreItem(writer http.ResponseWriter, request *http.Request, suffix string) {
	parts := strings.Split(suffix, "/")
	if len(parts) < 1 || len(parts) > 2 || parts[0] == "" {
		writeSkillError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if len(parts) == 1 && request.Method == http.MethodGet {
		candidate, err := handler.service.GetStoreItem(request.Context(), parts[0])
		if err != nil {
			writeSkillServiceError(writer, err)
			return
		}
		writeSkillJSON(writer, http.StatusOK, map[string]any{"skill": candidate})
		return
	}
	if len(parts) != 2 || parts[1] != "install" {
		writeSkillError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var input struct {
		PackageFingerprint string `json:"packageFingerprint"`
	}
	if !decodeSkillJSON(writer, request, &input) {
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	installation, err := handler.service.Install(request.Context(), user.ID, parts[0], input.PackageFingerprint)
	if err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writeSkillJSON(writer, http.StatusCreated, map[string]any{"skill": installation})
}

func (handler *Handler) handleLibrary(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	items, err := handler.service.ListLibrary(request.Context(), user.ID)
	if err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writeSkillJSON(writer, http.StatusOK, map[string]any{"skills": items})
}

func (handler *Handler) handleLibraryItem(writer http.ResponseWriter, request *http.Request, installationID string) {
	if strings.Contains(installationID, "/") || installationID == "" {
		writeSkillError(writer, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if request.Method != http.MethodDelete {
		methodNotAllowed(writer, http.MethodDelete)
		return
	}
	revision, err := strconv.ParseInt(strings.TrimSpace(request.URL.Query().Get("revision")), 10, 64)
	if err != nil || revision < 1 {
		writeSkillError(writer, http.StatusBadRequest, "INVALID_SKILL_UNINSTALL", "skill uninstall is invalid")
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	if err := handler.service.Uninstall(request.Context(), user.ID, installationID, revision); err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func writeCandidateResult(writer http.ResponseWriter, candidate Candidate, err error) {
	if err != nil {
		writeSkillServiceError(writer, err)
		return
	}
	writeSkillJSON(writer, http.StatusCreated, map[string]any{"candidate": candidate})
}

func decodeSkillJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	data, err := io.ReadAll(io.LimitReader(request.Body, maxSkillRequestJSONSize+1))
	if err != nil || strictjson.Decode(data, int(maxSkillRequestJSONSize), destination) != nil {
		writeSkillError(writer, http.StatusBadRequest, "INVALID_JSON", "request body is invalid")
		return false
	}
	return true
}

func skillQueryInt(request *http.Request, name string, fallback int) (int, bool) {
	value := strings.TrimSpace(request.URL.Query().Get(name))
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

func writeSkillServiceError(writer http.ResponseWriter, err error) {
	var validation ValidationError
	switch {
	case errors.As(err, &validation):
		writeSkillError(writer, http.StatusBadRequest, validation.Code, validation.Message)
	case errors.Is(err, ErrAdministratorNeeded):
		writeSkillError(writer, http.StatusForbidden, "SKILL_ADMIN_REQUIRED", "skill administrator access is required")
	case errors.Is(err, ErrCandidateNotFound), errors.Is(err, ErrInstallationNotFound):
		writeSkillError(writer, http.StatusNotFound, "SKILL_NOT_FOUND", "skill record was not found")
	case errors.Is(err, ErrAdmissionDenied):
		writeSkillError(writer, http.StatusForbidden, "SKILL_NOT_ADMITTED", "skill is not admitted")
	case errors.Is(err, ErrAdmissionIneligible):
		writeSkillError(writer, http.StatusConflict, "SKILL_ADMISSION_INELIGIBLE", "skill is not eligible for admission")
	case errors.Is(err, ErrSourceDrift):
		writeSkillError(writer, http.StatusConflict, "SKILL_SOURCE_DRIFT", "immutable skill source changed")
	case errors.Is(err, ErrPackageCollision):
		writeSkillError(writer, http.StatusConflict, "SKILL_PACKAGE_COLLISION", "skill package identity collided")
	case errors.Is(err, ErrPackageChanged):
		writeSkillError(writer, http.StatusConflict, "SKILL_PACKAGE_CHANGED", "skill package changed; review it again")
	case errors.Is(err, ErrRevisionConflict):
		writeSkillError(writer, http.StatusConflict, "SKILL_REVISION_CONFLICT", "skill changed; reload before writing")
	case errors.Is(err, ErrSelectionInvalid):
		writeSkillError(writer, http.StatusBadRequest, "SKILL_SELECTION_INVALID", "skill conversation selection is invalid")
	case errors.Is(err, ErrInstallationConflict):
		writeSkillError(writer, http.StatusConflict, "SKILL_ALREADY_INSTALLED", "skill is already installed")
	case errors.Is(err, ErrInvalidSource), errors.Is(err, ErrArchiveInvalid), errors.Is(err, ErrManifestInvalid):
		writeSkillError(writer, http.StatusBadRequest, "INVALID_SKILL_PACKAGE", "skill package is invalid")
	case errors.Is(err, ErrSourceUnavailable):
		writeSkillError(writer, http.StatusBadGateway, "SKILL_SOURCE_UNAVAILABLE", "skill source is unavailable")
	default:
		writeSkillError(writer, http.StatusServiceUnavailable, "SKILL_SUPPLY_UNAVAILABLE", "skill supply chain is unavailable")
	}
}

func methodNotAllowed(writer http.ResponseWriter, allow string) {
	writer.Header().Set("Allow", allow)
	writeSkillError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func writeSkillError(writer http.ResponseWriter, status int, code, message string) {
	writeSkillJSON(writer, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func writeSkillJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", contentTypeJSON)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
