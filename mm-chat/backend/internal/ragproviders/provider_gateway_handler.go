package ragproviders

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const (
	providerGatewaySmallRequestBytes     = 8 << 10
	providerGatewayEmbeddingRequestBytes = 5 << 20
)

var errProviderGatewayRequestTooLarge = errors.New("rag provider gateway request is too large")

type ProviderGatewayHandler struct {
	internalToken string
	operations    ProviderOperations
}

func NewProviderGatewayHandler(
	internalToken string,
	operations ProviderOperations,
) *ProviderGatewayHandler {
	return &ProviderGatewayHandler{
		internalToken: strings.TrimSpace(internalToken),
		operations:    operations,
	}
}

func (handler *ProviderGatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, InternalProviderPathPrefix) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if handler == nil || handler.operations == nil ||
		!validInternalToken(handler.internalToken) {
		writeError(
			w,
			http.StatusServiceUnavailable,
			"RAG_PROVIDER_UNAVAILABLE",
			"RAG provider gateway is unavailable",
		)
		return
	}
	if !providerGatewayTokenEqual(
		r.Header.Get(InternalProviderTokenHeader),
		handler.internalToken,
	) {
		writeError(
			w,
			http.StatusUnauthorized,
			"RAG_PROVIDER_GATEWAY_UNAUTHORIZED",
			"RAG provider gateway token is invalid",
		)
		return
	}
	if !isProviderGatewayPath(r.URL.Path) {
		writeError(
			w,
			http.StatusBadRequest,
			"RAG_PROVIDER_OPERATION_UNSUPPORTED",
			"RAG provider operation is unsupported",
		)
		return
	}
	if !providerGatewayJSONContentType(r.Header.Get("Content-Type")) {
		writeError(
			w,
			http.StatusBadRequest,
			"RAG_PROVIDER_REQUEST_INVALID",
			"RAG provider request is invalid",
		)
		return
	}

	switch r.URL.Path {
	case InternalMinerUAllocatePath:
		handler.serveMinerUAllocate(w, r)
	case InternalMinerUPollPath:
		handler.serveMinerUPoll(w, r)
	case InternalSiliconFlowEmbeddingsPath:
		handler.serveSiliconFlowEmbeddings(w, r)
	}
}

func (handler *ProviderGatewayHandler) serveMinerUAllocate(
	w http.ResponseWriter,
	r *http.Request,
) {
	var input MinerUAllocateRequest
	if err := decodeProviderGatewayRequest(w, r, providerGatewaySmallRequestBytes, &input); err != nil {
		writeProviderGatewayRequestError(w, err)
		return
	}
	result, err := handler.operations.AllocateMinerU(r.Context(), input)
	if err != nil {
		writeProviderGatewayOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *ProviderGatewayHandler) serveMinerUPoll(
	w http.ResponseWriter,
	r *http.Request,
) {
	var input MinerUPollRequest
	if err := decodeProviderGatewayRequest(w, r, providerGatewaySmallRequestBytes, &input); err != nil {
		writeProviderGatewayRequestError(w, err)
		return
	}
	result, err := handler.operations.PollMinerU(r.Context(), input)
	if err != nil {
		writeProviderGatewayOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handler *ProviderGatewayHandler) serveSiliconFlowEmbeddings(
	w http.ResponseWriter,
	r *http.Request,
) {
	var input PassageEmbeddingRequest
	if err := decodeProviderGatewayRequest(
		w,
		r,
		providerGatewayEmbeddingRequestBytes,
		&input,
	); err != nil {
		writeProviderGatewayRequestError(w, err)
		return
	}
	result, err := handler.operations.EmbedSiliconFlowPassages(r.Context(), input)
	if err != nil {
		writeProviderGatewayOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeProviderGatewayRequest(
	w http.ResponseWriter,
	r *http.Request,
	maxBytes int64,
	target any,
) error {
	if maxBytes < 1 || (r.ContentLength >= 0 && r.ContentLength > maxBytes) {
		return errProviderGatewayRequestTooLarge
	}
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return errProviderGatewayRequestTooLarge
		}
		if providerGatewayCallerControlError(err) {
			return ErrProviderGatewayOperationUnsupported
		}
		return ErrProviderGatewayInvalid
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return ErrProviderGatewayInvalid
	}
	return nil
}

func writeProviderGatewayRequestError(w http.ResponseWriter, err error) {
	if errors.Is(err, errProviderGatewayRequestTooLarge) {
		writeError(
			w,
			http.StatusRequestEntityTooLarge,
			"RAG_PROVIDER_REQUEST_INVALID",
			"RAG provider request exceeds the allowed size",
		)
		return
	}
	if errors.Is(err, ErrProviderGatewayOperationUnsupported) {
		writeError(
			w,
			http.StatusBadRequest,
			"RAG_PROVIDER_OPERATION_UNSUPPORTED",
			"RAG provider operation is unsupported",
		)
		return
	}
	writeError(
		w,
		http.StatusBadRequest,
		"RAG_PROVIDER_REQUEST_INVALID",
		"RAG provider request is invalid",
	)
}

func writeProviderGatewayOperationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrProviderGatewayOperationUnsupported):
		writeError(w, http.StatusBadRequest, "RAG_PROVIDER_OPERATION_UNSUPPORTED", "RAG provider operation is unsupported")
	case errors.Is(err, ErrProviderGatewayInvalid):
		writeError(w, http.StatusBadRequest, "RAG_PROVIDER_REQUEST_INVALID", "RAG provider request is invalid")
	case errors.Is(err, ErrProviderGatewayNotFound):
		writeError(w, http.StatusNotFound, "RAG_PROVIDER_NOT_FOUND", "RAG provider configuration was not found")
	case errors.Is(err, ErrProviderGatewayActivationRequired):
		writeError(w, http.StatusConflict, "RAG_PROVIDER_ACTIVATION_REQUIRED", "RAG provider activation is required")
	case errors.Is(err, ErrProviderGatewayUnavailable):
		writeError(w, http.StatusServiceUnavailable, "RAG_PROVIDER_UNAVAILABLE", "RAG provider gateway is unavailable")
	default:
		writeError(w, http.StatusBadGateway, "RAG_PROVIDER_UPSTREAM_FAILED", "RAG provider upstream request failed")
	}
}

func providerGatewayCallerControlError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, field := range []string{
		"url", "baseUrl", "method", "header", "headers", "authorization",
		"model", "modelId", "task", "operation", "provider", "apiKey",
	} {
		if message == `json: unknown field "`+field+`"` {
			return true
		}
	}
	return false
}

func isProviderGatewayPath(value string) bool {
	return value == InternalMinerUAllocatePath ||
		value == InternalMinerUPollPath ||
		value == InternalSiliconFlowEmbeddingsPath
}

func providerGatewayTokenEqual(got string, want string) bool {
	gotHash := sha256.Sum256([]byte(strings.TrimSpace(got)))
	wantHash := sha256.Sum256([]byte(strings.TrimSpace(want)))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}
