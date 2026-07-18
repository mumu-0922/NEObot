package websearch

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

const (
	SearchPath            = "/v1/search"
	searchContentTypeJSON = "application/json; charset=utf-8"
	maxSearchRequestBytes = 16 << 10
)

type Handler struct {
	service *Service
}

type SearchRequest struct {
	Query      string `json:"query"`
	Scope      Scope  `json:"scope,omitempty"`
	MaxResults int    `json:"maxResults,omitempty"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService(nil)
	}
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != SearchPath {
		writeSearchError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeSearchError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	var request SearchRequest
	if err := decodeSearchRequest(w, r, &request); err != nil {
		writeSearchError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	result, err := h.service.Search(r.Context(), Request{
		Query: request.Query, Scope: request.Scope, MaxResults: request.MaxResults,
	})
	if err != nil {
		writeSearchServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeSearchJSON(w, http.StatusOK, result)
}

func decodeSearchRequest(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxSearchRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeSearchServiceError(w http.ResponseWriter, err error) {
	var providerError *ProviderError
	switch {
	case errors.Is(err, ErrInvalidRequest):
		writeSearchError(w, http.StatusBadRequest, "INVALID_SEARCH_REQUEST", "search request is invalid")
	case errors.Is(err, ErrNotConfigured):
		writeSearchError(w, http.StatusServiceUnavailable, "SEARCH_NOT_CONFIGURED", "web search is not configured")
	case errors.Is(err, ErrResolutionFailed):
		writeSearchError(w, http.StatusServiceUnavailable, "SEARCH_RESOLUTION_FAILED", "web search provider is unavailable")
	case errors.Is(err, ErrInvalidConfig):
		writeSearchError(w, http.StatusServiceUnavailable, "SEARCH_CONFIG_INVALID", "web search configuration is invalid")
	case errors.Is(err, ErrModelBuiltInRequiresChat):
		writeSearchError(
			w,
			http.StatusConflict,
			"MODEL_BUILTIN_SEARCH_REQUIRES_CHAT",
			"model built-in search requires chat execution",
		)
	case errors.As(err, &providerError):
		writeSearchError(w, http.StatusBadGateway, "SEARCH_PROVIDER_ERROR", "web search provider failed")
	default:
		writeSearchError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "web search request failed")
	}
}

func writeSearchJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", searchContentTypeJSON)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeSearchError(w http.ResponseWriter, status int, code string, message string) {
	writeSearchJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}
