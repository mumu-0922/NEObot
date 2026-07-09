package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const (
	contentTypeJSON     = "application/json; charset=utf-8"
	maxAuthRequestBytes = 64 << 10
	authLoginPath       = "/v1/auth/login"
	authLogoutPath      = "/v1/auth/logout"
	mePath              = "/v1/me"
)

type Handler struct {
	service *Service
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type LoginRequest struct {
	Token string `json:"token"`
}

type LoginResponse struct {
	User      CurrentUserDTO `json:"user"`
	Token     string         `json:"token"`
	ExpiresAt string         `json:"expiresAt"`
}

type CurrentUserDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService(nil)
	}
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case authLoginPath:
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.login(w, r)
	case authLogoutPath:
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		h.logout(w, r)
	case mePath:
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		h.me(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var request LoginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_AUTH_PAYLOAD", "auth request body is invalid")
		return
	}
	result, err := h.service.Login(r.Context(), LoginInput{
		Token:     request.Token,
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, LoginResponse{
		User:      newCurrentUserDTO(result.User),
		Token:     result.Token,
		ExpiresAt: result.ExpiresAt.UTC().Format(timeFormatRFC3339Nano),
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok || token == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "session is required")
		return
	}
	if err := h.service.Logout(r.Context(), token); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, newCurrentUserDTO(h.service.CurrentUser(r.Context())))
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) == nil {
		return errors.New("multiple JSON values")
	}
	return nil
}

func newCurrentUserDTO(user User) CurrentUserDTO {
	user = UserOrDevelopment(WithUser(nil, user))
	return CurrentUserDTO{ID: user.ID, DisplayName: user.DisplayName, Role: user.Role}
}

func writeServiceError(w http.ResponseWriter, err error) {
	status, body := serviceErrorFor(err)
	writeError(w, status, body.Code, body.Message)
}

func serviceErrorFor(err error) (int, ErrorBody) {
	switch {
	case errors.Is(err, ErrDatabaseRequired):
		return http.StatusServiceUnavailable, ErrorBody{Code: "DATABASE_REQUIRED", Message: "database is required for auth endpoints"}
	case errors.Is(err, ErrAuthNotConfigured):
		return http.StatusServiceUnavailable, ErrorBody{Code: "AUTH_NOT_CONFIGURED", Message: "auth bootstrap token is not configured"}
	case errors.Is(err, ErrInvalidCredential):
		return http.StatusUnauthorized, ErrorBody{Code: "INVALID_CREDENTIALS", Message: "invalid credentials"}
	case errors.Is(err, ErrSessionNotFound), errors.Is(err, ErrSessionExpired), errors.Is(err, ErrSessionRevoked):
		return http.StatusUnauthorized, ErrorBody{Code: "UNAUTHENTICATED", Message: "session is invalid or expired"}
	default:
		return http.StatusInternalServerError, ErrorBody{Code: "INTERNAL_ERROR", Message: "internal server error"}
	}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func bearerToken(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return "", false
	}
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", true
	}
	return parts[1], true
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
