package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/health"
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
	readyChecker   health.ReadinessChecker
	chatRepository chat.Repository
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
	chatHandler := chat.NewHandler(chat.NewService(resolvedOptions.chatRepository))

	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.HandleFunc("/v1/version", healthHandler.Version)
	mux.Handle("/v1/chat/conversations", chatHandler)
	mux.Handle("/v1/chat/conversations/", chatHandler)
	mux.HandleFunc("/", notFound)

	return chain(mux, withRecover, withSecurityHeaders)
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
