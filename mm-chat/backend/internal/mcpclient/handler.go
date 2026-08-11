package mcpclient

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	mcpContentTypeJSON = "application/json; charset=utf-8"
	maxMCPAPIBodyBytes = 1 << 20
)

type Handler struct {
	service *Service
}

type serverResponse struct {
	Server serverView `json:"server"`
}

type serverView struct {
	Server
	Tools []Tool `json:"tools"`
}

type serversResponse struct {
	Servers []serverView `json:"servers"`
}

type credentialRequest struct {
	Value          string `json:"value"`
	ConversationID string `json:"conversationId,omitempty"`
}

type oauthStartRequest struct {
	ServerRef      ServerRef `json:"serverRef"`
	ConversationID string    `json:"conversationId,omitempty"`
	ReturnURL      string    `json:"returnUrl"`
}

type oauthRevokeRequest struct {
	ServerRef ServerRef `json:"serverRef"`
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if h == nil || h.service == nil {
		writeMCPError(writer, http.StatusServiceUnavailable, "MCP_DISABLED", "Tools are unavailable")
		return
	}
	path := strings.TrimSuffix(request.URL.Path, "/")
	switch {
	case path == "/v1/mcp/oauth/callback":
		h.handleOAuthCallback(writer, request)
	case path == "/v1/mcp/oauth/start":
		h.handleOAuthStart(writer, request)
	case path == "/v1/mcp/oauth/revoke":
		h.handleOAuthRevoke(writer, request)
	case path == "/v1/mcp/servers":
		h.handleServers(writer, request)
	case strings.HasPrefix(path, "/v1/mcp/servers/"):
		h.handleServerAction(writer, request, strings.TrimPrefix(path, "/v1/mcp/servers/"))
	case strings.HasPrefix(path, "/v1/mcp/conversations/") && strings.HasSuffix(path, "/selection"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/mcp/conversations/"), "/selection")
		h.handleConversationSelection(writer, request, id)
	case strings.HasPrefix(path, "/v1/mcp/conversations/") && strings.HasSuffix(path, "/calls"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/mcp/conversations/"), "/calls")
		h.handleConversationCalls(writer, request, id)
	case strings.HasPrefix(path, "/v1/mcp/workspaces/") && strings.HasSuffix(path, "/selection"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/mcp/workspaces/"), "/selection")
		h.handleWorkspaceSelection(writer, request, id)
	default:
		writeMCPError(writer, http.StatusNotFound, "NOT_FOUND", "Route not found")
	}
}

func (h *Handler) handleConversationCalls(writer http.ResponseWriter, request *http.Request, conversationID string) {
	if request.Method != http.MethodGet {
		writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	calls, err := h.service.ListCalls(
		request.Context(), user.ID, conversationID, strings.TrimSpace(request.URL.Query().Get("runId")),
	)
	if err != nil {
		writeMCPServiceError(writer, err)
		return
	}
	writeMCPJSON(writer, http.StatusOK, map[string]any{"calls": calls})
}

func (h *Handler) handleServers(writer http.ResponseWriter, request *http.Request) {
	user := auth.UserOrDevelopment(request.Context())
	switch request.Method {
	case http.MethodGet:
		conversationID := strings.TrimSpace(request.URL.Query().Get("conversationId"))
		servers, err := h.service.ListServers(request.Context(), user.ID, conversationID)
		if err != nil {
			writeMCPServiceError(writer, err)
			return
		}
		views := make([]serverView, 0, len(servers))
		for _, server := range servers {
			views = append(views, viewServer(server))
		}
		writeMCPJSON(writer, http.StatusOK, serversResponse{Servers: views})
	case http.MethodPost:
		var input struct {
			Name        string   `json:"name"`
			EndpointURL string   `json:"endpointUrl"`
			AuthType    string   `json:"authType"`
			HeaderName  string   `json:"headerName,omitempty"`
			ClientID    string   `json:"clientId,omitempty"`
			Scopes      []string `json:"scopes,omitempty"`
		}
		if !decodeMCPJSON(writer, request, &input) {
			return
		}
		server, err := h.service.CreatePrivateServer(request.Context(), user.ID, CreateServerInput{
			Name: input.Name, EndpointURL: input.EndpointURL, AuthType: input.AuthType,
			HeaderName: input.HeaderName, ClientID: input.ClientID, Scopes: input.Scopes,
		})
		if err != nil {
			writeMCPServiceError(writer, err)
			return
		}
		writeMCPJSON(writer, http.StatusCreated, serverResponse{Server: viewServer(server)})
	default:
		writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
	}
}

func (h *Handler) handleServerAction(writer http.ResponseWriter, request *http.Request, suffix string) {
	parts := strings.Split(suffix, "/")
	if len(parts) < 2 || len(parts) > 3 {
		writeMCPError(writer, http.StatusNotFound, "NOT_FOUND", "Route not found")
		return
	}
	ref := ServerRef{Source: parts[0], ID: parts[1]}
	user := auth.UserOrDevelopment(request.Context())
	if len(parts) == 2 && request.Method == http.MethodDelete && ref.Source == SourcePrivate {
		if err := h.service.DeletePrivateServer(request.Context(), user.ID, ref.ID); err != nil {
			writeMCPServiceError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) != 3 {
		writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	switch parts[2] {
	case "validate":
		if request.Method != http.MethodPost || ref.Source != SourcePrivate {
			writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
			return
		}
		server, err := h.service.ValidatePrivateServer(request.Context(), user.ID, ref.ID)
		if err != nil {
			writeMCPServiceError(writer, err)
			return
		}
		writeMCPJSON(writer, http.StatusOK, serverResponse{Server: viewServer(server)})
	case "credential":
		switch request.Method {
		case http.MethodPut:
			var input credentialRequest
			if !decodeMCPJSON(writer, request, &input) {
				return
			}
			server, err := h.service.SetHeaderCredential(
				request.Context(), user.ID, input.ConversationID, ref, input.Value,
			)
			if err != nil {
				writeMCPServiceError(writer, err)
				return
			}
			writeMCPJSON(writer, http.StatusOK, serverResponse{Server: viewServer(server)})
		case http.MethodDelete:
			conversationID := strings.TrimSpace(request.URL.Query().Get("conversationId"))
			if err := h.service.DeleteCredential(request.Context(), user.ID, conversationID, ref); err != nil {
				writeMCPServiceError(writer, err)
				return
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		}
	default:
		writeMCPError(writer, http.StatusNotFound, "NOT_FOUND", "Route not found")
	}
}

func (h *Handler) handleConversationSelection(writer http.ResponseWriter, request *http.Request, conversationID string) {
	user := auth.UserOrDevelopment(request.Context())
	switch request.Method {
	case http.MethodGet:
		selection, err := h.service.GetSelection(request.Context(), user.ID, conversationID)
		if err != nil {
			writeMCPServiceError(writer, err)
			return
		}
		writeMCPJSON(writer, http.StatusOK, map[string]any{"selection": viewConversationSelection(selection)})
	case http.MethodPut:
		var input struct {
			Mode     string            `json:"mode"`
			Revision int64             `json:"revision"`
			Servers  []SelectionServer `json:"servers"`
		}
		if !decodeMCPJSON(writer, request, &input) {
			return
		}
		selection, err := h.service.ReplaceSelection(request.Context(), user.ID, Selection{
			ConversationID: conversationID, Mode: input.Mode,
			Revision: input.Revision, Servers: input.Servers,
		})
		if err != nil {
			writeMCPServiceError(writer, err)
			return
		}
		writeMCPJSON(writer, http.StatusOK, map[string]any{"selection": viewConversationSelection(selection)})
	default:
		writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
	}
}

func (h *Handler) handleWorkspaceSelection(writer http.ResponseWriter, request *http.Request, workspaceID string) {
	user := auth.UserOrDevelopment(request.Context())
	switch request.Method {
	case http.MethodGet:
		selection, err := h.service.GetWorkspaceSelection(request.Context(), user.ID, workspaceID)
		if err != nil {
			writeMCPServiceError(writer, err)
			return
		}
		writeMCPJSON(writer, http.StatusOK, map[string]any{"selection": viewWorkspaceSelection(selection)})
	case http.MethodPut:
		var input struct {
			Revision int64             `json:"revision"`
			Servers  []SelectionServer `json:"servers"`
		}
		if !decodeMCPJSON(writer, request, &input) {
			return
		}
		selection, err := h.service.ReplaceWorkspaceSelection(request.Context(), user.ID, WorkspaceSelection{
			WorkspaceID: workspaceID, Revision: input.Revision, Servers: input.Servers,
		})
		if err != nil {
			writeMCPServiceError(writer, err)
			return
		}
		writeMCPJSON(writer, http.StatusOK, map[string]any{"selection": viewWorkspaceSelection(selection)})
	default:
		writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
	}
}

func (h *Handler) handleOAuthStart(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var input oauthStartRequest
	if !decodeMCPJSON(writer, request, &input) {
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	result, err := h.service.StartOAuth(
		request.Context(), user.ID, input.ConversationID, input.ServerRef, input.ReturnURL,
	)
	if err != nil {
		writeMCPServiceError(writer, err)
		return
	}
	writeMCPJSON(writer, http.StatusOK, result)
}

func (h *Handler) handleOAuthCallback(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	if providerError := strings.TrimSpace(request.URL.Query().Get("error")); providerError != "" {
		writeMCPError(writer, http.StatusBadRequest, "MCP_OAUTH_FAILED", "Authorization failed")
		return
	}
	result, err := h.service.CompleteOAuth(
		request.Context(), request.URL.Query().Get("state"), request.URL.Query().Get("code"),
	)
	if err != nil {
		writeMCPServiceError(writer, err)
		return
	}
	http.Redirect(writer, request, result.ReturnURL, http.StatusSeeOther)
}

func (h *Handler) handleOAuthRevoke(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeMCPError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
		return
	}
	var input oauthRevokeRequest
	if !decodeMCPJSON(writer, request, &input) {
		return
	}
	user := auth.UserOrDevelopment(request.Context())
	if err := h.service.RevokeOAuth(request.Context(), user.ID, input.ServerRef); err != nil {
		writeMCPServiceError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func viewServer(server Server) serverView {
	tools := make([]Tool, len(server.Tools))
	copy(tools, server.Tools)
	server.Tools = nil
	return serverView{Server: server, Tools: tools}
}

func viewConversationSelection(selection Selection) Selection {
	selection.Servers = viewSelectionServers(selection.Servers)
	return selection
}

func viewWorkspaceSelection(selection WorkspaceSelection) WorkspaceSelection {
	selection.Servers = viewSelectionServers(selection.Servers)
	return selection
}

func viewSelectionServers(servers []SelectionServer) []SelectionServer {
	result := make([]SelectionServer, len(servers))
	for index, server := range servers {
		result[index] = server
		result[index].DisabledTools = append([]string{}, server.DisabledTools...)
	}
	return result
}

func decodeMCPJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	defer request.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxMCPAPIBodyBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeMCPError(writer, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeMCPError(writer, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid")
		return false
	}
	return true
}

func writeMCPServiceError(writer http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "MCP_INTERNAL", "Tools request failed"
	switch {
	case errors.Is(err, ErrDisabled):
		status, code, message = http.StatusServiceUnavailable, "MCP_DISABLED", "Tools are disabled"
	case errors.Is(err, ErrRemoteDisabled), errors.Is(err, ErrStdioDisabled):
		status, code, message = http.StatusServiceUnavailable, "MCP_TRANSPORT_DISABLED", "Tool transport is disabled"
	case errors.Is(err, ErrServerNotFound), errors.Is(err, ErrToolNotFound):
		status, code, message = http.StatusNotFound, "MCP_NOT_FOUND", "Tool server or tool was not found"
	case errors.Is(err, ErrServerLimit), errors.Is(err, ErrSelectionLimit):
		status, code, message = http.StatusConflict, "MCP_LIMIT_REACHED", "Tools limit was reached"
	case errors.Is(err, ErrServerConflict):
		status, code, message = http.StatusConflict, "MCP_CONFLICT", "Tool server already exists"
	case errors.Is(err, ErrSelectionInvalid), errors.Is(err, ErrToolArgumentsInvalid),
		errors.Is(err, ErrCredentialInvalid), errors.Is(err, ErrURLBlocked),
		errors.Is(err, ErrOAuthStateInvalid), errors.Is(err, ErrOAuthStateExpired),
		errors.Is(err, ErrOAuthStateConsumed):
		status, code, message = http.StatusBadRequest, "MCP_INVALID", "Tools request is invalid"
	case errors.Is(err, ErrCredentialRequired), errors.Is(err, ErrServerNeedsAuth):
		status, code, message = http.StatusConflict, "MCP_AUTH_REQUIRED", "Tool server authorization is required"
	case errors.Is(err, ErrServerNotReady), errors.Is(err, ErrServerUnavailable):
		status, code, message = http.StatusServiceUnavailable, "MCP_SERVER_UNAVAILABLE", "Tool server is unavailable"
	}
	writeMCPError(writer, status, code, message)
}

func writeMCPError(writer http.ResponseWriter, status int, code, message string) {
	writeMCPJSON(writer, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeMCPJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", mcpContentTypeJSON)
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
