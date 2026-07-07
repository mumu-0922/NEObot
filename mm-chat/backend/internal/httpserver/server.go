package httpserver

import (
	"encoding/json"
	"net/http"
	"time"

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

func New(cfg config.Config) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           NewHandler(cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func NewHandler(cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	healthHandler := health.New(cfg.Version)

	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/ready", healthHandler.Ready)
	mux.HandleFunc("/v1/version", healthHandler.Version)
	mux.HandleFunc("/", notFound)

	return chain(mux, withRecover, withSecurityHeaders)
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
