package agentrunner

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const rpcPath = "/internal/neo-runner/v1/rpc"

type HTTPHandler struct {
	service       *Service
	maxSkew       time.Duration
	allowedCaller string
	requests      chan struct{}
}

func NewHTTPHandler(service *Service, maxSkew time.Duration, allowedCaller string) (http.Handler, error) {
	if service == nil || maxSkew < time.Second || maxSkew > time.Minute ||
		!identityPattern.MatchString(allowedCaller) {
		return nil, ErrInvalidInput
	}
	return &HTTPHandler{service: service, maxSkew: maxSkew, allowedCaller: allowedCaller,
		requests: make(chan struct{}, 32)}, nil
}

func (handler *HTTPHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	select {
	case handler.requests <- struct{}{}:
		defer func() { <-handler.requests }()
	default:
		writer.Header().Set("Retry-After", "1")
		writeHTTPError(writer, http.StatusServiceUnavailable, ErrorRuntimeUnavailable)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	if request.URL.Path != rpcPath {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeHTTPError(writer, http.StatusMethodNotAllowed, ErrorInvalidTransition)
		return
	}
	if request.TLS == nil || len(request.TLS.VerifiedChains) != 1 || len(request.TLS.VerifiedChains[0]) < 1 {
		writeHTTPError(writer, http.StatusUnauthorized, ErrorAuthFailed)
		return
	}
	caller := strings.TrimSpace(request.TLS.VerifiedChains[0][0].Subject.CommonName)
	if !identityPattern.MatchString(caller) || caller != handler.allowedCaller {
		writeHTTPError(writer, http.StatusUnauthorized, ErrorAuthFailed)
		return
	}
	if mediaType := strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0]); mediaType != "application/json" {
		writeHTTPError(writer, http.StatusUnsupportedMediaType, ErrorInvalidTransition)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxRPCBytes))
	if err != nil {
		writeHTTPError(writer, http.StatusRequestEntityTooLarge, ErrorInvalidTransition)
		return
	}
	decoded, err := DecodeRequest(body, time.Now(), handler.maxSkew)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrVersionUnsupported) {
			status = http.StatusUpgradeRequired
		}
		writeHTTPError(writer, status, ErrorCode(err))
		return
	}
	response, operationErr := handler.service.Handle(request.Context(), caller, decoded)
	status := http.StatusOK
	if operationErr != nil {
		status = http.StatusConflict
		if errors.Is(operationErr, ErrAuthFailed) {
			status = http.StatusUnauthorized
		} else if errors.Is(operationErr, ErrIsolationUnavailable) || errors.Is(operationErr, ErrRuntimeUnavailable) {
			status = http.StatusServiceUnavailable
		}
	}
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(response)
}

func writeHTTPError(writer http.ResponseWriter, status int, code string) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]any{"error": RPCError{Code: code, Retryable: false}})
}

func RPCPath() string { return rpcPath }
