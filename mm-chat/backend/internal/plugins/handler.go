package plugins

import (
	"encoding/json"
	"net/http"
)

const contentTypeJSON = "application/json; charset=utf-8"

type Handler struct{}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ListResponse struct {
	Plugins     []PluginSummary `json:"plugins"`
	Unavailable bool            `json:"unavailable,omitempty"`
}

type PluginSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	LogoURL     string `json:"logoUrl,omitempty"`
	ManifestURL string `json:"manifestUrl,omitempty"`
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/plugins":
		h.handlePlugins(w, r)
	case "/v1/plugins/install":
		h.requireMethod(w, r, http.MethodPost, h.installPlugin)
	case "/v1/plugins/execute":
		h.requireMethod(w, r, http.MethodPost, h.executePlugin)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, ListResponse{
		Plugins:     []PluginSummary{},
		Unavailable: true,
	})
}

func (h *Handler) installPlugin(w http.ResponseWriter, _ *http.Request) {
	writeError(
		w,
		http.StatusNotImplemented,
		"PLUGIN_INSTALL_UNAVAILABLE",
		"plugin install is not available in the Go backend yet",
	)
}

func (h *Handler) executePlugin(w http.ResponseWriter, _ *http.Request) {
	writeError(
		w,
		http.StatusNotImplemented,
		"PLUGIN_EXECUTION_UNAVAILABLE",
		"plugin execution is not available in the Go backend yet",
	)
}

func (h *Handler) requireMethod(
	w http.ResponseWriter,
	r *http.Request,
	method string,
	next func(http.ResponseWriter, *http.Request),
) {
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	next(w, r)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}
