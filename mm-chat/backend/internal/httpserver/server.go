package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/auth"
	"neo-chat/mm-chat/backend/internal/browserimport"
	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/files"
	"neo-chat/mm-chat/backend/internal/health"
	"neo-chat/mm-chat/backend/internal/ratelimit"
	"neo-chat/mm-chat/backend/internal/storage"
)

const contentTypeJSON = "application/json; charset=utf-8"

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Option func(*options)

type SessionResolver interface {
	ResolveByTokenHash(ctx context.Context, tokenHash string) (auth.Session, error)
}

type options struct {
	readyChecks          []health.Check
	logger               *slog.Logger
	chatRepository       chat.Repository
	chatProvider         chat.Provider
	runCancellationStore chat.RunCancellationStore
	rateLimitStore       ratelimit.Store
	sessionResolver      SessionResolver
	authService          *auth.Service
	fileRepository       files.Repository
	objectStore          storage.ObjectStore
	metrics              *Metrics
	dbStatsProvider      DatabaseStatsProvider
	maxUploadBytes       int64
	importRepository     browserimport.Repository
	maxImportBytes       int64
}

func WithReadyChecker(checker health.ReadinessChecker) Option {
	return func(opts *options) {
		opts.readyChecks = append(opts.readyChecks, health.Check{Name: "database", Checker: checker})
	}
}

func WithReadyCheck(name string, checker health.ReadinessChecker) Option {
	return func(opts *options) {
		opts.readyChecks = append(opts.readyChecks, health.Check{Name: name, Checker: checker})
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

func WithChatRepository(repo chat.Repository) Option {
	return func(opts *options) {
		opts.chatRepository = repo
	}
}

func WithChatProvider(provider chat.Provider) Option {
	return func(opts *options) {
		opts.chatProvider = provider
	}
}

func WithRunCancellationStore(store chat.RunCancellationStore) Option {
	return func(opts *options) {
		opts.runCancellationStore = store
	}
}

func WithRateLimitStore(store ratelimit.Store) Option {
	return func(opts *options) {
		opts.rateLimitStore = store
	}
}

func WithSessionResolver(resolver SessionResolver) Option {
	return func(opts *options) {
		opts.sessionResolver = resolver
	}
}

func WithAuthService(service *auth.Service) Option {
	return func(opts *options) {
		opts.authService = service
	}
}

func WithFileRepository(repo files.Repository) Option {
	return func(opts *options) {
		opts.fileRepository = repo
	}
}

func WithObjectStore(store storage.ObjectStore) Option {
	return func(opts *options) {
		opts.objectStore = store
	}
}

func WithMetrics(metrics *Metrics) Option {
	return func(opts *options) {
		opts.metrics = metrics
	}
}

func WithDatabaseStatsProvider(provider DatabaseStatsProvider) Option {
	return func(opts *options) {
		opts.dbStatsProvider = provider
	}
}

func WithMaxUploadBytes(maxUploadBytes int64) Option {
	return func(opts *options) {
		opts.maxUploadBytes = maxUploadBytes
	}
}

func WithBrowserImportRepository(repo browserimport.Repository) Option {
	return func(opts *options) {
		opts.importRepository = repo
	}
}

func WithMaxImportBytes(maxImportBytes int64) Option {
	return func(opts *options) {
		opts.maxImportBytes = maxImportBytes
	}
}

func New(cfg config.Config, opts ...Option) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           NewHandler(cfg, opts...),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func NewHandler(cfg config.Config, opts ...Option) http.Handler {
	resolvedOptions := resolveOptions(opts...)
	logger := resolvedOptions.logger
	if logger == nil {
		logger = slog.Default()
	}
	metrics := resolvedOptions.metrics
	if metrics == nil {
		metrics = NewMetrics(cfg.Version, cfg.Storage.Backend)
	}
	metrics.SetReadyChecks(resolvedOptions.readyChecks)
	metrics.SetDBStatsProvider(resolvedOptions.dbStatsProvider)

	mux := http.NewServeMux()
	healthHandler := health.NewWithChecks(cfg.Version, resolvedOptions.readyChecks...)
	authHandler := auth.NewHandler(resolvedOptions.authService)
	chatHandler := chat.NewHandler(
		chat.NewService(resolvedOptions.chatRepository),
		chat.WithProvider(resolvedOptions.chatProvider),
		chat.WithRunCancellationStore(resolvedOptions.runCancellationStore),
	)
	fileHandler := files.NewHandler(
		files.NewService(
			resolvedOptions.fileRepository,
			resolvedOptions.objectStore,
			files.WithStorageBackend(cfg.Storage.Backend),
		),
		files.WithMaxUploadBytes(resolvedOptions.maxUploadBytes),
	)
	importHandler := browserimport.NewHandler(
		browserimport.NewService(
			resolvedOptions.importRepository,
			browserimport.WithMaxPackageBytes(resolvedOptions.maxImportBytes),
		),
		browserimport.WithHandlerMaxPackageBytes(resolvedOptions.maxImportBytes),
	)

	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.Handle("/metrics", metrics)
	mux.HandleFunc("/v1/version", healthHandler.Version)
	mux.Handle("/v1/me", authHandler)
	mux.Handle("/v1/auth/login", authHandler)
	mux.Handle("/v1/auth/logout", authHandler)
	mux.Handle("/v1/chat/conversations", chatHandler)
	mux.Handle("/v1/chat/conversations/", chatHandler)
	mux.Handle("/v1/chat/runs/", chatHandler)
	mux.Handle("/v1/files", fileHandler)
	mux.Handle("/v1/files/", fileHandler)
	mux.Handle("/v1/import/browser", importHandler)
	mux.Handle("/v1/import/browser/", importHandler)
	mux.HandleFunc("/", notFound)

	middlewares := []Middleware{
		withRequestID,
		withRequestMetrics(metrics),
		withRequestLogging(logger),
		withRecover(logger),
		withSecurityHeaders,
	}
	authRequired := cfg.Auth.RequireAuth()
	if resolvedOptions.sessionResolver != nil || authRequired {
		middlewares = append(middlewares, withSessionIdentity(resolvedOptions.sessionResolver, authRequired))
	}
	if cfg.Redis.RateLimitEnabled && resolvedOptions.rateLimitStore != nil {
		middlewares = append(middlewares, withRateLimit(resolvedOptions.rateLimitStore, cfg.Redis, nil))
	}

	return chain(mux, middlewares...)
}

func resolveOptions(opts ...Option) options {
	resolved := options{}
	for _, opt := range opts {
		if opt != nil {
			opt(&resolved)
		}
	}

	return resolved
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, ErrorResponse{
		Error: ErrorBody{
			Code:    "NOT_FOUND",
			Message: "route not found",
		},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return
	}
}

func withSessionIdentity(resolver SessionResolver, requireAuth bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicWithoutAuthRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				if requireAuth {
					writeAuthError(w, auth.ErrSessionNotFound)
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if token == "" {
				writeAuthError(w, auth.ErrSessionNotFound)
				return
			}
			if resolver == nil {
				writeAuthError(w, auth.ErrDatabaseRequired)
				return
			}

			session, err := resolver.ResolveByTokenHash(r.Context(), auth.HashSessionToken(token))
			if err != nil {
				writeAuthError(w, err)
				return
			}

			next.ServeHTTP(w, r.WithContext(auth.WithUser(r.Context(), auth.UserFromSession(session))))
		})
	}
}

func isPublicWithoutAuthRequest(r *http.Request) bool {
	if r == nil {
		return false
	}

	switch r.URL.Path {
	case "/health", "/ready", "/metrics", "/v1/version":
		return r.Method == http.MethodGet
	case "/v1/auth/login":
		return r.Method == http.MethodPost
	default:
		return false
	}
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

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, auth.ErrDatabaseRequired):
		writeJSON(w, http.StatusServiceUnavailable, ErrorResponse{
			Error: ErrorBody{
				Code:    "DATABASE_REQUIRED",
				Message: "database is required for auth verification",
			},
		})
	case errors.Is(err, auth.ErrSessionNotFound),
		errors.Is(err, auth.ErrSessionExpired),
		errors.Is(err, auth.ErrSessionRevoked):
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{
			Error: ErrorBody{
				Code:    "UNAUTHENTICATED",
				Message: "session is invalid or expired",
			},
		})
	default:
		writeJSON(w, http.StatusUnauthorized, ErrorResponse{
			Error: ErrorBody{
				Code:    "UNAUTHENTICATED",
				Message: "session could not be verified",
			},
		})
	}
}
