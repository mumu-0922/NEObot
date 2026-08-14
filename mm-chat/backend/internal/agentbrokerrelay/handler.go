package agentbrokerrelay

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/agentrunner"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

const maxRelayBytes = 256 << 10

type Handler struct {
	target         Target
	authority      agentrunner.AuthorityVerifier
	runnerIdentity string
	canaryIdentity string
	runnerID       string
	requests       chan struct{}
	now            func() time.Time
}

func NewHandler(target Target, authority agentrunner.AuthorityVerifier, runnerIdentity,
	canaryIdentity, runnerID string,
) (http.Handler, error) {
	if target == nil || authority == nil || !validIdentity(runnerIdentity) ||
		!validIdentity(canaryIdentity) || !validIdentity(runnerID) ||
		runnerIdentity == canaryIdentity {
		return nil, agentrunner.ErrInvalidInput
	}
	return &Handler{target: target, authority: authority, runnerIdentity: runnerIdentity,
		canaryIdentity: canaryIdentity, runnerID: runnerID, requests: make(chan struct{}, 16),
		now: time.Now}, nil
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	if request.URL.Path != Path {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		writeError(writer, http.StatusMethodNotAllowed, agentrunner.ErrorInvalidTransition)
		return
	}
	if !handler.authorizedTLS(request.TLS) {
		writeError(writer, http.StatusUnauthorized, agentrunner.ErrorAuthFailed)
		return
	}
	mediaType := strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0])
	if mediaType != "application/json" {
		writeError(writer, http.StatusUnsupportedMediaType, agentrunner.ErrorInvalidTransition)
		return
	}
	select {
	case handler.requests <- struct{}{}:
		defer func() { <-handler.requests }()
	default:
		writeError(writer, http.StatusServiceUnavailable, agentrunner.ErrorRuntimeUnavailable)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxRelayBytes))
	if err != nil {
		writeError(writer, http.StatusRequestEntityTooLarge, agentrunner.ErrorInvalidTransition)
		return
	}
	var incoming envelope
	if strictjson.Decode(body, maxRelayBytes, &incoming) != nil || incoming.SchemaVersion != ProtocolVersion ||
		(incoming.Method != agentrunner.MethodPrepare && incoming.Method != agentrunner.MethodCommit) {
		writeError(writer, http.StatusBadRequest, agentrunner.ErrorVersionUnsupported)
		return
	}
	result, operationErr := handler.dispatch(request, incoming)
	if operationErr != nil {
		status := http.StatusConflict
		if errors.Is(operationErr, agentrunner.ErrAuthFailed) || errors.Is(operationErr, agentrunner.ErrLeaseStale) {
			status = http.StatusUnauthorized
		} else if errors.Is(operationErr, agentrunner.ErrRuntimeUnavailable) ||
			errors.Is(operationErr, agentrunner.ErrExecutorUnavailable) {
			status = http.StatusServiceUnavailable
		}
		writeError(writer, status, relayErrorCode(operationErr))
		return
	}
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(result)
}

func (handler *Handler) dispatch(request *http.Request, incoming envelope) (response, error) {
	now := handler.now().UTC()
	switch incoming.Method {
	case agentrunner.MethodPrepare:
		var body agentrunner.PrepareRequest
		if strictjson.Decode(incoming.Body, maxRelayBytes, &body) != nil {
			return response{}, agentrunner.ErrInvalidTransition
		}
		if _, err := agentrunner.NewRequest(agentrunner.MethodPrepare, body, now); err != nil {
			return response{}, err
		}
		if err := handler.verifyPrepare(body, now); err != nil {
			return response{}, err
		}
		result, err := handler.target.Prepare(request.Context(), body)
		if err != nil {
			return response{}, err
		}
		return response{SchemaVersion: ProtocolVersion, Method: incoming.Method, Prepare: &result}, nil
	case agentrunner.MethodCommit:
		var body agentrunner.CommitRequest
		if strictjson.Decode(incoming.Body, maxRelayBytes, &body) != nil {
			return response{}, agentrunner.ErrInvalidTransition
		}
		if _, err := agentrunner.NewRequest(agentrunner.MethodCommit, body, now); err != nil {
			return response{}, err
		}
		if err := handler.verifyCommit(body, now); err != nil {
			return response{}, err
		}
		result, err := handler.target.Commit(request.Context(), body)
		if err != nil {
			return response{}, err
		}
		return response{SchemaVersion: ProtocolVersion, Method: incoming.Method, Commit: &result}, nil
	default:
		return response{}, agentrunner.ErrVersionUnsupported
	}
}

func (handler *Handler) verifyPrepare(body agentrunner.PrepareRequest, now time.Time) error {
	ticket := body.Authority
	if ticket.CallerIdentity != handler.canaryIdentity {
		return agentrunner.ErrAuthFailed
	}
	return handler.authority.Verify(handler.canaryIdentity, handler.runnerID, agentrunner.MethodPrepare,
		ticket.RequestID, ticket.Nonce, agentrunner.AuthorityRequestFingerprint(agentrunner.MethodPrepare, body),
		body.SnapshotFingerprint, body.Attempt, ticket, now)
}

func (handler *Handler) verifyCommit(body agentrunner.CommitRequest, now time.Time) error {
	ticket := body.Authority
	if ticket.CallerIdentity != handler.canaryIdentity {
		return agentrunner.ErrAuthFailed
	}
	return handler.authority.Verify(handler.canaryIdentity, handler.runnerID, agentrunner.MethodCommit,
		ticket.RequestID, ticket.Nonce, agentrunner.AuthorityRequestFingerprint(agentrunner.MethodCommit, body),
		body.SnapshotFingerprint, body.Attempt, ticket, now)
}

func (handler *Handler) authorizedTLS(state *tls.ConnectionState) bool {
	return handler != nil && state != nil && len(state.VerifiedChains) == 1 && len(state.VerifiedChains[0]) > 0 &&
		strings.TrimSpace(state.VerifiedChains[0][0].Subject.CommonName) == handler.runnerIdentity
}

func writeError(writer http.ResponseWriter, status int, code string) {
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(response{SchemaVersion: ProtocolVersion,
		Error: &agentrunner.RPCError{Code: code, Retryable: false}})
}

func relayErrorCode(err error) string {
	for _, mapping := range []struct {
		err  error
		code string
	}{
		{agentbroker.ErrGrantDenied, agentrunner.ErrorGrantDenied},
		{agentbroker.ErrLeaseStale, agentrunner.ErrorLeaseStale},
		{agentbroker.ErrSnapshotMismatch, agentrunner.ErrorSnapshotMismatch},
		{agentbroker.ErrKillSwitchActive, agentrunner.ErrorKillSwitchActive},
		{agentbroker.ErrBudgetExhausted, agentrunner.ErrorBudgetExhausted},
		{agentbroker.ErrApprovalRequired, agentrunner.ErrorApprovalRequired},
		{agentbroker.ErrApprovalDenied, agentrunner.ErrorApprovalDenied},
		{agentbroker.ErrIntentExpired, agentrunner.ErrorIntentExpired},
		{agentbroker.ErrReplayDetected, agentrunner.ErrorReplayDetected},
		{agentbroker.ErrArtifactDenied, agentrunner.ErrorArtifactDenied},
		{agentbroker.ErrExecutorUnavailable, agentrunner.ErrorExecutorUnavailable},
		{agentbroker.ErrInvalidTransition, agentrunner.ErrorInvalidTransition},
		{agentbroker.ErrOutcomeUnknown, agentrunner.ErrorOutcomeUnknown},
	} {
		if errors.Is(err, mapping.err) {
			return mapping.code
		}
	}
	return agentrunner.ErrorCode(err)
}

func validIdentity(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if index == 0 && !((character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z')) {
			return false
		}
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("_.:@/-", character) {
			continue
		}
		return false
	}
	return true
}
