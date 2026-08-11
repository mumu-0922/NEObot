package mcprunner

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const maxRunnerRequestBytes = 128 << 10

type Handler struct {
	manager *Manager
	token   string
}

type listToolsRequest struct {
	ServerID string `json:"serverId"`
}

type callToolRequest struct {
	ServerID  string         `json:"serverId"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func NewHandler(manager *Manager, token string) (*Handler, error) {
	token = strings.TrimSpace(token)
	if manager == nil || len(token) < 32 || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return nil, ErrUnavailable
	}
	return &Handler{manager: manager, token: token}, nil
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if request.URL.Path == "/healthz" {
		if request.Method != http.MethodGet || h.manager.Health() != nil {
			http.Error(writer, "unavailable", http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if !h.authorized(request) {
		writeRunnerError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	switch request.URL.Path {
	case "/internal/v1/tools/list":
		if request.Method != http.MethodPost {
			writeRunnerError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		var input listToolsRequest
		if !decodeRunnerJSON(writer, request, &input) || !validRunnerIdentifier(input.ServerID) {
			return
		}
		tools, err := h.manager.ListTools(request.Context(), input.ServerID)
		if err != nil {
			writeManagerError(writer, err)
			return
		}
		writeRunnerJSON(writer, http.StatusOK, map[string]any{"tools": tools})
	case "/internal/v1/tools/call":
		if request.Method != http.MethodPost {
			writeRunnerError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
			return
		}
		var input callToolRequest
		if !decodeRunnerJSON(writer, request, &input) || !validRunnerIdentifier(input.ServerID) ||
			strings.TrimSpace(input.Name) == "" || len(input.Name) > 256 {
			writeRunnerError(writer, http.StatusBadRequest, "INVALID_REQUEST")
			return
		}
		if input.Arguments == nil {
			input.Arguments = map[string]any{}
		}
		result, err := h.manager.CallTool(request.Context(), input.ServerID, input.Name, input.Arguments)
		if err != nil {
			writeManagerError(writer, err)
			return
		}
		writeRunnerJSON(writer, http.StatusOK, map[string]any{"result": result})
	default:
		writeRunnerError(writer, http.StatusNotFound, "NOT_FOUND")
	}
}

func (h *Handler) authorized(request *http.Request) bool {
	presented := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
	return len(presented) == len(h.token) && subtle.ConstantTimeCompare([]byte(presented), []byte(h.token)) == 1
}

func decodeRunnerJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	defer request.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxRunnerRequestBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeRunnerError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeRunnerError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return false
	}
	return true
}

func validRunnerIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' ||
			char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func writeManagerError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrServerNotApproved):
		writeRunnerError(writer, http.StatusNotFound, "SERVER_NOT_APPROVED")
	case errors.Is(err, ErrCapacity):
		writeRunnerError(writer, http.StatusServiceUnavailable, "CAPACITY_EXHAUSTED")
	default:
		writeRunnerError(writer, http.StatusBadGateway, "SERVER_UNAVAILABLE")
	}
}

func writeRunnerError(writer http.ResponseWriter, status int, code string) {
	writeRunnerJSON(writer, status, map[string]any{"error": map[string]string{"code": code}})
}

func writeRunnerJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
