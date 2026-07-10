package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/ratelimit"
)

const (
	contentTypeJSON          = "application/json; charset=utf-8"
	maxAuthRequestBytes      = 8 << 10
	authLoginPath            = "/v1/auth/login"
	authLogoutPath           = "/v1/auth/logout"
	authInviteAcceptPath     = "/v1/auth/invites/accept"
	authRecoveryRequestPath  = "/v1/auth/recovery/request"
	authRecoveryCompletePath = "/v1/auth/recovery/complete"
	mePath                   = "/v1/me"
	meSessionsPath           = "/v1/me/sessions"

	defaultAuthRateLimitEntries = 10_000
)

type Handler struct {
	service            *Service
	primaryRateLimiter ratelimit.Store
	localRateLimiter   ratelimit.Store
	now                func() time.Time
}

type HandlerOption func(*Handler)

func WithAuthRateLimitStore(store ratelimit.Store) HandlerOption {
	return func(handler *Handler) {
		handler.primaryRateLimiter = store
	}
}

func withAuthHandlerClock(now func() time.Time) HandlerOption {
	return func(handler *Handler) {
		if now != nil {
			handler.now = now
		}
	}
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AcceptInviteRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

type RecoveryRequest struct {
	Email string `json:"email"`
}

type RecoveryCompleteRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

type RecoveryAcceptedResponse struct {
	Status string `json:"status"`
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

func NewHandler(service *Service, opts ...HandlerOption) *Handler {
	if service == nil {
		service = NewService(nil)
	}
	handler := &Handler{
		service:          service,
		localRateLimiter: ratelimit.NewMemoryStore(defaultAuthRateLimitEntries),
		now:              time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	if handler.now == nil {
		handler.now = time.Now
	}
	return handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case authLoginPath:
		h.requireMethod(w, r, http.MethodPost, h.login)
	case authInviteAcceptPath:
		h.requireMethod(w, r, http.MethodPost, h.acceptInvite)
	case authRecoveryRequestPath:
		h.requireMethod(w, r, http.MethodPost, h.requestRecovery)
	case authRecoveryCompletePath:
		h.requireMethod(w, r, http.MethodPost, h.completeRecovery)
	case authLogoutPath:
		h.requireMethod(w, r, http.MethodPost, h.logout)
	case meSessionsPath:
		h.requireMethod(w, r, http.MethodDelete, h.revokeAllSessions)
	case mePath:
		h.requireMethod(w, r, http.MethodGet, h.me)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) requireMethod(
	w http.ResponseWriter,
	r *http.Request,
	method string,
	next func(http.ResponseWriter, *http.Request),
) {
	if r.Method != method {
		methodNotAllowed(w, method)
		return
	}
	next(w, r)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var request LoginRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	if !h.allowPublicAuth(w, r, authEmailRateSubject(request.Email), loginRateLimitPolicy) {
		return
	}
	result, err := h.service.Login(r.Context(), LoginInput{
		Email:     request.Email,
		Password:  request.Password,
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeLoginResponse(w, result)
}

func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var request AcceptInviteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	if !h.allowPublicAuth(w, r, HashSessionToken(request.Token), tokenRateLimitPolicy) {
		return
	}
	result, err := h.service.AcceptInvite(r.Context(), AcceptInviteInput{
		Token:     request.Token,
		Password:  request.Password,
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeLoginResponse(w, result)
}

func (h *Handler) requestRecovery(w http.ResponseWriter, r *http.Request) {
	var request RecoveryRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	if !h.allowPublicAuth(w, r, authEmailRateSubject(request.Email), recoveryRequestRateLimitPolicy) {
		return
	}
	if err := h.service.RequestRecovery(r.Context(), RecoveryRequestInput{Email: request.Email}); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, RecoveryAcceptedResponse{Status: "accepted"})
}

func (h *Handler) completeRecovery(w http.ResponseWriter, r *http.Request) {
	var request RecoveryCompleteRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	if !h.allowPublicAuth(w, r, HashSessionToken(request.Token), tokenRateLimitPolicy) {
		return
	}
	if err := h.service.CompleteRecovery(r.Context(), RecoveryCompleteInput{
		Token:       request.Token,
		NewPassword: request.NewPassword,
	}); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

func (h *Handler) revokeAllSessions(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RevokeAllSessions(r.Context()); err != nil {
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
		return classifyDecodeError(err)
	}
	if decoder.Decode(&struct{}{}) == nil {
		return errors.New("multiple JSON values")
	}
	return nil
}

type forbiddenIdentityFieldError struct{ field string }

func (e *forbiddenIdentityFieldError) Error() string {
	return fmt.Sprintf("forbidden identity field %q", e.field)
}

func classifyDecodeError(err error) error {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return maxBytesErr
	}
	const unknownFieldPrefix = "json: unknown field \""
	message := err.Error()
	if strings.HasPrefix(message, unknownFieldPrefix) && strings.HasSuffix(message, "\"") {
		field := strings.TrimSuffix(strings.TrimPrefix(message, unknownFieldPrefix), "\"")
		if isForbiddenIdentityField(field) {
			return &forbiddenIdentityFieldError{field: field}
		}
	}
	return err
}

func isForbiddenIdentityField(field string) bool {
	field = strings.ToLower(strings.TrimSpace(field))
	for _, forbidden := range []string{
		"userid", "user_id", "role", "teamid", "teamids", "sessionid",
		"owneruserid", "acl", "aclgroups", "impersonate", "impersonation",
	} {
		if field == forbidden {
			return true
		}
	}
	return strings.Contains(field, "revision") || strings.Contains(field, "epoch")
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	var forbiddenErr *forbiddenIdentityFieldError
	switch {
	case errors.As(err, &maxBytesErr):
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "auth request body is too large")
	case errors.As(err, &forbiddenErr):
		writeError(w, http.StatusBadRequest, "FORBIDDEN_IDENTITY_FIELD", "caller identity fields are not accepted")
	default:
		writeError(w, http.StatusBadRequest, "INVALID_AUTH_PAYLOAD", "auth request body is invalid")
	}
}

func newCurrentUserDTO(user User) CurrentUserDTO {
	user = UserOrDevelopment(WithUser(nil, user))
	return CurrentUserDTO{ID: user.ID, DisplayName: user.DisplayName, Role: user.Role}
}

func writeLoginResponse(w http.ResponseWriter, result LoginResult) {
	writeJSON(w, http.StatusOK, LoginResponse{
		User:      newCurrentUserDTO(result.User),
		Token:     result.Token,
		ExpiresAt: result.ExpiresAt.UTC().Format(timeFormatRFC3339Nano),
	})
}

func writeServiceError(w http.ResponseWriter, err error) {
	status, body := serviceErrorFor(err)
	writeError(w, status, body.Code, body.Message)
}

func serviceErrorFor(err error) (int, ErrorBody) {
	switch {
	case errors.Is(err, ErrDatabaseRequired):
		return http.StatusServiceUnavailable, ErrorBody{Code: "DATABASE_REQUIRED", Message: "database is required for auth endpoints"}
	case errors.Is(err, ErrInvalidIdentityInput):
		return http.StatusBadRequest, ErrorBody{Code: "INVALID_AUTH_PAYLOAD", Message: "email or password is invalid"}
	case errors.Is(err, ErrInviteNotActive):
		return http.StatusGone, ErrorBody{Code: "INVITE_NOT_ACTIVE", Message: "invite is not active"}
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

type authRateLimitPolicy struct {
	name         string
	ipLimit      int
	subjectLimit int
	window       time.Duration
}

var (
	loginRateLimitPolicy           = authRateLimitPolicy{name: "login", ipLimit: 10, subjectLimit: 5, window: 15 * time.Minute}
	tokenRateLimitPolicy           = authRateLimitPolicy{name: "token", ipLimit: 10, subjectLimit: 5, window: 15 * time.Minute}
	recoveryRequestRateLimitPolicy = authRateLimitPolicy{name: "recovery", ipLimit: 5, subjectLimit: 3, window: time.Hour}
)

func (h *Handler) allowPublicAuth(
	w http.ResponseWriter,
	r *http.Request,
	subject string,
	policy authRateLimitPolicy,
) bool {
	client := authClientAddress(r)
	ipKey := hashAuthRateLimitKey(policy.name, "ip", client)
	subjectKey := hashAuthRateLimitKey(policy.name, "subject", strings.TrimSpace(subject))
	allowed, retryAfter := h.allowAuthKey(r, ipKey, policy.ipLimit, policy.window)
	if allowed {
		allowed, retryAfter = h.allowAuthKey(r, subjectKey, policy.subjectLimit, policy.window)
	}
	if allowed {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
	writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
	return false
}

func (h *Handler) allowAuthKey(
	r *http.Request,
	key string,
	limit int,
	window time.Duration,
) (bool, time.Duration) {
	now := h.now()
	local, err := h.localRateLimiter.Allow(r.Context(), key, limit, window, now)
	if err != nil || !local.Allowed {
		if err != nil {
			return false, window
		}
		return false, local.RetryAfter
	}
	if h.primaryRateLimiter == nil {
		return true, 0
	}
	primary, err := h.primaryRateLimiter.Allow(r.Context(), key, limit, window, now)
	if err != nil {
		return true, 0
	}
	return primary.Allowed, primary.RetryAfter
}

func hashAuthRateLimitKey(route, dimension, value string) string {
	sum := sha256.Sum256([]byte(route + ":" + dimension + ":" + value))
	return "auth:" + route + ":" + dimension + ":" + hex.EncodeToString(sum[:])
}

func authEmailRateSubject(value string) string {
	canonical, err := canonicalizeEmail(value)
	if err == nil {
		return canonical
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func authClientAddress(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	remoteAddr := r.RemoteAddr
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		peerIP := net.ParseIP(host)
		if peerIP == nil {
			return host
		}
		if peerIP != nil && peerIP.IsLoopback() {
			if forwarded := lastForwardedClientIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
				return forwarded
			}
		}
		return peerIP.String()
	}
	if strings.TrimSpace(remoteAddr) != "" {
		return strings.TrimSpace(remoteAddr)
	}
	return "unknown"
}

func lastForwardedClientIP(header string) string {
	parts := strings.Split(header, ",")
	for index := len(parts) - 1; index >= 0; index-- {
		candidate := net.ParseIP(strings.TrimSpace(parts[index]))
		if candidate != nil {
			return candidate.String()
		}
	}
	return ""
}

func retryAfterSeconds(duration time.Duration) int {
	if duration <= 0 {
		return 1
	}
	seconds := int(duration / time.Second)
	if duration%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

const timeFormatRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
