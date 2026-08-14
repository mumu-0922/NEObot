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
	callerMethods map[string]map[string]struct{}
	requests      chan struct{}
}

type CallerPolicy struct {
	Identity string
	Methods  []string
}

func NewHTTPHandler(service *Service, maxSkew time.Duration, allowedCaller string) (http.Handler, error) {
	return NewHTTPHandlerWithPolicies(service, maxSkew, []CallerPolicy{{
		Identity: allowedCaller,
		Methods: []string{MethodProbe, MethodLaunch, MethodHeartbeat, MethodCancel,
			MethodPrepare, MethodCommit, MethodList, MethodReconcile},
	}})
}

func NewHTTPHandlerWithPolicies(service *Service, maxSkew time.Duration, policies []CallerPolicy) (http.Handler, error) {
	if service == nil || maxSkew < time.Second || maxSkew > time.Minute || len(policies) < 1 || len(policies) > 8 {
		return nil, ErrInvalidInput
	}
	callerMethods := make(map[string]map[string]struct{}, len(policies))
	for _, policy := range policies {
		if !identityPattern.MatchString(policy.Identity) || len(policy.Methods) < 1 || callerMethods[policy.Identity] != nil {
			return nil, ErrInvalidInput
		}
		methods := make(map[string]struct{}, len(policy.Methods))
		for _, method := range policy.Methods {
			if !member(method, MethodProbe, MethodLaunch, MethodHeartbeat, MethodCancel,
				MethodPrepare, MethodCommit, MethodList, MethodReconcile) {
				return nil, ErrInvalidInput
			}
			if _, duplicate := methods[method]; duplicate {
				return nil, ErrInvalidInput
			}
			methods[method] = struct{}{}
		}
		callerMethods[policy.Identity] = methods
	}
	return &HTTPHandler{service: service, maxSkew: maxSkew, callerMethods: callerMethods,
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
	methods := handler.callerMethods[caller]
	if !identityPattern.MatchString(caller) || methods == nil {
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
	if _, allowed := methods[decoded.Method]; !allowed {
		writeHTTPError(writer, http.StatusForbidden, ErrorAuthFailed)
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
