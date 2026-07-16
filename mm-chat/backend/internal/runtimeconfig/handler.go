package runtimeconfig

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"neo-chat/mm-chat/backend/internal/config"
)

const (
	contentTypeJSON     = "application/json; charset=utf-8"
	maxJSONRequestBytes = 16 << 10
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

func NewHandler(service *Service) *Handler {
	if service == nil {
		service = NewService(config.Config{})
	}
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/v1/config":
		h.requireMethod(w, r, http.MethodGet, h.getConfig)
	case "/v1/providers/models":
		h.requireMethod(w, r, http.MethodPost, h.listProviderModels)
	case "/v1/byok/public-key":
		h.requireMethod(w, r, http.MethodGet, h.getBYOKPublicKey)
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
		w.Header().Set("Allow", method)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	next(w, r)
}

func (h *Handler) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.service.PublicConfig())
}

func (h *Handler) listProviderModels(w http.ResponseWriter, r *http.Request) {
	var request ProviderModelsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	response, err := h.service.ProviderModels(request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) getBYOKPublicKey(w http.ResponseWriter, _ *http.Request) {
	response, err := h.service.BYOKPublicKey()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
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

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBYOKNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "BYOK_NOT_CONFIGURED", "BYOK is not configured")
	case errors.Is(err, ErrPlaintextProviderSecret):
		writeError(w, http.StatusBadRequest, "PLAINTEXT_PROVIDER_SECRET_REJECTED", "plaintext provider secrets are not accepted")
	case errors.Is(err, ErrProviderModelsUnsupported):
		writeError(w, http.StatusNotImplemented, "PROVIDER_MODEL_LIST_UNSUPPORTED", "provider model listing is not available for this provider")
	case errors.Is(err, ErrProviderSecretRequired):
		writeError(w, http.StatusBadRequest, "PROVIDER_SECRET_REQUIRED", "provider API key is required")
	case errors.Is(err, ErrProviderConfigUnsupported):
		writeError(w, http.StatusBadRequest, "PROVIDER_CONFIG_UNSUPPORTED", "provider configuration is unsupported")
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "runtime config request failed")
	}
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
