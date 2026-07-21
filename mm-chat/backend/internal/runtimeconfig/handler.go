package runtimeconfig

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

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
	if r.URL.Path == "/v1/admin/rag/providers" ||
		strings.HasPrefix(r.URL.Path, "/v1/admin/rag/providers/") {
		h.adminRAGProviderConfig(w, r)
		return
	}
	if r.URL.Path == "/v1/admin/search/providers" ||
		strings.HasPrefix(r.URL.Path, "/v1/admin/search/providers/") {
		h.adminSearchProviderConfig(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/admin/providers/") {
		h.adminProviderConfigByID(w, r)
		return
	}
	switch r.URL.Path {
	case "/v1/config":
		h.requireMethod(w, r, http.MethodGet, h.getConfig)
	case "/v1/providers/models":
		h.requireMethod(w, r, http.MethodPost, h.listProviderModels)
	case "/v1/admin/provider-config":
		switch r.Method {
		case http.MethodGet:
			h.getAdminProviderConfig(w, r)
		case http.MethodPut:
			h.updateAdminProviderConfig(w, r)
		default:
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		}
	case "/v1/admin/providers":
		h.requireMethod(w, r, http.MethodGet, h.listAdminProviderConfigs)
	case "/v1/admin/task-models":
		switch r.Method {
		case http.MethodGet:
			h.getAdminTaskModels(w, r)
		case http.MethodPatch:
			h.updateAdminTaskModels(w, r)
		default:
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPatch)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		}
	case "/v1/byok/public-key":
		h.requireMethod(w, r, http.MethodGet, h.getBYOKPublicKey)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (h *Handler) getAdminTaskModels(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.AdminTaskModelSettings(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) updateAdminTaskModels(w http.ResponseWriter, r *http.Request) {
	var request TaskModelSettingsPatch
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	response, err := h.service.UpdateAdminTaskModelSettings(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) adminRAGProviderConfig(w http.ResponseWriter, r *http.Request) {
	const collectionPath = "/v1/admin/rag/providers"
	if r.URL.Path == collectionPath {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		response, err := h.service.AdminRAGProviderConfigs(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	remainder := strings.Trim(strings.TrimPrefix(r.URL.Path, collectionPath+"/"), "/")
	parts := strings.Split(remainder, "/")
	providerID := strings.TrimSpace(parts[0])
	if providerID == "" || len(parts) > 2 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var response AdminRAGProviderConnectionResponse
		var err error
		switch strings.TrimSpace(parts[1]) {
		case "test":
			response, err = h.service.TestAdminRAGProviderConnection(r.Context(), providerID)
		case "activate":
			response, err = h.service.ActivateAdminRAGProvider(r.Context(), providerID)
		default:
			writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
			return
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var request UpdateAdminRAGProviderConfigRequest
		if err := decodeJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
			return
		}
		response, err := h.service.UpsertAdminRAGProviderConfig(
			r.Context(), providerID, request,
		)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodDelete:
		if err := h.service.DeleteAdminRAGProviderConfig(r.Context(), providerID); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", http.MethodPut+", "+http.MethodDelete)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h *Handler) adminSearchProviderConfig(w http.ResponseWriter, r *http.Request) {
	const collectionPath = "/v1/admin/search/providers"
	if r.URL.Path == collectionPath {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		response, err := h.service.AdminSearchProviderConfigs(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	remainder := strings.Trim(strings.TrimPrefix(r.URL.Path, collectionPath+"/"), "/")
	parts := strings.Split(remainder, "/")
	providerID := strings.TrimSpace(parts[0])
	if providerID == "" || len(parts) > 2 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var response AdminSearchProviderConnectionResponse
		var err error
		switch strings.TrimSpace(parts[1]) {
		case "test":
			response, err = h.service.TestAdminSearchProviderConnection(r.Context(), providerID)
		case "activate":
			response, err = h.service.ActivateAdminSearchProvider(r.Context(), providerID)
		default:
			writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
			return
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var request UpdateAdminSearchProviderConfigRequest
		if err := decodeJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
			return
		}
		response, err := h.service.UpsertAdminSearchProviderConfig(
			r.Context(), providerID, request,
		)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodDelete:
		if err := h.service.DeleteAdminSearchProviderConfig(r.Context(), providerID); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", http.MethodPut+", "+http.MethodDelete)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (h *Handler) listAdminProviderConfigs(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.AdminProviderConfigs(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) adminProviderConfigByID(w http.ResponseWriter, r *http.Request) {
	remainder := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/admin/providers/"), "/")
	parts := strings.Split(remainder, "/")
	providerID := strings.TrimSpace(parts[0])
	if providerID == "" || len(parts) > 2 {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if len(parts) == 2 {
		action := strings.TrimSpace(parts[1])
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var response AdminProviderConnectionResponse
		var err error
		switch action {
		case "test":
			response, err = h.service.TestAdminProviderConnection(r.Context(), providerID)
		case "activate":
			response, err = h.service.ActivateAdminProvider(r.Context(), providerID)
		default:
			writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
			return
		}
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var request UpdateAdminProviderConfigRequest
		if err := decodeJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
			return
		}
		response, err := h.service.UpsertAdminProviderConfig(r.Context(), providerID, request)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodDelete:
		if err := h.service.DeleteAdminProviderConfig(r.Context(), providerID); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", http.MethodPut+", "+http.MethodDelete)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
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

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, h.service.PublicConfigForContext(r.Context()))
}

func (h *Handler) listProviderModels(w http.ResponseWriter, r *http.Request) {
	var request ProviderModelsRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	response, err := h.service.ProviderModelsForContext(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) getAdminProviderConfig(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.AdminProviderConfig(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) updateAdminProviderConfig(w http.ResponseWriter, r *http.Request) {
	var request UpdateAdminProviderConfigRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	response, err := h.service.UpdateAdminProviderConfig(r.Context(), request)
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
	case errors.Is(err, ErrProviderSecretVaultUnavailable):
		writeError(w, http.StatusServiceUnavailable, "PROVIDER_SECRET_VAULT_UNAVAILABLE", "provider secret vault is unavailable")
	case errors.Is(err, ErrProviderSecretInvalid):
		writeError(w, http.StatusServiceUnavailable, "PROVIDER_SECRET_UNAVAILABLE", "stored provider secret is unavailable")
	case errors.Is(err, ErrProviderConfigUnsupported):
		writeError(w, http.StatusBadRequest, "PROVIDER_CONFIG_UNSUPPORTED", "provider configuration is unsupported")
	case errors.Is(err, ErrDatabaseRequired):
		writeError(w, http.StatusServiceUnavailable, "DATABASE_REQUIRED", "database is required for provider configuration")
	case errors.Is(err, ErrProviderConfigNotFound):
		writeError(w, http.StatusNotFound, "PROVIDER_CONFIG_NOT_FOUND", "provider configuration was not found")
	case errors.Is(err, ErrProviderDisabled):
		writeError(w, http.StatusConflict, "PROVIDER_DISABLED", "provider is disabled")
	case errors.Is(err, ErrProviderActivationRequired):
		writeError(w, http.StatusConflict, "PROVIDER_ACTIVATION_REQUIRED", "provider must pass connection testing before activation")
	case errors.Is(err, ErrProviderConnectionTestFailed):
		writeError(w, http.StatusBadGateway, "PROVIDER_CONNECTION_TEST_FAILED", "provider connection test failed")
	case errors.Is(err, ErrProviderConfigChanged):
		writeError(w, http.StatusConflict, "PROVIDER_CONFIG_CHANGED", "provider configuration changed during connection testing")
	case errors.Is(err, ErrTaskModelSettingsInvalid):
		writeError(w, http.StatusBadRequest, "TASK_MODEL_SETTINGS_INVALID", "task model settings are invalid")
	case errors.Is(err, ErrTaskModelUnavailable):
		writeError(w, http.StatusConflict, "TASK_MODEL_UNAVAILABLE", "task model is not available from an enabled provider")
	case errors.Is(err, ErrSearchProviderConfigUnsupported):
		writeError(w, http.StatusBadRequest, "SEARCH_PROVIDER_CONFIG_UNSUPPORTED", "search provider configuration is unsupported")
	case errors.Is(err, ErrSearchProviderNotFound):
		writeError(w, http.StatusNotFound, "SEARCH_PROVIDER_NOT_FOUND", "search provider configuration was not found")
	case errors.Is(err, ErrSearchProviderSecretRequired):
		writeError(w, http.StatusBadRequest, "SEARCH_PROVIDER_SECRET_REQUIRED", "search provider API key is required")
	case errors.Is(err, ErrSearchProviderConnectionFailed):
		writeError(w, http.StatusBadGateway, "SEARCH_PROVIDER_CONNECTION_TEST_FAILED", "search provider connection test failed")
	case errors.Is(err, ErrSearchProviderConfigChanged):
		writeError(w, http.StatusConflict, "SEARCH_PROVIDER_CONFIG_CHANGED", "search provider configuration changed during connection testing")
	case errors.Is(err, ErrRAGProviderConfigUnsupported):
		writeError(w, http.StatusBadRequest, "RAG_PROVIDER_CONFIG_UNSUPPORTED", "RAG provider configuration is unsupported")
	case errors.Is(err, ErrRAGProviderNotFound):
		writeError(w, http.StatusNotFound, "RAG_PROVIDER_NOT_FOUND", "RAG provider configuration was not found")
	case errors.Is(err, ErrRAGProviderSecretRequired):
		writeError(w, http.StatusBadRequest, "RAG_PROVIDER_SECRET_REQUIRED", "RAG provider API key is required")
	case errors.Is(err, ErrRAGProviderSecretVaultUnavailable):
		writeError(w, http.StatusServiceUnavailable, "RAG_PROVIDER_SECRET_VAULT_UNAVAILABLE", "RAG provider secret vault is unavailable")
	case errors.Is(err, ErrRAGProviderSecretInvalid):
		writeError(w, http.StatusServiceUnavailable, "RAG_PROVIDER_SECRET_UNAVAILABLE", "stored RAG provider secret is unavailable")
	case errors.Is(err, ErrRAGProviderConnectionFailed):
		writeError(w, http.StatusBadGateway, "RAG_PROVIDER_CONNECTION_TEST_FAILED", "RAG provider connection test failed")
	case errors.Is(err, ErrRAGProviderConfigChanged):
		writeError(w, http.StatusConflict, "RAG_PROVIDER_CONFIG_CHANGED", "RAG provider configuration changed during connection testing")
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
