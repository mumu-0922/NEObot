package voicejobs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"neo-chat/mm-chat/backend/internal/jobaudit"
)

const (
	contentTypeJSON       = "application/json; charset=utf-8"
	transcribePath        = "/v1/voice/transcribe"
	synthesizePath        = "/v1/voice/synthesize"
	maxJSONRequestBytes   = 16 << 10
	maxMultipartBodyBytes = 25 << 20
	maxMultipartMemory    = 8 << 20
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
		service = NewService()
	}
	return &Handler{service: service}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case transcribePath:
		h.requireMethod(w, r, http.MethodPost, h.transcribe)
	case synthesizePath:
		h.requireMethod(w, r, http.MethodPost, h.synthesize)
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

func (h *Handler) transcribe(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMultipartBodyBytes)
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}

	file, fileHeader, err := r.FormFile("audio")
	if err != nil {
		writeError(w, http.StatusBadRequest, "AUDIO_REQUIRED", "audio file is required")
		return
	}
	_ = file.Close()
	if fileHeader == nil || fileHeader.Size <= 0 {
		writeError(w, http.StatusBadRequest, "AUDIO_REQUIRED", "audio file is required")
		return
	}

	provider, err := parseProvider(r.FormValue("provider"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_VOICE_PROVIDER", "voice provider is not supported")
		return
	}

	response, err := h.service.Transcribe(r.Context(), TranscribeRequest{
		Provider: provider,
		ModelID:  strings.TrimSpace(r.FormValue("modelId")),
		Language: strings.TrimSpace(r.FormValue("language")),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) synthesize(w http.ResponseWriter, r *http.Request) {
	var request SynthesizeRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request body is invalid")
		return
	}

	request.Text = strings.TrimSpace(request.Text)
	request.VoiceID = strings.TrimSpace(request.VoiceID)
	request.ModelID = strings.TrimSpace(request.ModelID)
	provider, err := parseProvider(string(request.Provider))
	if err != nil {
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_VOICE_PROVIDER", "voice provider is not supported")
		return
	}
	request.Provider = provider
	if request.Text == "" {
		writeError(w, http.StatusBadRequest, "TEXT_REQUIRED", "text is required")
		return
	}

	response, err := h.service.Synthesize(r.Context(), request)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func parseProvider(value string) (Provider, error) {
	switch Provider(strings.ToLower(strings.TrimSpace(value))) {
	case ProviderDefault:
		return ProviderDefault, nil
	case ProviderElevenLabs:
		return ProviderElevenLabs, nil
	case ProviderMimo:
		return ProviderMimo, nil
	case ProviderModel:
		return ProviderModel, nil
	default:
		return "", errors.New("unsupported voice provider")
	}
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
	case errors.Is(err, jobaudit.ErrAuditUnavailable):
		writeError(w, http.StatusServiceUnavailable, "JOB_AUDIT_UNAVAILABLE", "job audit is unavailable")
	case errors.Is(err, ErrVoiceJobsUnavailable):
		writeError(w, http.StatusNotImplemented, "VOICE_JOBS_UNAVAILABLE", "voice jobs are not configured")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusRequestTimeout, "REQUEST_CANCELLED", "request was cancelled")
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "voice job request failed")
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
