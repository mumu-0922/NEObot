package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

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

type options struct {
	readyChecker         health.ReadinessChecker
	chatRepository       chat.Repository
	chatProvider         chat.Provider
	runCancellationStore chat.RunCancellationStore
	rateLimitStore       ratelimit.Store
	fileRepository       files.Repository
	objectStore          storage.ObjectStore
	maxUploadBytes       int64
}

func WithReadyChecker(checker health.ReadinessChecker) Option {
	return func(opts *options) {
		opts.readyChecker = checker
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

func WithMaxUploadBytes(maxUploadBytes int64) Option {
	return func(opts *options) {
		opts.maxUploadBytes = maxUploadBytes
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
	mux := http.NewServeMux()
	healthHandler := health.New(cfg.Version, resolvedOptions.readyChecker)
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

	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.HandleFunc("/v1/version", healthHandler.Version)
	mux.Handle("/v1/chat/conversations", chatHandler)
	mux.Handle("/v1/chat/conversations/", chatHandler)
	mux.Handle("/v1/chat/runs/", chatHandler)
	mux.Handle("/v1/files", fileHandler)
	mux.Handle("/v1/files/", fileHandler)
	mux.HandleFunc("/", notFound)

	middlewares := []Middleware{withRecover, withSecurityHeaders}
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
