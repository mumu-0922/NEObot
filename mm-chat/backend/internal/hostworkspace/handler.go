package hostworkspace

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"neo-chat/mm-chat/backend/internal/agenthost"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	workspacePath        = "/v1/workspaces"
	workspacePathPrefix  = workspacePath + "/"
	maxWorkspaceAPIBytes = (5 << 20) / 4
)

type Handler struct{ service *Service }

type workspaceDTO struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	SystemPrompt         string          `json:"systemPrompt,omitempty"`
	Files                []WorkspaceFile `json:"files"`
	Color                string          `json:"color,omitempty"`
	EnableSearch         *bool           `json:"enableSearch,omitempty"`
	EnableReasoning      *bool           `json:"enableReasoning,omitempty"`
	Revision             int64           `json:"revision"`
	BindingStatus        string          `json:"bindingStatus"`
	RunnerID             string          `json:"runnerId,omitempty"`
	CanonicalPath        string          `json:"canonicalPath,omitempty"`
	DisplayPath          string          `json:"displayPath,omitempty"`
	PathKind             string          `json:"pathKind,omitempty"`
	DirectoryFingerprint string          `json:"directoryFingerprint,omitempty"`
	BoundAt              string          `json:"boundAt,omitempty"`
	LegacyImportedAt     string          `json:"legacyImportedAt,omitempty"`
	CreatedAt            string          `json:"createdAt"`
	UpdatedAt            string          `json:"updatedAt"`
}

type settingsRequest struct {
	Name             string          `json:"name"`
	SystemPrompt     string          `json:"systemPrompt"`
	Files            []WorkspaceFile `json:"files"`
	Color            string          `json:"color"`
	EnableSearch     *bool           `json:"enableSearch"`
	EnableReasoning  *bool           `json:"enableReasoning"`
	ExpectedRevision int64           `json:"expectedRevision,omitempty"`
}

type bindRequest struct {
	ExpectedRevision int64  `json:"expectedRevision"`
	Path             string `json:"path"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if request.URL.RawQuery != "" {
		writeWorkspaceError(writer, http.StatusBadRequest, "INVALID_WORKSPACE_REQUEST", "Request is invalid")
		return
	}
	path := strings.TrimSuffix(request.URL.Path, "/")
	switch {
	case path == workspacePath:
		handler.collection(writer, request)
	case strings.HasPrefix(path, workspacePathPrefix):
		handler.item(writer, request, strings.TrimPrefix(path, workspacePathPrefix))
	default:
		writeWorkspaceError(writer, http.StatusNotFound, "NOT_FOUND", "Route not found")
	}
}

func (handler *Handler) collection(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		methodNotAllowed(writer, http.MethodGet)
		return
	}
	items, err := handler.service.List(request.Context())
	if err != nil {
		writeWorkspaceServiceError(writer, err)
		return
	}
	views := make([]workspaceDTO, 0, len(items))
	for _, item := range items {
		views = append(views, viewWorkspace(item))
	}
	writeWorkspaceJSON(writer, http.StatusOK, map[string]any{"workspaces": views})
}

func (handler *Handler) item(writer http.ResponseWriter, request *http.Request, suffix string) {
	parts := strings.Split(suffix, "/")
	if len(parts) == 1 {
		handler.workspace(writer, request, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "bind" {
		handler.bind(writer, request, parts[0])
		return
	}
	if len(parts) == 3 && parts[1] == "conversations" {
		handler.setConversation(writer, request, parts[0], parts[2])
		return
	}
	writeWorkspaceError(writer, http.StatusNotFound, "NOT_FOUND", "Route not found")
}

func (handler *Handler) workspace(writer http.ResponseWriter, request *http.Request, workspaceID string) {
	switch request.Method {
	case http.MethodGet:
		workspace, err := handler.service.Get(request.Context(), workspaceID)
		if err != nil {
			writeWorkspaceServiceError(writer, err)
			return
		}
		writeWorkspaceJSON(writer, http.StatusOK, map[string]any{"workspace": viewWorkspace(workspace)})
	case http.MethodPut:
		var input settingsRequest
		if decodeWorkspaceJSON(writer, request, &input) != nil {
			writeWorkspaceError(writer, http.StatusBadRequest, "INVALID_WORKSPACE_REQUEST", "Request is invalid")
			return
		}
		workspace, err := handler.service.ImportLegacy(request.Context(), workspaceID, input.settings())
		if err != nil {
			writeWorkspaceServiceError(writer, err)
			return
		}
		writeWorkspaceJSON(writer, http.StatusOK, map[string]any{"workspace": viewWorkspace(workspace)})
	case http.MethodPatch:
		var input settingsRequest
		if decodeWorkspaceJSON(writer, request, &input) != nil {
			writeWorkspaceError(writer, http.StatusBadRequest, "INVALID_WORKSPACE_REQUEST", "Request is invalid")
			return
		}
		workspace, err := handler.service.UpdateSettings(
			request.Context(), workspaceID, input.ExpectedRevision, input.settings(),
		)
		if err != nil {
			writeWorkspaceServiceError(writer, err)
			return
		}
		writeWorkspaceJSON(writer, http.StatusOK, map[string]any{"workspace": viewWorkspace(workspace)})
	case http.MethodDelete:
		var input struct {
			ExpectedRevision int64 `json:"expectedRevision"`
		}
		if decodeWorkspaceJSON(writer, request, &input) != nil {
			writeWorkspaceError(writer, http.StatusBadRequest, "INVALID_WORKSPACE_REQUEST", "Request is invalid")
			return
		}
		if err := handler.service.Delete(request.Context(), workspaceID, input.ExpectedRevision); err != nil {
			writeWorkspaceServiceError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(writer, http.MethodGet+", "+http.MethodPut+", "+http.MethodPatch+", "+http.MethodDelete)
	}
}

func (handler *Handler) bind(writer http.ResponseWriter, request *http.Request, workspaceID string) {
	if request.Method != http.MethodPost {
		methodNotAllowed(writer, http.MethodPost)
		return
	}
	var input bindRequest
	if decodeWorkspaceJSON(writer, request, &input) != nil {
		writeWorkspaceError(writer, http.StatusBadRequest, "INVALID_WORKSPACE_REQUEST", "Request is invalid")
		return
	}
	workspace, err := handler.service.Bind(
		request.Context(), workspaceID, input.ExpectedRevision, input.Path,
	)
	if err != nil {
		writeWorkspaceServiceError(writer, err)
		return
	}
	writeWorkspaceJSON(writer, http.StatusOK, map[string]any{"workspace": viewWorkspace(workspace)})
}

func (handler *Handler) setConversation(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID string,
	conversationID string,
) {
	if request.Method != http.MethodPut {
		methodNotAllowed(writer, http.MethodPut)
		return
	}
	if request.Body != nil && request.ContentLength != 0 {
		writeWorkspaceError(writer, http.StatusBadRequest, "INVALID_WORKSPACE_REQUEST", "Request is invalid")
		return
	}
	if err := handler.service.SetConversationWorkspace(
		request.Context(), conversationID, workspaceID,
	); err != nil {
		writeWorkspaceServiceError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (input settingsRequest) settings() Settings {
	return Settings{
		Name: input.Name, SystemPrompt: input.SystemPrompt, Files: input.Files,
		Color: input.Color, EnableSearch: input.EnableSearch, EnableReasoning: input.EnableReasoning,
	}
}

func viewWorkspace(workspace Workspace) workspaceDTO {
	status := "unbound"
	if workspace.Bound() {
		status = "bound"
	}
	view := workspaceDTO{
		ID: workspace.ID, Name: workspace.Settings.Name,
		SystemPrompt: workspace.Settings.SystemPrompt, Files: workspace.Settings.Files,
		Color: workspace.Settings.Color, EnableSearch: workspace.Settings.EnableSearch,
		EnableReasoning: workspace.Settings.EnableReasoning, Revision: workspace.Revision,
		BindingStatus: status, RunnerID: workspace.RunnerID, CanonicalPath: workspace.CanonicalPath,
		DisplayPath: workspace.DisplayPath, PathKind: workspace.PathKind,
		DirectoryFingerprint: workspace.DirectoryFingerprint,
		CreatedAt:            workspace.CreatedAt.UTC().Format(timeFormat),
		UpdatedAt:            workspace.UpdatedAt.UTC().Format(timeFormat),
	}
	if workspace.BoundAt != nil {
		view.BoundAt = workspace.BoundAt.UTC().Format(timeFormat)
	}
	if workspace.LegacyImportedAt != nil {
		view.LegacyImportedAt = workspace.LegacyImportedAt.UTC().Format(timeFormat)
	}
	if view.Files == nil {
		view.Files = []WorkspaceFile{}
	}
	return view
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func decodeWorkspaceJSON(writer http.ResponseWriter, request *http.Request, output any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return ErrInvalid
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxWorkspaceAPIBytes)
	data, err := io.ReadAll(request.Body)
	if err != nil {
		return err
	}
	return strictjson.Decode(data, maxWorkspaceAPIBytes, output)
}

func writeWorkspaceServiceError(writer http.ResponseWriter, err error) {
	var remoteError agenthost.RemoteError
	switch {
	case errors.Is(err, ErrDisabled), errors.Is(err, agenthost.ErrHostUnavailable):
		writeWorkspaceError(
			writer, http.StatusServiceUnavailable,
			"HOST_WORKSPACE_UNAVAILABLE", "Host Workspace is unavailable",
		)
	case errors.As(err, &remoteError):
		writeWorkspaceError(
			writer, http.StatusBadGateway,
			"HOST_WORKSPACE_RESOLVE_FAILED", "Host Workspace resolution failed",
		)
	case errors.Is(err, agenthost.ErrHostProtocol):
		writeWorkspaceError(
			writer, http.StatusBadGateway,
			"HOST_WORKSPACE_PROTOCOL_INVALID", "Host Workspace resolution failed",
		)
	case errors.Is(err, ErrInvalid):
		writeWorkspaceError(writer, http.StatusBadRequest, "INVALID_WORKSPACE_REQUEST", "Request is invalid")
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrConversationNotFound):
		writeWorkspaceError(writer, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "Workspace was not found")
	case errors.Is(err, ErrRevisionConflict):
		writeWorkspaceError(writer, http.StatusConflict, "WORKSPACE_REVISION_CONFLICT", "Workspace changed; reload and retry")
	case errors.Is(err, ErrAlreadyBound):
		writeWorkspaceError(writer, http.StatusConflict, "WORKSPACE_ALREADY_BOUND", "Workspace is already bound")
	case errors.Is(err, ErrDirectoryAlreadyRegistered):
		writeWorkspaceError(writer, http.StatusConflict, "WORKSPACE_DIRECTORY_REGISTERED", "Directory is already registered")
	case errors.Is(err, ErrConversationLocked):
		writeWorkspaceError(writer, http.StatusConflict, "CONVERSATION_WORKSPACE_LOCKED", "Conversation Workspace is locked")
	case errors.Is(err, ErrWorkspaceUnbound):
		writeWorkspaceError(writer, http.StatusConflict, "WORKSPACE_UNBOUND", "Workspace has no Host directory")
	case errors.Is(err, ErrWorkspaceInUse):
		writeWorkspaceError(writer, http.StatusConflict, "WORKSPACE_IN_USE", "Workspace is in use")
	default:
		writeWorkspaceError(writer, http.StatusInternalServerError, "WORKSPACE_INTERNAL_ERROR", "Workspace request failed")
	}
}

func methodNotAllowed(writer http.ResponseWriter, allowed string) {
	writer.Header().Set("Allow", allowed)
	writeWorkspaceError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
}

func writeWorkspaceError(writer http.ResponseWriter, status int, code, message string) {
	response := errorResponse{}
	response.Error.Code = code
	response.Error.Message = message
	writeWorkspaceJSON(writer, status, response)
}

func writeWorkspaceJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
