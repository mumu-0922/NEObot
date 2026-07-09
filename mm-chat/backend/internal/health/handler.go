package health

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	contentTypeJSON = "application/json; charset=utf-8"
	readyTimeout    = 2 * time.Second
)

type ReadinessChecker interface {
	CheckReady(ctx context.Context) error
}

type Check struct {
	Name    string
	Checker ReadinessChecker
}

type Handler struct {
	version string
	checks  []Check
}

type StatusResponse struct {
	Status string                `json:"status"`
	Checks map[string]CheckState `json:"checks,omitempty"`
	Error  *ErrorBody            `json:"error,omitempty"`
}

type CheckState struct {
	Status string `json:"status"`
}

type VersionResponse struct {
	Version string `json:"version"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func New(version string, readyChecker ...ReadinessChecker) *Handler {
	var checks []Check
	if len(readyChecker) > 0 && readyChecker[0] != nil {
		checks = append(checks, Check{Name: "database", Checker: readyChecker[0]})
	}

	return NewWithChecks(version, checks...)
}

func NewWithChecks(version string, checks ...Check) *Handler {
	if version == "" {
		version = "dev"
	}

	return &Handler{version: version, checks: normalizeChecks(checks)}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{Status: "healthy"})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	if len(h.checks) > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()

		checks, ready := h.runChecks(ctx)
		if !ready {
			writeJSON(w, http.StatusServiceUnavailable, StatusResponse{
				Status: "not_ready",
				Checks: checks,
				Error: &ErrorBody{
					Code:    "DEPENDENCY_NOT_READY",
					Message: "one or more readiness checks failed",
				},
			})
			return
		}

		writeJSON(w, http.StatusOK, StatusResponse{Status: "ready", Checks: checks})
		return
	}

	writeJSON(w, http.StatusOK, StatusResponse{Status: "ready"})
}

func (h *Handler) runChecks(ctx context.Context) (map[string]CheckState, bool) {
	checks := make(map[string]CheckState, len(h.checks))
	ready := true

	for _, check := range h.checks {
		state := CheckState{Status: "ready"}
		if check.Checker != nil {
			if err := check.Checker.CheckReady(ctx); err != nil {
				state.Status = "not_ready"
				ready = false
			}
		}
		checks[check.Name] = state
	}

	return checks, ready
}

func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	writeJSON(w, http.StatusOK, VersionResponse{Version: h.version})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}

	w.Header().Set("Allow", method)
	writeJSON(w, http.StatusMethodNotAllowed, ErrorResponse{
		Error: ErrorBody{
			Code:    "METHOD_NOT_ALLOWED",
			Message: "method not allowed",
		},
	})
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}

func normalizeChecks(checks []Check) []Check {
	normalized := make([]Check, 0, len(checks))
	used := map[string]int{}
	for _, check := range checks {
		name := normalizeCheckName(check.Name)
		if name == "" {
			name = "dependency"
		}
		used[name]++
		if used[name] > 1 {
			name = name + "-" + strconv.Itoa(used[name])
		}
		normalized = append(normalized, Check{Name: name, Checker: check.Checker})
	}

	return normalized
}

func normalizeCheckName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		case r == ' ' || r == '.':
			builder.WriteRune('-')
		default:
			builder.WriteRune('-')
		}
	}

	return strings.Trim(builder.String(), "-_")
}
